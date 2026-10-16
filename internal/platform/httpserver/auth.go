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

type handleRequest struct {
	Handle string `json:"handle"`
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

	h.setSessionCookie(w, verified.Token, verified.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         verified.User,
		"needs_handle": verified.User.Handle == nil,
	})
}

func (h authHandlers) session(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	authenticated, err := h.service.Authenticate(r.Context(), token)
	if err != nil {
		if errors.Is(err, identity.ErrUnauthenticated) {
			h.clearSessionCookie(w)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session_lookup_failed"})
		return
	}
	h.setSessionCookie(w, token, authenticated.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": authenticated.User, "needs_handle": authenticated.User.Handle == nil,
	})
}

func (h authHandlers) signOut(w http.ResponseWriter, r *http.Request) {
	if token, ok := readSessionToken(r); ok {
		if err := h.service.SignOut(r.Context(), token); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sign_out_failed"})
			return
		}
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h authHandlers) claimHandle(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	var request handleRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	authenticated, err := h.service.ClaimHandle(r.Context(), token, request.Handle)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrUnauthenticated):
			h.clearSessionCookie(w)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		case errors.Is(err, identity.ErrVerificationStale):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "fresh_verification_required"})
		case errors.Is(err, identity.ErrInvalidHandle):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		case errors.Is(err, identity.ErrHandleTaken):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "handle_taken"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "handle_update_failed"})
		}
		return
	}
	h.setSessionCookie(w, token, authenticated.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": authenticated.User, "needs_handle": false,
	})
}

func readSessionToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (h authHandlers) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/",
		Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds()),
		HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func (h authHandlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Path: "/", MaxAge: -1,
		Expires: time.Unix(1, 0), HttpOnly: true, Secure: h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}
