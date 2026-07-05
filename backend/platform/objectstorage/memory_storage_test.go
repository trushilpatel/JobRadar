package objectstorage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
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

func TestMemoryStorage_GetReturnsStoredBytesAndSize(t *testing.T) {
	m := NewMemoryStorage()
	_ = m.Put(context.Background(), "k", strings.NewReader("hello"), 5, "application/pdf")

	r, size, err := m.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer r.Close()
	if size != 5 {
		t.Fatalf("expected size 5, got %d", size)
	}
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if string(data) != "hello" {
		t.Fatalf("expected %q, got %q", "hello", data)
	}
}

func TestMemoryStorage_GetMissingKeyReturnsErrNotFound(t *testing.T) {
	m := NewMemoryStorage()
	if _, _, err := m.Get(context.Background(), "never-existed"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
