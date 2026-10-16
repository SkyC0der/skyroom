package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SkyC0der/skyroom/internal/identity"
)

type handlerSender struct {
	code string
}

func (s *handlerSender) SendVerificationCode(_ context.Context, _, code string) error {
	s.code = code
	return nil
}

func TestAuthenticationCodeFlow(t *testing.T) {
	t.Parallel()

	sender := &handlerSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	handler := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"http://localhost:3000",
		WithIdentity(auth, false),
	)

	requestBody, _ := json.Marshal(map[string]string{"email": "Member@Example.com"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/codes", bytes.NewReader(requestBody))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected request status %d, got %d: %s", http.StatusAccepted, response.Code, response.Body.String())
	}
	if sender.code == "" {
		t.Fatal("expected verification code to be delivered")
	}

	verifyBody, _ := json.Marshal(map[string]string{"email": "member@example.com", "code": sender.code})
	request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewReader(verifyBody))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected verify status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly {
		t.Fatalf("expected HTTP-only session cookie, got %+v", cookies)
	}

	var body struct {
		NeedsHandle bool `json:"needs_handle"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.NeedsHandle {
		t.Fatal("expected a new user to require a handle")
	}

	sessionCookie := cookies[0]
	request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	request.AddCookie(sessionCookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected active session status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	handleBody, _ := json.Marshal(map[string]string{"handle": "Sky.Member"})
	request = httptest.NewRequest(http.MethodPut, "/api/v1/users/me/handle", bytes.NewReader(handleBody))
	request.AddCookie(sessionCookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected handle status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	var handled struct {
		NeedsHandle bool          `json:"needs_handle"`
		User        identity.User `json:"user"`
	}
	if err := json.NewDecoder(response.Body).Decode(&handled); err != nil {
		t.Fatal(err)
	}
	if handled.NeedsHandle || handled.User.Handle == nil || *handled.User.Handle != "sky.member" {
		t.Fatalf("unexpected handle response: %+v", handled)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
	request.AddCookie(sessionCookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected sign out status %d, got %d", http.StatusNoContent, response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	request.AddCookie(sessionCookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked session status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestAuthenticationRejectsUnknownJSONFields(t *testing.T) {
	t.Parallel()

	auth := identity.NewService(identity.NewMemoryStore(), &handlerSender{}, "test-secret")
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), "http://localhost:3000", WithIdentity(auth, false))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/codes", bytes.NewBufferString(`{"email":"member@example.com","admin":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestRejectsUnexpectedMutationOrigin(t *testing.T) {
	t.Parallel()

	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), "http://localhost:3000")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/codes", bytes.NewBufferString(`{"email":"member@example.com"}`))
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
	}
}
