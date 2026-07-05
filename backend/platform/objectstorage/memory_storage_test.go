package objectstorage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestMemoryStorage_PutThenHasReturnsTrue(t *testing.T) {
	m := NewMemoryStorage()
	err := m.Put(context.Background(), "resumes/u1/r1.pdf", strings.NewReader("data"), 4, "application/pdf")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !m.Has("resumes/u1/r1.pdf") {
		t.Fatal("expected key to exist after Put")
	}
}

func TestMemoryStorage_PutPropagatesReadError(t *testing.T) {
	m := NewMemoryStorage()
	if err := m.Put(context.Background(), "k", erroringReader{}, 0, "application/pdf"); err == nil {
		t.Fatal("expected an error when the reader fails")
	}
}

func TestMemoryStorage_DeleteRemovesKey(t *testing.T) {
	m := NewMemoryStorage()
	_ = m.Put(context.Background(), "k", strings.NewReader("d"), 1, "application/pdf")
	if err := m.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if m.Has("k") {
		t.Fatal("expected key to be gone after Delete")
	}
}

func TestMemoryStorage_DeleteMissingKeyIsNotAnError(t *testing.T) {
	m := NewMemoryStorage()
	if err := m.Delete(context.Background(), "never-existed"); err != nil {
		t.Fatalf("Delete on missing key should not error, got: %v", err)
	}
}

func TestMemoryStorage_PresignGetReturnsDeterministicURL(t *testing.T) {
	m := NewMemoryStorage()
	url, err := m.PresignGet(context.Background(), "k", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if !strings.Contains(url, "k") {
		t.Fatalf("expected URL to contain the key, got %s", url)
	}
}
