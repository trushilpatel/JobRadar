package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimit_AllowsSmallBody(t *testing.T) {
	mw := BodyLimit(64)
	var got []byte
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll: %v", err)
			return
		}
		got = b
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.Repeat("a", 32)
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200", w.Code)
	}
	if string(got) != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestBodyLimit_TruncatesOverLimit(t *testing.T) {
	mw := BodyLimit(16)
	var readErr error
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if readErr == nil {
		t.Fatal("expected read error from MaxBytesReader, got nil")
	}
	if !strings.Contains(readErr.Error(), "too large") {
		t.Fatalf("error = %q, want contains 'too large'", readErr.Error())
	}
}

func TestBodyLimit_ZeroLimitIsNoOp(t *testing.T) {
	mw := BodyLimit(0)
	var n int
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		n = len(b)
		w.WriteHeader(http.StatusOK)
	}))
	body := strings.Repeat("a", 1024)
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if n != 1024 {
		t.Fatalf("read %d bytes with zero limit, expected 1024 (no-op)", n)
	}
}

func TestBodyLimit_NilBodyNoPanic(t *testing.T) {
	mw := BodyLimit(16)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Body = nil
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200", w.Code)
	}
}
