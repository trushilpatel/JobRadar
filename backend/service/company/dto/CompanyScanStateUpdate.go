package dto

import "time"

// CompanyScanStateUpdate is the worker-only write payload for the result of
// a scan attempt. Workers call ScanSettingsRepository.UpdateScanState after
// each scan finishes — admins never write this shape, so it lives off the
// admin input path.
//
// All fields are optional pointers: a nil field is left untouched. The
// canonical patterns:
//
//   - successful scan: LastScanAt + LastSuccessAt + ConsecutiveErrors=0
//   - failed scan:     LastScanAt + LastErrorAt + LastErrorMessage +
//     ConsecutiveErrors=current+1 (writer computes)
type CompanyScanStateUpdate struct {
	LastScanAt        *time.Time
	LastSuccessAt     *time.Time
	LastErrorAt       *time.Time
	LastErrorMessage  *string
	ConsecutiveErrors *int
}
