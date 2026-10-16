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
	"github.com/SkyC0der/skyroom/internal/conversation"
	"github.com/SkyC0der/skyroom/internal/identity"
)

func TestConversationHTTPFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &handlerSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	communityStore := community.NewMemoryStore()
	communities := community.NewService(communityStore, auth)
	conversations := conversation.NewService(conversation.NewMemoryStore(communityStore), auth)
	handler := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)), "http://localhost:3000",
		WithIdentity(auth, false), WithCommunity(communities), WithConversation(conversations),
	)
	if err := auth.RequestCode(ctx, "message-http@example.com"); err != nil {
		t.Fatal(err)
	}
	verified, err := auth.VerifyCode(ctx, "message-http@example.com", sender.code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ClaimHandle(ctx, verified.Token, "http.messenger"); err != nil {
		t.Fatal(err)
	}
	created, err := communities.Create(ctx, verified.Token, "HTTP Messages", "")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: verified.Token}
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+created.GeneralRoom.ID+"/messages", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	response := post(`{"client_idempotency_key":"http-client-key","body":"Hello, room."}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("post message: status=%d body=%s", response.Code, response.Body.String())
	}
	var posted conversation.PostResult
	if err := json.NewDecoder(response.Body).Decode(&posted); err != nil {
		t.Fatal(err)
	}
	response = post(`{"client_idempotency_key":"http-client-key","body":"Hello, room."}`)
	if response.Code != http.StatusOK {
		t.Fatalf("repeat message: status=%d body=%s", response.Code, response.Body.String())
	}
	var repeated conversation.PostResult
	if err := json.NewDecoder(response.Body).Decode(&repeated); err != nil {
		t.Fatal(err)
	}
	if !repeated.Duplicate || repeated.Message.ID != posted.Message.ID {
		t.Fatalf("unexpected repeat response: %+v", repeated)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+created.GeneralRoom.ID+"/messages?limit=20", nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("message history: status=%d body=%s", response.Code, response.Body.String())
	}
	var page conversation.HistoryPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].Body != "Hello, room." || page.LatestCursor == "" {
		t.Fatalf("unexpected history: %+v", page)
	}
}
