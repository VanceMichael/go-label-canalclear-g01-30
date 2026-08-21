package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func TestRequestContextAcceptsSafeClientID(t *testing.T) {
	var seen string
	handler := requestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "port-request-123")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || seen != "port-request-123" || recorder.Header().Get("X-Request-ID") != seen {
		t.Fatalf("status=%d seen=%q headers=%v", recorder.Code, seen, recorder.Header())
	}
}

func TestRequestContextReplacesUnsafeOrMissingID(t *testing.T) {
	for _, supplied := range []string{"", "short", "contains spaces", strings.Repeat("a", 129)} {
		t.Run(supplied, func(t *testing.T) {
			var seen string
			handler := requestContext(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = RequestID(r.Context()) }))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-Request-ID", supplied)
			handler.ServeHTTP(httptest.NewRecorder(), request)
			if len(seen) != 32 || seen == supplied {
				t.Fatalf("supplied=%q generated=%q", supplied, seen)
			}
		})
	}
}

func TestSecurityHeadersProtectAPIResponses(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers=%v", recorder.Header())
	}
	if !strings.Contains(recorder.Header().Get("Content-Security-Policy"), "frame-ancestors") {
		t.Fatalf("CSP=%q", recorder.Header().Get("Content-Security-Policy"))
	}
}

func TestRecoverPanicReturnsStableErrorContract(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := requestContext(recoverPanic(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "internal_error" || response.Error.RequestID == "" || strings.Contains(recorder.Body.String(), "boom") {
		t.Fatalf("response=%+v", response)
	}
}

func TestClassifyErrorMapsDomainSemantics(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{err: domain.ErrInvalid, status: http.StatusBadRequest, code: "invalid_request"},
		{err: domain.ErrForbidden, status: http.StatusForbidden, code: "forbidden"},
		{err: domain.ErrNotFound, status: http.StatusNotFound, code: "not_found"},
		{err: domain.ErrConflict, status: http.StatusConflict, code: "conflict"},
		{err: domain.ErrState, status: http.StatusConflict, code: "invalid_state"},
		{err: domain.ErrExpired, status: http.StatusUnauthorized, code: "session_expired"},
		{err: errors.New("database unavailable"), status: http.StatusInternalServerError, code: "internal_error"},
	}
	for _, test := range tests {
		status, code, message := classifyError(test.err)
		if status != test.status || code != test.code || message == "" {
			t.Errorf("err=%v status=%d code=%q message=%q", test.err, status, code, message)
		}
	}
}

func TestResponseCaptureWritesStatusOnlyOnce(t *testing.T) {
	recorder := httptest.NewRecorder()
	capture := &responseCapture{ResponseWriter: recorder}
	capture.WriteHeader(http.StatusCreated)
	capture.WriteHeader(http.StatusInternalServerError)
	count, err := capture.Write([]byte("created"))
	if err != nil || count != 7 || capture.status != http.StatusCreated || capture.bytes != 7 || recorder.Code != http.StatusCreated {
		t.Fatalf("capture=%+v recorder=%+v err=%v", capture, recorder, err)
	}
}

func TestRequestIDReturnsEmptyOutsideMiddleware(t *testing.T) {
	if requestID := RequestID(context.Background()); requestID != "" {
		t.Fatalf("request id=%q", requestID)
	}
}
