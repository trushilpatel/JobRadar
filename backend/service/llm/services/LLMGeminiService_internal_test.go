package services

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"google.golang.org/genai"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
)

func TestIsGemini429(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"other error", errors.New("boom"), false},
		{"api error 500", genai.APIError{Code: http.StatusInternalServerError}, false},
		{"api error 429 value", genai.APIError{Code: http.StatusTooManyRequests}, true},
		{"api error 429 wrapped", fmt.Errorf("wrap: %w", genai.APIError{Code: http.StatusTooManyRequests}), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGemini429(tc.err); got != tc.want {
				t.Fatalf("isGemini429(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsGemini429MappedToSentinel(t *testing.T) {
	// Simulate the mapping done by generate().
	src := genai.APIError{Code: http.StatusTooManyRequests, Message: "rate limit"}
	mapped := fmt.Errorf("%w: %s", llmerr.ErrLLMProviderRateLimited, src.Error())
	if !errors.Is(mapped, llmerr.ErrLLMProviderRateLimited) {
		t.Fatalf("mapped error should unwrap to ErrLLMProviderRateLimited: %v", mapped)
	}
}
