package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/conversation"
	"github.com/SkyC0der/skyroom/internal/identity"
)

type healthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

type config struct {
	identity      *identity.Service
	community     *community.Service
	conversation  *conversation.Service
	secureCookies bool
}

func WithCommunity(service *community.Service) Option {
	return func(config *config) {
		config.community = service
	}
}

func WithConversation(service *conversation.Service) Option {
	return func(config *config) {
		config.conversation = service
	}
}

type Option func(*config)

func WithIdentity(service *identity.Service, secureCookies bool) Option {
	return func(config *config) {
		config.identity = service
		config.secureCookies = secureCookies
	}
}

func New(logger *slog.Logger, webOrigin string, options ...Option) http.Handler {
	configuration := config{}
	for _, option := range options {
		option(&configuration)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", health)
	mux.HandleFunc("GET /api/v1/health/live", live)
	if configuration.identity != nil {
		auth := authHandlers{service: configuration.identity, secureCookies: configuration.secureCookies}
		mux.HandleFunc("POST /api/v1/auth/codes", auth.requestCode)
		mux.HandleFunc("POST /api/v1/auth/verify", auth.verifyCode)
		mux.HandleFunc("GET /api/v1/auth/session", auth.session)
		mux.HandleFunc("DELETE /api/v1/auth/session", auth.signOut)
		mux.HandleFunc("PUT /api/v1/users/me/handle", auth.claimHandle)
	}
	if configuration.community != nil {
		communities := communityHandlers{service: configuration.community}
		mux.HandleFunc("GET /api/v1/communities", communities.list)
		mux.HandleFunc("POST /api/v1/communities", communities.create)
		mux.HandleFunc("POST /api/v1/communities/{communityID}/invitations", communities.createInvitation)
		mux.HandleFunc("DELETE /api/v1/communities/{communityID}/invitations/{invitationID}", communities.revokeInvitation)
		mux.HandleFunc("GET /api/v1/invitations/{token}", communities.previewInvitation)
		mux.HandleFunc("POST /api/v1/invitations/{token}/join", communities.joinInvitation)
	}
	if configuration.conversation != nil {
		conversations := conversationHandlers{service: configuration.conversation, webOrigin: webOrigin}
		mux.HandleFunc("GET /api/v1/rooms/{roomID}/messages", conversations.history)
		mux.HandleFunc("POST /api/v1/rooms/{roomID}/messages", conversations.post)
		mux.HandleFunc("GET /api/v1/rooms/{roomID}/events", conversations.events)
		mux.HandleFunc("GET /api/v1/rooms/{roomID}/read-state", conversations.readState)
		mux.HandleFunc("PUT /api/v1/rooms/{roomID}/read-state", conversations.markRead)
	}

	return recoverPanic(logger, requestLog(logger, cors(webOrigin, enforceOrigin(webOrigin, mux))))
}

func enforceOrigin(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		if requestOrigin != "" && requestOrigin != origin && r.Method != http.MethodOptions {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin_not_allowed"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16_384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return false
	}
	return true
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

func live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func cors(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		if requestOrigin != "" && requestOrigin == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", "Retry-After")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-CSRF-Token")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func recoverPanic(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("request panic", "error", recovered, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_server_error"})
			}
		}()
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
