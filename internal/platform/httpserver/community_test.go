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

	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/identity"
)

func TestCommunityInvitationHTTPFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &handlerSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	communities := community.NewService(community.NewMemoryStore(), auth)
	handler := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)), "http://localhost:3000",
		WithIdentity(auth, false), WithCommunity(communities),
	)

	verifiedCookie := func(email, handle string) *http.Cookie {
		t.Helper()
		if err := auth.RequestCode(ctx, email); err != nil {
			t.Fatal(err)
		}
		verified, err := auth.VerifyCode(ctx, email, sender.code)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := auth.ClaimHandle(ctx, verified.Token, handle); err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: sessionCookieName, Value: verified.Token}
	}

	ownerCookie := verifiedCookie("owner@example.com", "http.owner")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/communities", bytes.NewBufferString(`{"name":"HTTP Guild","description":"An end-to-end community."}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(ownerCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create community: status=%d body=%s", response.Code, response.Body.String())
	}
	var created community.CreatedCommunity
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/communities/"+created.Community.ID+"/invitations", bytes.NewBufferString(`{"max_uses":2}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(ownerCookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create invitation: status=%d body=%s", response.Code, response.Body.String())
	}
	var invitation community.Invitation
	if err := json.NewDecoder(response.Body).Decode(&invitation); err != nil {
		t.Fatal(err)
	}
	if invitation.Token == "" {
		t.Fatal("expected raw invitation token in creation response")
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/invitations/"+invitation.Token, nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preview invitation: status=%d body=%s", response.Code, response.Body.String())
	}
	var preview community.InvitationPreview
	if err := json.NewDecoder(response.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if preview.CommunityName != "HTTP Guild" || preview.InviterHandle != "http.owner" || preview.MemberCount != 1 {
		t.Fatalf("unexpected invitation preview: %+v", preview)
	}

	memberCookie := verifiedCookie("member@example.com", "http.member")
	request = httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+invitation.Token+"/join", nil)
	request.AddCookie(memberCookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("join invitation: status=%d body=%s", response.Code, response.Body.String())
	}
	var joined community.JoinResult
	if err := json.NewDecoder(response.Body).Decode(&joined); err != nil {
		t.Fatal(err)
	}
	if joined.AlreadyMember || joined.Community.MemberCount != 2 {
		t.Fatalf("unexpected join response: %+v", joined)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+invitation.Token+"/join", nil)
	request.AddCookie(memberCookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("repeat join: status=%d body=%s", response.Code, response.Body.String())
	}
	if err := json.NewDecoder(response.Body).Decode(&joined); err != nil {
		t.Fatal(err)
	}
	if !joined.AlreadyMember {
		t.Fatal("expected repeat join to report existing membership")
	}
}
