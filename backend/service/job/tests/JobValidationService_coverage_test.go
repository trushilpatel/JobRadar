package services_test

import (
	"context"
	"strings"
	"testing"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
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

func TestValidate_Dismiss_NoteTooLong(t *testing.T) {
	v := newValidator()
	if err := v.Dismiss(context.Background(), "j1", jobcommon.DismissalReasonNotRelevant, strings.Repeat("a", 5001)); err == nil {
		t.Fatal("Dismiss note >5000 must return error")
	}
}

func TestValidate_BulkDismiss_NoteTooLong(t *testing.T) {
	v := newValidator()
	if err := v.BulkDismiss(context.Background(), []string{"j1"}, jobcommon.DismissalReasonNotRelevant, strings.Repeat("a", 5001)); err == nil {
		t.Fatal("BulkDismiss note >5000 must return error")
	}
}
