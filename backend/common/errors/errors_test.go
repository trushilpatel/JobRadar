package errors_test

import (
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
)

func TestValidationError_Error(t *testing.T) {
	v := &domainerrs.ValidationError{Field: "email", Code: "INVALID", Message: "must be lowercase"}
	got := v.Error()
	if !strings.Contains(got, "email") || !strings.Contains(got, "must be lowercase") {
		t.Fatalf("expected error to mention field and message, got %q", got)
	}
}

func TestValidationErrors_Error(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var v domainerrs.ValidationErrors
		if v.Error() != "" {
			t.Fatalf("empty ValidationErrors should render to empty string, got %q", v.Error())
		}
	})
	t.Run("single", func(t *testing.T) {
		v := domainerrs.ValidationErrors{{Field: "name", Message: "required"}}
		if v.Error() != "name: required" {
			t.Fatalf("unexpected single rendering: %q", v.Error())
		}
	})
	t.Run("multiple", func(t *testing.T) {
		v := domainerrs.ValidationErrors{
			{Field: "name", Message: "required"},
			{Field: "email", Message: "invalid"},
		}
		got := v.Error()
		if got != "name: required; email: invalid" {
			t.Fatalf("unexpected multi rendering: %q", got)
		}
	})
}

func TestServiceError_Error_InternalPriority(t *testing.T) {
	e := &domainerrs.ServiceError{Code: "X", Status: 500, InternalMessage: "boom", ExternalMessage: "polite"}
	if e.Error() != "boom" {
		t.Fatalf("Error() should return internal when set, got %q", e.Error())
	}
}

func TestServiceError_Error_FallsBackToExternal(t *testing.T) {
	e := &domainerrs.ServiceError{Code: "X", Status: 500, ExternalMessage: "polite"}
	if e.Error() != "polite" {
		t.Fatalf("Error() should fall back to external when internal empty, got %q", e.Error())
	}
}

func TestServiceError_Error_EmptyWhenBothEmpty(t *testing.T) {
	e := &domainerrs.ServiceError{Code: "X", Status: 500}
	if e.Error() != "" {
		t.Fatalf("Error() should return empty when both messages empty, got %q", e.Error())
	}
}

func TestServiceError_PublicMessage(t *testing.T) {
	t.Run("explicit external", func(t *testing.T) {
		e := &domainerrs.ServiceError{Code: "X", ExternalMessage: "all good"}
		if e.PublicMessage() != "all good" {
			t.Fatalf("expected explicit external message, got %q", e.PublicMessage())
		}
	})
	t.Run("fallback to code", func(t *testing.T) {
		e := &domainerrs.ServiceError{Code: "NOT_FOUND"}
		if e.PublicMessage() != "resource not found" {
			t.Fatalf("expected code-derived message, got %q", e.PublicMessage())
		}
	})
}

func TestServiceError_WithMessage(t *testing.T) {
	base := domainerrs.ErrNotFound
	wrapped := base.WithMessage("job 42")
	if wrapped == base {
		t.Fatalf("WithMessage must return a copy, not the same pointer")
	}
	if wrapped.Code != base.Code || wrapped.Status != base.Status {
		t.Fatalf("WithMessage must preserve Code and Status; got %+v", wrapped)
	}
	if wrapped.InternalMessage != "job 42" {
		t.Fatalf("WithMessage must set internal message, got %q", wrapped.InternalMessage)
	}
	if base.InternalMessage != "" {
		t.Fatalf("WithMessage must not mutate the sentinel; sentinel now %+v", base)
	}
}

func TestServiceError_WithExternal(t *testing.T) {
	base := domainerrs.ErrInternal.WithMessage("redis down")
	wrapped := base.WithExternal("temporarily unavailable")
	if wrapped == base {
		t.Fatalf("WithExternal must return a copy")
	}
	if wrapped.InternalMessage != "redis down" {
		t.Fatalf("WithExternal must preserve internal message; got %q", wrapped.InternalMessage)
	}
	if wrapped.ExternalMessage != "temporarily unavailable" {
		t.Fatalf("WithExternal must set external; got %q", wrapped.ExternalMessage)
	}
}

func TestPublicMessage_GenericForEveryKnownCode(t *testing.T) {
	cases := map[string]string{
		"NOT_FOUND":        "resource not found",
		"CONFLICT":         "resource already exists",
		"BAD_REQUEST":      "invalid request",
		"INTERNAL":         "internal server error",
		"LLM_VALIDATION":   "request validation failed",
		"LLM_LOCAL_QUOTA":  "rate limited",
		"LLM_PROVIDER_429": "rate limited",
		"LLM_EXHAUSTED":    "service temporarily unavailable",
		"UNKNOWN":          "error",
		"":                 "error",
	}
	for code, want := range cases {
		t.Run(code, func(t *testing.T) {
			got := (&domainerrs.ServiceError{Code: code}).PublicMessage()
			if got != want {
				t.Fatalf("code %q: expected %q, got %q", code, want, got)
			}
		})
	}
}

func TestSentinels_AllSet(t *testing.T) {
	cases := []struct {
		err  *domainerrs.ServiceError
		code string
		stat int
	}{
		{domainerrs.ErrNotFound, "NOT_FOUND", 404},
		{domainerrs.ErrConflict, "CONFLICT", 409},
		{domainerrs.ErrBadRequest, "BAD_REQUEST", 400},
		{domainerrs.ErrInternal, "INTERNAL", 500},
		{domainerrs.ErrLLMValidation, "LLM_VALIDATION", 400},
		{domainerrs.ErrLLMLocalQuotaExceeded, "LLM_LOCAL_QUOTA", 429},
		{domainerrs.ErrLLMProviderRateLimited, "LLM_PROVIDER_429", 429},
		{domainerrs.ErrLLMAllExhausted, "LLM_EXHAUSTED", 503},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			if c.err.Code != c.code {
				t.Fatalf("code: want %q, got %q", c.code, c.err.Code)
			}
			if c.err.Status != c.stat {
				t.Fatalf("status: want %d, got %d", c.stat, c.err.Status)
			}
		})
	}
}

func TestLLMAllExhaustedError_ErrorAndUnwrap(t *testing.T) {
	e := &domainerrs.LLMAllExhaustedError{
		Slots:      []domainerrs.LLMSlotStatus{{SlotID: "s1", Model: "m", LastDenial: "rpm", FailureCount: 1, CooldownUntil: time.Now()}},
		RetryAfter: time.Now().Add(time.Minute),
	}
	if e.Error() != domainerrs.ErrLLMAllExhausted.InternalMessage {
		t.Fatalf("Error() should mirror sentinel internal message, got %q", e.Error())
	}
	if !stderrors.Is(e, domainerrs.ErrLLMAllExhausted) {
		t.Fatalf("errors.Is should match sentinel via Unwrap")
	}
}

func TestIsNotFound(t *testing.T) {
	t.Run("gorm record not found", func(t *testing.T) {
		if !domainerrs.IsNotFound(gorm.ErrRecordNotFound) {
			t.Fatalf("expected IsNotFound(gorm.ErrRecordNotFound) to be true")
		}
	})
	t.Run("wrapped gorm record not found", func(t *testing.T) {
		wrapped := stderrors.Join(gorm.ErrRecordNotFound, stderrors.New("ctx"))
		if !domainerrs.IsNotFound(wrapped) {
			t.Fatalf("expected IsNotFound on wrapped error to be true")
		}
	})
	t.Run("other errors", func(t *testing.T) {
		if domainerrs.IsNotFound(stderrors.New("nope")) {
			t.Fatalf("expected IsNotFound(stderrors.New) to be false")
		}
		if domainerrs.IsNotFound(nil) {
			t.Fatalf("expected IsNotFound(nil) to be false")
		}
	})
}
