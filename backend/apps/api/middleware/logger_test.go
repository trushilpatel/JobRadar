package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/trushilpatel/jobradar/common/logger"
)

// spyLogger captures Info calls so logger middleware tests can assert which
// requests are emitted and which are intentionally skipped (ops endpoints).
type spyLogger struct {
	mu     sync.Mutex
	infos  []spyEvent
	warns  []spyEvent
	errors []spyEvent
	debugs []spyEvent
}

type spyEvent struct {
	msg    string
	fields []logger.Field
}

func (s *spyLogger) Debug(msg string, f ...logger.Field) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debugs = append(s.debugs, spyEvent{msg, f})
}

func (s *spyLogger) Info(msg string, f ...logger.Field) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.infos = append(s.infos, spyEvent{msg, f})
}

func (s *spyLogger) Warn(msg string, f ...logger.Field) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warns = append(s.warns, spyEvent{msg, f})
}

func (s *spyLogger) Error(msg string, _ error, f ...logger.Field) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors = append(s.errors, spyEvent{msg, f})
}

func (s *spyLogger) With(_ ...logger.Field) logger.Logger { return s }

func (s *spyLogger) infoCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.infos)
}

func TestLogger_LogsRegularRequest(t *testing.T) {
	sl := &spyLogger{}
	mw := Logger(sl)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/things", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if sl.infoCount() != 1 {
		t.Fatalf("expected 1 info log, got %d", sl.infoCount())
	}
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d want 201 (handler must still run)", rr.Code)
	}

	// Inspect the log fields — they must carry method/path/status, not raw strings.
	ev := sl.infos[0]
	if ev.msg != "request" {
		t.Errorf("log msg=%q want 'request'", ev.msg)
	}
	seenKeys := map[string]bool{}
	for _, f := range ev.fields {
		seenKeys[f.Key] = true
	}
	for _, want := range []string{logger.FieldMethod, logger.FieldPath, logger.FieldStatus, logger.FieldDuration} {
		if !seenKeys[want] {
			t.Errorf("log missing required field %q (have %v)", want, seenKeys)
		}
	}
}

func TestLogger_SkipsOpsEndpoints(t *testing.T) {
	for _, path := range []string{"/metrics", "/health", "/ready"} {
		t.Run(path, func(t *testing.T) {
			sl := &spyLogger{}
			mw := Logger(sl)
			h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if sl.infoCount() != 0 {
				t.Fatalf("%s must not be logged (got %d logs)", path, sl.infoCount())
			}
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d — handler must still run for %s", rr.Code, path)
			}
		})
	}
}

func TestLogger_DefaultStatusWhenHandlerSilent(t *testing.T) {
	// Handler that never calls WriteHeader — the wrapped writer defaults to 200.
	sl := &spyLogger{}
	mw := Logger(sl)
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// no-op
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if sl.infoCount() != 1 {
		t.Fatalf("want 1 log, got %d", sl.infoCount())
	}
	// Locate the status field — must be 200 by default.
	var statusVal int
	for _, f := range sl.infos[0].fields {
		if f.Key == logger.FieldStatus {
			if i, ok := f.Value.(int); ok {
				statusVal = i
			}
		}
	}
	if statusVal != http.StatusOK {
		t.Errorf("status field=%d want 200 default", statusVal)
	}
}

func TestResponseWriter_WriteHeaderCapturesCode(t *testing.T) {
	// The wrapped writer must remember the status the handler wrote AND
	// propagate to the underlying writer (otherwise the response is broken).
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
	rw.WriteHeader(http.StatusTeapot)
	if rw.status != http.StatusTeapot {
		t.Errorf("status=%d want 418 (captured)", rw.status)
	}
	if w.Code != http.StatusTeapot {
		t.Errorf("underlying code=%d want 418 (propagated)", w.Code)
	}
}
