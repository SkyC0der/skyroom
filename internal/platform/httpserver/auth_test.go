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
