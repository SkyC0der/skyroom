package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/SkyC0der/skyroom/internal/identity"
)

const sessionCookieName = "skyroom_session"

type authHandlers struct {
	service       *identity.Service
	secureCookies bool
}

type emailRequest struct {
	Email string `json:"email"`
}

type verifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func (h authHandlers) requestCode(w http.ResponseWriter, r *http.Request) {
	var request emailRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := h.service.RequestCode(r.Context(), request.Email); err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidEmail):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		case errors.Is(err, identity.ErrResendTooSoon):
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "resend_too_soon"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "verification_delivery_failed"})
		}
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h authHandlers) verifyCode(w http.ResponseWriter, r *http.Request) {
	var request verifyRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	verified, err := h.service.VerifyCode(r.Context(), request.Email, request.Code)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidEmail):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		case errors.Is(err, identity.ErrTooManyAttempts):
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "verification_locked"})
		default:
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_or_expired_code"})
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: verified.Token, Path: "/",
		Expires: verified.ExpiresAt, MaxAge: int(time.Until(verified.ExpiresAt).Seconds()),
		HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         verified.User,
		"needs_handle": verified.User.Handle == nil,
	})
}
