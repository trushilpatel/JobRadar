package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	scancrawlercommon "github.com/trushilpatel/jobradar/service/scan/common"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"

	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
)

// scanLogRepositoryService implements ScanLogStore by delegating to the
// existing scan.ScanLogWriter interface.
// Terminal — no next, no BaseNodeService.
type scanLogRepositoryService struct {
	inner scansvc.ScanLogWriter
}

// NewScanLogRepositoryService constructs a ScanLogStore backed by the existing
// scan log writer (scan.ScanLogRepositoryService).
func NewScanLogRepositoryService(inner scansvc.ScanLogWriter) ScanLogStore {
	return &scanLogRepositoryService{inner: inner}
}

func (s *scanLogRepositoryService) RecordScanResult(ctx context.Context, companyID string, r dto.ScanResult) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.RecordScanResult")
	defer tracing.End(span, &err)

	status := scancrawlercommon.ScanStatusOK
	var errMsg *string
	if r.ErrMsg != "" {
		status = scancrawlercommon.ScanStatusError
		msg := r.ErrMsg
		errMsg = &msg
	}

	if err := s.inner.RecordScanResult(
		ctx,
		companyID,
		status,
		r.FetchedCount,
		r.NewCount,
		int(r.DurationMs),
		errMsg,
	); err != nil {
		return errors.ErrInternal.WithMessage("record scan result: " + err.Error())
	}
	return nil
}
