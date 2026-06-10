package errors

import "errors"

var (
	// ErrNodeUnauthorized is returned when a bearer token is missing or does not
	// match any active crawler node.
	ErrNodeUnauthorized = errors.New("crawler node: unauthorized")

	// ErrNodeDisabled is returned when the authenticating node exists but its
	// status is disabled or retired.
	ErrNodeDisabled = errors.New("crawler node: disabled")

	// ErrAssignmentNotFound is returned when a poll request finds no pending
	// work assigned to the given node.
	ErrAssignmentNotFound = errors.New("crawler node: assignment not found")

	// ErrAssignmentMismatch is returned when an assignment exists but does not
	// belong to the node submitting results or an error report.
	ErrAssignmentMismatch = errors.New("crawler node: assignment mismatch")

	// ErrInvalidAdapter is returned when an adapter type string does not map to
	// any registered adapter in the registry.
	ErrInvalidAdapter = errors.New("crawler node: invalid adapter")
)
