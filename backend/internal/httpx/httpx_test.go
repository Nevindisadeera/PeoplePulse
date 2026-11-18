package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog redirects the default logger for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func TestFailHidesDetailsOfUnexpectedErrors(t *testing.T) {
	log := captureLog(t)
	rec := httptest.NewRecorder()
	Fail(rec, httptest.NewRequest("GET", "/x", nil), errors.New("connection string mongodb://secret"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") || !strings.Contains(rec.Body.String(), `"code":"INTERNAL"`) {
		t.Errorf("response leaked details or lacks the generic code: %s", rec.Body)
	}
	if !strings.Contains(log.String(), "secret") {
		t.Error("the real error should be in the server log")
	}
}

func TestFailWritesKnownErrorsAsGiven(t *testing.T) {
	log := captureLog(t)
	rec := httptest.NewRecorder()
	Fail(rec, httptest.NewRequest("GET", "/x", nil), fmt.Errorf("wrapped: %w", Invalid(map[string]string{"email": "is required"})))

	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"email":"is required"`) {
		t.Errorf("status %d body %s", rec.Code, rec.Body)
	}
	if log.Len() != 0 {
		t.Errorf("a validation error should not be logged as a server error: %s", log)
	}
}

func TestFailTreatsClientDisconnectAsNotAnError(t *testing.T) {
	// A database call interrupted because the browser closed the connection.
	driverErr := fmt.Errorf("incomplete read of message header: %w", context.Canceled)

	for name, req := range map[string]*http.Request{
		"request context cancelled": func() *http.Request {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return httptest.NewRequest("GET", "/x", nil).WithContext(ctx)
		}(),
		"error wraps context.Canceled": httptest.NewRequest("GET", "/x", nil),
	} {
		log := captureLog(t)
		rec := httptest.NewRecorder()
		Fail(rec, req, driverErr)
		if rec.Code != StatusClientClosedRequest || rec.Body.Len() != 0 {
			t.Errorf("%s: status %d body %q, want 499 with no body", name, rec.Code, rec.Body)
		}
		if log.Len() != 0 {
			t.Errorf("%s: logged as a server error: %s", name, log)
		}
	}

	// A known error is still written normally even if the client has gone.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	Fail(rec, httptest.NewRequest("GET", "/x", nil).WithContext(ctx), NotFound("Employee"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("known error with cancelled context: status %d, want 404", rec.Code)
	}
}
