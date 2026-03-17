package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/webssh/manager/internal/models"
	"github.com/webssh/manager/pkg/crypto"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
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

func (s *Service) Login(username, password string) (*models.Session, error) {
	var user models.User
	err := s.db.QueryRow(
		"SELECT id, username, password_hash, encryption_key_salt FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.EncryptionKeySalt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Derive encryption key from password
	encryptionKey, err := crypto.DeriveKey(password, user.EncryptionKeySalt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}

	// Encode the key as base64 for storage
	encryptionKeyB64 := base64.StdEncoding.EncodeToString(encryptionKey)

	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	_, err = s.db.Exec(
		"INSERT INTO sessions (token, user_id, encryption_key, expires_at) VALUES (?, ?, ?, ?)",
		token, user.ID, encryptionKeyB64, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &models.Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

func (s *Service) ValidateSession(token string) (*models.User, error) {
	var user models.User
	err := s.db.QueryRow(`
		SELECT u.id, u.username, u.encryption_key_salt, s.encryption_key
		FROM users u
		JOIN sessions s ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ?
	`, token, time.Now()).Scan(&user.ID, &user.Username, &user.EncryptionKeySalt, &user.EncryptionKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid or expired session")
		}
		return nil, err
	}

	return &user, nil
}

func (s *Service) RevokeSession(token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
