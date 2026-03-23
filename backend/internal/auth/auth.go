package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/webssh/manager/internal/models"
	"github.com/webssh/manager/pkg/crypto"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	db        *sql.DB
	masterKey []byte
}

func NewService(db *sql.DB, masterKey []byte) *Service {
	return &Service{db: db, masterKey: masterKey}
}

// LoginResult is the discriminated result of a Login call.
// If Requires2FA is true, TempToken is set and Session is nil.
// Otherwise Session is set and TempToken is empty.
type LoginResult struct {
	Session     *models.Session
	TempToken   string
	Requires2FA bool
}

func (s *Service) Register(username, password string) (*models.User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	result, err := s.db.Exec(
		"INSERT INTO users (username, password_hash, encryption_key_salt) VALUES (?, ?, ?)",
		username, string(passwordHash), salt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &models.User{
		ID:                int(id),
		Username:          username,
		EncryptionKeySalt: salt,
		CreatedAt:         time.Now(),
	}, nil
}

func (s *Service) Login(username, password string) (*LoginResult, error) {
	var user models.User
	var totpEnabled int
	err := s.db.QueryRow(
		"SELECT id, username, password_hash, encryption_key_salt, totp_enabled FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.EncryptionKeySalt, &totpEnabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Derive credential encryption key from password
	encryptionKey, err := crypto.DeriveKey(password, user.EncryptionKeySalt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}
	encryptionKeyB64 := base64.StdEncoding.EncodeToString(encryptionKey)

	// Wrap the key with the server master key
	wrappedKey, err := crypto.Encrypt(encryptionKeyB64, s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap encryption key: %w", err)
	}

	// If 2FA is enabled, issue a short-lived temp token instead of a full session
	if totpEnabled == 1 {
		token, err := s.CreateTempToken(user.ID, wrappedKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create temp token: %w", err)
		}
		return &LoginResult{TempToken: token, Requires2FA: true}, nil
	}

	return s.createSession(user.ID, wrappedKey)
}

func (s *Service) createSession(userID int, wrappedKey string) (*LoginResult, error) {
	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	csrfToken, err := generateToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	_, err = s.db.Exec(
		"INSERT INTO sessions (token, user_id, encryption_key, csrf_token, expires_at) VALUES (?, ?, ?, ?, ?)",
		token, userID, wrappedKey, csrfToken, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &LoginResult{
		Session: &models.Session{
			Token:     token,
			CSRFToken: csrfToken,
			UserID:    userID,
			ExpiresAt: expiresAt,
			CreatedAt: time.Now(),
		},
	}, nil
}

func (s *Service) ValidateSession(token string) (*models.User, error) {
	var user models.User
	var wrappedKey string
	err := s.db.QueryRow(`
		SELECT u.id, u.username, u.encryption_key_salt, s.encryption_key, s.csrf_token
		FROM users u
		JOIN sessions s ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ?
	`, token, time.Now()).Scan(
		&user.ID, &user.Username, &user.EncryptionKeySalt, &wrappedKey, &user.CSRFToken,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid or expired session")
		}
		return nil, err
	}

	encryptionKeyB64, err := crypto.Decrypt(wrappedKey, s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired session")
	}
	user.EncryptionKey = encryptionKeyB64

	return &user, nil
}

func (s *Service) RevokeSession(token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// --- TOTP methods ---

// GenerateTOTPSecret generates a new TOTP key for the given user (by username lookup).
// It does NOT store anything — storage happens in EnableTOTP after code confirmation.
func (s *Service) GenerateTOTPSecret(userID int) (secret, otpauthURL string, err error) {
	var username string
	if err := s.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username); err != nil {
		return "", "", fmt.Errorf("user not found: %w", err)
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "WebSSH Manager",
		AccountName: username,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

// EnableTOTP verifies the provided code against plaintextSecret and, if valid,
// encrypts and stores the secret in the database with totp_enabled = 1.
func (s *Service) EnableTOTP(userID int, plaintextSecret, code string) error {
	if !totp.Validate(code, plaintextSecret) {
		return fmt.Errorf("invalid TOTP code")
	}

	encryptedSecret, err := crypto.Encrypt(plaintextSecret, s.masterKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt TOTP secret: %w", err)
	}

	_, err = s.db.Exec(
		"UPDATE users SET totp_secret = ?, totp_enabled = 1 WHERE id = ?",
		encryptedSecret, userID,
	)
	return err
}

// DisableTOTP verifies the provided code and clears TOTP from the account.
func (s *Service) DisableTOTP(userID int, code string) error {
	var encryptedSecret string
	if err := s.db.QueryRow(
		"SELECT totp_secret FROM users WHERE id = ?", userID,
	).Scan(&encryptedSecret); err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	plaintextSecret, err := crypto.Decrypt(encryptedSecret, s.masterKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt TOTP secret: %w", err)
	}

	if !totp.Validate(code, plaintextSecret) {
		return fmt.Errorf("invalid TOTP code")
	}

	_, err = s.db.Exec(
		"UPDATE users SET totp_secret = '', totp_enabled = 0 WHERE id = ?", userID,
	)
	return err
}

// ValidateTOTP checks the given code against the stored TOTP secret for the user.
func (s *Service) ValidateTOTP(userID int, code string) error {
	var encryptedSecret string
	if err := s.db.QueryRow(
		"SELECT totp_secret FROM users WHERE id = ?", userID,
	).Scan(&encryptedSecret); err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	plaintextSecret, err := crypto.Decrypt(encryptedSecret, s.masterKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt TOTP secret: %w", err)
	}

	if !totp.Validate(code, plaintextSecret) {
		return fmt.Errorf("invalid TOTP code")
	}
	return nil
}

// TOTPStatus returns whether 2FA is enabled for the given user.
func (s *Service) TOTPStatus(userID int) (bool, error) {
	var enabled int
	err := s.db.QueryRow(
		"SELECT totp_enabled FROM users WHERE id = ?", userID,
	).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled == 1, nil
}

// CreateTempToken stores a short-lived token tied to a pre-authenticated user
// (password verified but 2FA not yet confirmed). Expires in 5 minutes.
func (s *Service) CreateTempToken(userID int, wrappedKey string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = s.db.Exec(
		"INSERT INTO temp_tokens (token, user_id, encrypted_key, expires_at) VALUES (?, ?, ?, ?)",
		token, userID, wrappedKey, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create temp token: %w", err)
	}
	return token, nil
}

// ConsumeTempToken atomically validates and deletes a temp token.
// Returns the token data if valid and not expired.
func (s *Service) ConsumeTempToken(token string) (*models.TempToken, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var tt models.TempToken
	tt.Token = token
	err = tx.QueryRow(
		"SELECT user_id, encrypted_key, expires_at FROM temp_tokens WHERE token = ? AND expires_at > ?",
		token, time.Now(),
	).Scan(&tt.UserID, &tt.EncryptedKey, &tt.ExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid or expired temp token")
		}
		return nil, err
	}

	if _, err := tx.Exec("DELETE FROM temp_tokens WHERE token = ?", token); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &tt, nil
}

// CompleteTOTPLogin creates a full session from a consumed temp token.
func (s *Service) CompleteTOTPLogin(tt *models.TempToken) (*LoginResult, error) {
	return s.createSession(tt.UserID, tt.EncryptedKey)
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// --- WebSocket ticket ---
//
// A WS ticket is a short-lived (60 s) HMAC-signed token that lets the browser
// authenticate a WebSocket upgrade without relying on cookie transmission,
// which is unreliable in some browser / SameSite configurations.
//
// Format (before base64url): "<userID>:<unix_ts>:<hmac_hex>"
// HMAC key  : server master key
// HMAC input: "ws-ticket:<userID>:<unix_ts>"

// GenerateWSTicket creates a 60-second single-origin ticket for the given user.
func (s *Service) GenerateWSTicket(userID int) (string, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	uid := strconv.Itoa(userID)
	mac := hmac.New(sha256.New, s.masterKey)
	mac.Write([]byte("ws-ticket:" + uid + ":" + ts))
	sig := hex.EncodeToString(mac.Sum(nil))
	raw := uid + ":" + ts + ":" + sig
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

// ValidateWSTicket verifies the ticket signature and expiry, then returns the
// user loaded from their most-recent valid session (needed for the encryption key).
func (s *Service) ValidateWSTicket(ticket string) (*models.User, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(ticket)
	if err != nil {
		return nil, fmt.Errorf("invalid ticket encoding")
	}

	parts := strings.SplitN(string(decoded), ":", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ticket format")
	}
	uidStr, tsStr, sig := parts[0], parts[1], parts[2]

	userID, err := strconv.Atoi(uidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ticket user")
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ticket timestamp")
	}
	if time.Now().Unix()-ts > 60 {
		return nil, fmt.Errorf("ticket expired")
	}

	mac := hmac.New(sha256.New, s.masterKey)
	mac.Write([]byte("ws-ticket:" + uidStr + ":" + tsStr))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid ticket signature")
	}

	// Load the user + encryption key from their most recent valid session.
	var user models.User
	var wrappedKey string
	err = s.db.QueryRow(`
		SELECT u.id, u.username, u.encryption_key_salt, s.encryption_key, s.csrf_token
		FROM users u
		JOIN sessions s ON u.id = s.user_id
		WHERE u.id = ? AND s.expires_at > ?
		ORDER BY s.created_at DESC
		LIMIT 1
	`, userID, time.Now()).Scan(
		&user.ID, &user.Username, &user.EncryptionKeySalt, &wrappedKey, &user.CSRFToken,
	)
	if err != nil {
		return nil, fmt.Errorf("no valid session for ticket user")
	}

	encryptionKeyB64, err := crypto.Decrypt(wrappedKey, s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt user key")
	}
	user.EncryptionKey = encryptionKeyB64
	return &user, nil
}
