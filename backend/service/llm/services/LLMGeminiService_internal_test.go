package services

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"google.golang.org/genai"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
)

func TestIsGeminiProviderError(t *testing.T) {
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
		{"api error 503 value", genai.APIError{Code: http.StatusServiceUnavailable}, true},
		{"api error 503 wrapped", fmt.Errorf("wrap: %w", genai.APIError{Code: http.StatusServiceUnavailable}), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGeminiProviderError(tc.err); got != tc.want {
				t.Fatalf("isGeminiProviderError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsGeminiProviderErrorMappedToSentinel(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		src := genai.APIError{Code: code, Message: "provider error"}
		mapped := fmt.Errorf("%w: %s", llmerr.ErrLLMProviderRateLimited, src.Error())
		if !errors.Is(mapped, llmerr.ErrLLMProviderRateLimited) {
			t.Fatalf("code %d: mapped error should unwrap to ErrLLMProviderRateLimited: %v", code, mapped)
		}
	}
}
