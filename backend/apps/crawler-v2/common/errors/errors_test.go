package errors_test

import (
	"errors"
	"fmt"
	"testing"

	crawlerrors "github.com/trushilpatel/jobradar/apps/crawler-v2/common/errors"
)

func TestSentinels_AreDistinct(t *testing.T) {
	sentinels := []error{
		crawlerrors.ErrNodeUnauthorized,
		crawlerrors.ErrNodeDisabled,
		crawlerrors.ErrAssignmentNotFound,
		crawlerrors.ErrAssignmentMismatch,
		crawlerrors.ErrInvalidAdapter,
	}
	for i, a := range sentinels {
		if a == nil {
			t.Errorf("sentinel[%d] is nil", i)
			continue
		}
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("sentinel[%d] and sentinel[%d] are the same: %v == %v", i, j, a, b)
			}
		}
	}
}

func TestSentinels_WorkWithErrorsIs(t *testing.T) {
	cases := []struct {
		name     string
		sentinel error
		wrapped  error
	}{
		{"ErrNodeUnauthorized", crawlerrors.ErrNodeUnauthorized, fmt.Errorf("node check: %w", crawlerrors.ErrNodeUnauthorized)},
		{"ErrNodeDisabled", crawlerrors.ErrNodeDisabled, fmt.Errorf("node check: %w", crawlerrors.ErrNodeDisabled)},
		{"ErrAssignmentNotFound", crawlerrors.ErrAssignmentNotFound, fmt.Errorf("poll: %w", crawlerrors.ErrAssignmentNotFound)},
		{"ErrAssignmentMismatch", crawlerrors.ErrAssignmentMismatch, fmt.Errorf("complete: %w", crawlerrors.ErrAssignmentMismatch)},
		{"ErrInvalidAdapter", crawlerrors.ErrInvalidAdapter, fmt.Errorf("executor: %w", crawlerrors.ErrInvalidAdapter)},
	}
	for _, tc := range cases {
		if !errors.Is(tc.wrapped, tc.sentinel) {
			t.Errorf("%s: errors.Is(wrapped, sentinel) = false, want true", tc.name)
		}
	}
}
