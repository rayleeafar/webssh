package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/middleware"
)

type TOTPHandler struct {
	authService  *auth.Service
	secureCookie bool
}

func NewTOTPHandler(authService *auth.Service, secureCookie bool) *TOTPHandler {
	return &TOTPHandler{authService: authService, secureCookie: secureCookie}
}

// SetupTOTP handles GET /api/auth/2fa/setup
// Requires auth. Generates a new TOTP secret and returns it with the otpauth URL.
// Does NOT store anything yet — storage happens on EnableTOTP confirmation.
func (h *TOTPHandler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	secret, otpauthURL, err := h.authService.GenerateTOTPSecret(user.ID)
	if err != nil {
		http.Error(w, "Failed to generate 2FA setup", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"secret":      secret,
		"otpauth_url": otpauthURL,
	})
}

// GetTOTPStatus handles GET /api/auth/2fa/status
// Requires auth. Returns whether 2FA is enabled for the current user.
func (h *TOTPHandler) GetTOTPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	enabled, err := h.authService.TOTPStatus(user.ID)
	if err != nil {
		http.Error(w, "Failed to get 2FA status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": enabled})
}

// EnableTOTP handles POST /api/auth/2fa/enable
// Requires auth + CSRF. Body: {"secret": "...", "code": "123456"}.
func (h *TOTPHandler) EnableTOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Secret == "" || req.Code == "" {
		http.Error(w, "secret and code are required", http.StatusBadRequest)
		return
	}

	if err := h.authService.EnableTOTP(user.ID, req.Secret, req.Code); err != nil {
		http.Error(w, "Invalid code", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "2FA enabled"})
}

// DisableTOTP handles POST /api/auth/2fa/disable
// Requires auth + CSRF. Body: {"code": "123456"}.
func (h *TOTPHandler) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	if err := h.authService.DisableTOTP(user.ID, req.Code); err != nil {
		http.Error(w, "Invalid code", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// VerifyTOTP handles POST /api/auth/2fa/verify
// No auth required. Completes a pending 2FA login by consuming the temp token
// and validating the TOTP code, then creates a full session.
func (h *TOTPHandler) VerifyTOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TempToken string `json:"temp_token"`
		Code      string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.TempToken == "" || req.Code == "" {
		http.Error(w, "temp_token and code are required", http.StatusBadRequest)
		return
	}

	// Consume the temp token atomically
	tt, err := h.authService.ConsumeTempToken(req.TempToken)
	if err != nil {
		http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
		return
	}

	// Validate the TOTP code
	if err := h.authService.ValidateTOTP(tt.UserID, req.Code); err != nil {
		http.Error(w, "Invalid authenticator code", http.StatusUnauthorized)
		return
	}

	// Create full session using the wrapped key from the temp token
	result, err := h.authService.CompleteTOTPLogin(tt)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	session := result.Session
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":    session.UserID,
		"csrf_token": session.CSRFToken,
	})
}
