package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/SkyC0der/skyroom/internal/conversation"
	"github.com/SkyC0der/skyroom/internal/identity"
)

type conversationHandlers struct {
	service *conversation.Service
}

type postMessageRequest struct {
	ClientIdempotencyKey string `json:"client_idempotency_key"`
	Body                 string `json:"body"`
}

func (h conversationHandlers) post(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	var request postMessageRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := h.service.Post(r.Context(), token, r.PathValue("roomID"), request.ClientIdempotencyKey, request.Body)
	if err != nil {
		writeConversationError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (h conversationHandlers) history(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		limit = parsed
	}
	page, err := h.service.History(
		r.Context(), token, r.PathValue("roomID"),
		r.URL.Query().Get("before"), r.URL.Query().Get("after"), limit,
	)
	if err != nil {
		writeConversationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func writeConversationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrUnauthenticated):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
	case errors.Is(err, conversation.ErrInvalidMessage):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_message"})
	case errors.Is(err, conversation.ErrInvalidCursor):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
	case errors.Is(err, conversation.ErrNotAuthorized):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not_authorized"})
	case errors.Is(err, conversation.ErrIdempotencyConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency_conflict"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conversation_operation_failed"})
	}
}
