package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/conversation"
	"github.com/SkyC0der/skyroom/internal/identity"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
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

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/rooms/"+created.GeneralRoom.ID+"/messages/"+posted.Message.ID, strings.NewReader(`{"body":"Hello, edited room.","expected_revision":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("edit message: status=%d body=%s", response.Code, response.Body.String())
	}
	var edited conversation.MutationResult
	if err := json.NewDecoder(response.Body).Decode(&edited); err != nil {
		t.Fatal(err)
	}
	if edited.Message.Body != "Hello, edited room." || edited.Message.Revision != 2 || edited.Message.EditedAt == nil {
		t.Fatalf("unexpected edited message: %+v", edited)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+created.GeneralRoom.ID+"/messages?limit=20", nil)
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
	if len(page.Messages) != 1 || page.Messages[0].Body != "Hello, edited room." || page.LatestCursor == "" {
		t.Fatalf("unexpected history: %+v", page)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+created.GeneralRoom.ID+"/read-state", nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var readState conversation.ReadState
	if response.Code != http.StatusOK {
		t.Fatalf("get read state: status=%d body=%s", response.Code, response.Body.String())
	}
	if err := json.NewDecoder(response.Body).Decode(&readState); err != nil {
		t.Fatal(err)
	}
	if readState.UnreadCount != 1 {
		t.Fatalf("expected one unread Message, got %+v", readState)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/v1/rooms/"+created.GeneralRoom.ID+"/read-state", strings.NewReader(`{"last_message_id":"`+posted.Message.ID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("mark read: status=%d body=%s", response.Code, response.Body.String())
	}
	if err := json.NewDecoder(response.Body).Decode(&readState); err != nil {
		t.Fatal(err)
	}
	if readState.UnreadCount != 0 || readState.LastReadMessageID != posted.Message.ID {
		t.Fatalf("unexpected marked read state: %+v", readState)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/"+created.GeneralRoom.ID+"/messages/"+posted.Message.ID, strings.NewReader(`{"expected_revision":2}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delete message: status=%d body=%s", response.Code, response.Body.String())
	}
	var deleted conversation.MutationResult
	if err := json.NewDecoder(response.Body).Decode(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.Message.Body != "" || deleted.Message.Revision != 3 || deleted.Message.DeletedAt == nil {
		t.Fatalf("unexpected deleted message: %+v", deleted)
	}
}

func TestRoomEventsWebSocketDeliversCommittedMessage(t *testing.T) {
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
	if err := auth.RequestCode(ctx, "events-http@example.com"); err != nil {
		t.Fatal(err)
	}
	verified, err := auth.VerifyCode(ctx, "events-http@example.com", sender.code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ClaimHandle(ctx, verified.Token, "events.messenger"); err != nil {
		t.Fatal(err)
	}
	created, err := communities.Create(ctx, verified.Token, "Live HTTP Messages", "")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	header := http.Header{}
	header.Set("Cookie", (&http.Cookie{Name: sessionCookieName, Value: verified.Token}).String())
	header.Set("Origin", "http://localhost:3000")
	connection, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/rooms/"+created.GeneralRoom.ID+"/events", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if response != nil {
			t.Fatalf("dial Room events: status=%d err=%v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	defer connection.CloseNow()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/rooms/"+created.GeneralRoom.ID+"/messages", strings.NewReader(`{"client_idempotency_key":"websocket-client-key","body":"Live over WebSocket"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: verified.Token})
	posted, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer posted.Body.Close()
	if posted.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(posted.Body)
		t.Fatalf("post message: status=%d body=%s", posted.StatusCode, body)
	}

	var event conversation.Event
	if err := wsjson.Read(ctx, connection, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "message.created" || event.Payload.Message.Body != "Live over WebSocket" || event.Cursor == "" {
		t.Fatalf("unexpected Room event: %+v", event)
	}
}
