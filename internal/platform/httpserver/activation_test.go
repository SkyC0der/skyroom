package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/conversation"
	"github.com/SkyC0der/skyroom/internal/identity"
	"github.com/SkyC0der/skyroom/internal/platform/postgres/pgtest"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type activationSender struct {
	mu    sync.Mutex
	codes map[string]string
}

func (s *activationSender) SendVerificationCode(_ context.Context, email, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.codes == nil {
		s.codes = make(map[string]string)
	}
	s.codes[email] = code
	return nil
}

func (s *activationSender) code(email string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.codes[email]
}

func TestActivationJourneySurvivesReconnectAndNewBrowserSession(t *testing.T) {
	pool := pgtest.MigratedPool(t)
	sender := &activationSender{}
	auth := identity.NewService(identity.NewPostgresStore(pool), sender, "activation-test-secret")
	communities := community.NewService(community.NewPostgresStore(pool), auth)
	conversations := conversation.NewService(conversation.NewPostgresStore(pool), auth)
	handler := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)), "http://localhost:3000",
		WithIdentity(auth, false), WithCommunity(communities), WithConversation(conversations),
	)
	server := httptest.NewServer(handler)
	defer server.Close()

	ownerCookie := activateUser(t, server.URL, sender, "activation-owner@example.com", "activation.owner")
	var created community.CreatedCommunity
	activationJSON(t, http.MethodPost, server.URL+"/api/v1/communities", ownerCookie,
		map[string]string{"name": "Activation Guild", "description": "Proves the complete first Room journey."}, http.StatusCreated, &created)
	var invitation community.Invitation
	activationJSON(t, http.MethodPost, server.URL+"/api/v1/communities/"+created.Community.ID+"/invitations", ownerCookie,
		map[string]any{"max_uses": 1}, http.StatusCreated, &invitation)

	memberCookie := activateUser(t, server.URL, sender, "activation-member@example.com", "activation.member")
	var preview community.InvitationPreview
	activationJSON(t, http.MethodGet, server.URL+"/api/v1/invitations/"+url.PathEscape(invitation.Token), nil, nil, http.StatusOK, &preview)
	if preview.CommunityID != created.Community.ID || preview.InviterHandle != "activation.owner" {
		t.Fatalf("unexpected invitation preview: %+v", preview)
	}
	var joined community.JoinResult
	activationJSON(t, http.MethodPost, server.URL+"/api/v1/invitations/"+url.PathEscape(invitation.Token)+"/join", memberCookie, nil, http.StatusOK, &joined)
	if joined.AlreadyMember || joined.Community.ID != created.Community.ID {
		t.Fatalf("unexpected join result: %+v", joined)
	}

	ownerSocket := activationSocket(t, server.URL, created.GeneralRoom.ID, ownerCookie)
	memberSocket := activationSocket(t, server.URL, created.GeneralRoom.ID, memberCookie)
	memberMessage := activationPostMessage(t, server.URL, created.GeneralRoom.ID, memberCookie, "activation-member-message", "Hello from the invited Member.")
	ownerEvent := activationReadEvent(t, ownerSocket)
	if ownerEvent.Payload.Message.ID != memberMessage.Message.ID {
		t.Fatalf("owner received the wrong live Message: event=%+v posted=%+v", ownerEvent, memberMessage)
	}
	memberEcho := activationReadEvent(t, memberSocket)
	if memberEcho.Payload.Message.ID != memberMessage.Message.ID {
		t.Fatalf("member did not reconcile its committed Message: event=%+v posted=%+v", memberEcho, memberMessage)
	}
	ownerMessage := activationPostMessage(t, server.URL, created.GeneralRoom.ID, ownerCookie, "activation-owner-message", "Welcome to the Room.")
	memberEvent := activationReadEvent(t, memberSocket)
	if memberEvent.Payload.Message.ID != ownerMessage.Message.ID || memberEvent.Cursor == "" {
		t.Fatalf("member received the wrong live Message: event=%+v posted=%+v", memberEvent, ownerMessage)
	}

	_ = ownerSocket.Close(websocket.StatusNormalClosure, "activation reconnect check")
	_ = memberSocket.Close(websocket.StatusNormalClosure, "activation reconnect check")

	// A fresh HTTP client represents reopening Skyroom in a new browser session.
	activationJSON(t, http.MethodGet, server.URL+"/api/v1/auth/session", ownerCookie, nil, http.StatusOK, nil)
	activationJSON(t, http.MethodGet, server.URL+"/api/v1/auth/session", memberCookie, nil, http.StatusOK, nil)

	missed := activationPostMessage(t, server.URL, created.GeneralRoom.ID, ownerCookie, "activation-gap-message", "This Message was sent while the Member was disconnected.")
	reconnected := activationSocket(t, server.URL, created.GeneralRoom.ID, memberCookie)
	defer reconnected.CloseNow()
	var recovered conversation.HistoryPage
	activationJSON(t, http.MethodGet,
		server.URL+"/api/v1/rooms/"+created.GeneralRoom.ID+"/messages?after="+url.QueryEscape(memberEvent.Cursor),
		memberCookie, nil, http.StatusOK, &recovered)
	if len(recovered.Messages) != 1 || recovered.Messages[0].ID != missed.Message.ID {
		t.Fatalf("expected cursor recovery to return the disconnected Message, got %+v", recovered)
	}
}

func activateUser(t *testing.T, serverURL string, sender *activationSender, email, handle string) *http.Cookie {
	t.Helper()
	activationJSON(t, http.MethodPost, serverURL+"/api/v1/auth/codes", nil, map[string]string{"email": email}, http.StatusAccepted, nil)
	code := sender.code(email)
	if code == "" {
		t.Fatalf("verification code was not delivered for %s", email)
	}
	response := activationRequest(t, http.MethodPost, serverURL+"/api/v1/auth/verify", nil, map[string]string{"email": email, "code": code})
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("verify %s: status=%d body=%s", email, response.StatusCode, body)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == sessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("invalid session cookie for %s: %+v", email, sessionCookie)
	}
	activationJSON(t, http.MethodPut, serverURL+"/api/v1/users/me/handle", sessionCookie, map[string]string{"handle": handle}, http.StatusOK, nil)
	return sessionCookie
}

func activationPostMessage(t *testing.T, serverURL, roomID string, cookie *http.Cookie, key, body string) conversation.PostResult {
	t.Helper()
	var result conversation.PostResult
	activationJSON(t, http.MethodPost, serverURL+"/api/v1/rooms/"+roomID+"/messages", cookie,
		map[string]string{"client_idempotency_key": key, "body": body}, http.StatusCreated, &result)
	return result
}

func activationSocket(t *testing.T, serverURL, roomID string, cookie *http.Cookie) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Cookie", cookie.String())
	header.Set("Origin", "http://localhost:3000")
	websocketURL := "ws" + serverURL[len("http"):] + "/api/v1/rooms/" + roomID + "/events"
	connection, response, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if response != nil {
			t.Fatalf("connect Room events: status=%d err=%v", response.StatusCode, err)
		}
		t.Fatalf("connect Room events: %v", err)
	}
	return connection
}

func activationReadEvent(t *testing.T, connection *websocket.Conn) conversation.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var event conversation.Event
	if err := wsjson.Read(ctx, connection, &event); err != nil {
		t.Fatalf("read Room event: %v", err)
	}
	return event
}

func activationJSON(t *testing.T, method, requestURL string, cookie *http.Cookie, requestBody any, expectedStatus int, destination ...any) {
	t.Helper()
	response := activationRequest(t, method, requestURL, cookie, requestBody)
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, requestURL, response.StatusCode, expectedStatus, body)
	}
	var target any
	if len(destination) > 0 {
		target = destination[0]
	}
	if target != nil && response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatalf("decode %s %s: %v", method, requestURL, err)
		}
	}
}

func activationRequest(t *testing.T, method, requestURL string, cookie *http.Cookie, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, requestURL, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, requestURL, err)
	}
	return response
}
