package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	New(slog.New(slog.NewTextHandler(io.Discard, nil)), "http://localhost:3000").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected ok status, got %q", body.Status)
	}
}

func TestCORSAllowsConfiguredDevelopmentOrigin(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	New(slog.New(slog.NewTextHandler(io.Discard, nil)), "http://localhost:3000").ServeHTTP(response, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("expected configured CORS origin, got %q", got)
	}
	if got := response.Header().Get("Access-Control-Expose-Headers"); got != "Retry-After, X-Request-ID" {
		t.Fatalf("expected operational headers to be exposed, got %q", got)
	}
}

func TestRequestLoggingAndMetricsCaptureStatusLatencyAndRequestID(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	handler := New(slog.New(slog.NewJSONHandler(&logs, nil)), "http://localhost:3000")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected a generated request ID")
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected a not-found response, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", response.Code, response.Body.String())
	}
	var metrics metricsResponse
	if err := json.NewDecoder(response.Body).Decode(&metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.Requests != 2 || metrics.ClientErrors != 1 || metrics.ServerErrors != 0 {
		t.Fatalf("unexpected metrics snapshot: %+v", metrics)
	}
	if metrics.TotalDurationMicros == 0 {
		t.Fatal("expected request duration to be accumulated")
	}

	var entry map[string]any
	decoder := json.NewDecoder(&logs)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"request_id", "method", "path", "status", "response_bytes", "duration_ms"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("structured request log is missing %q: %+v", key, entry)
		}
	}
}
