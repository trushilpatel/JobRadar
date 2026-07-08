package services_test

import (
	"context"
	"strings"
	"testing"
)

func TestValidate_SetNotes_TooLong(t *testing.T) {
	v := newValidator()
	if err := v.SetNotes(context.Background(), "j1", strings.Repeat("a", 5001)); err == nil {
		t.Fatal("SetNotes >5000 chars must return error")
	}
}

func TestValidate_SetNotes_Valid(t *testing.T) {
	v := newValidator()
	if err := v.SetNotes(context.Background(), "j1", strings.Repeat("a", 5000)); err != nil {
		t.Fatalf("SetNotes 5000 chars: %v", err)
	}
}
