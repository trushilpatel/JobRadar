package store

import "context"

// ReserveResult reports the outcome of a rate-limit reservation attempt.
type ReserveResult int

const (
	// ReserveOK means the reservation was granted.
	ReserveOK ReserveResult = iota
	// ReserveDeniedRPM means the RPM (requests per minute) cap was hit.
	ReserveDeniedRPM
	// ReserveDeniedRPD means the RPD (requests per day, UTC) cap was hit.
	ReserveDeniedRPD
	// ReserveDeniedTPM means the TPM (input tokens per minute) cap was hit.
	ReserveDeniedTPM
)

// ReserveReq captures the parameters of a single reservation attempt.
type ReserveReq struct {
	SlotID string
	ReqID  string
	RPM    int
	RPD    int
	TPMIn  int
	Tokens int // estimated input tokens
}

// Usage is a snapshot of current consumption for a slot.
type Usage struct {
	RPM int
	RPD int
	TPM int
}

// LLMRateLimitStore tracks request/token usage per slot and arbitrates
// whether a new call may proceed.
type LLMRateLimitStore interface {
	Reserve(ctx context.Context, req ReserveReq) (ReserveResult, error)
	Release(ctx context.Context, slotID, reqID string) error
	Usage(ctx context.Context, slotID string) (Usage, error)
}
