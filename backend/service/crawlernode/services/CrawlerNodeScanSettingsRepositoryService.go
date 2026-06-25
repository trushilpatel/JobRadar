package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"gorm.io/gorm"
)

var _ ScanSettingsReader = (*scanSettingsRepositoryService)(nil)

// scanSettingsRow is the minimal DB projection of company_scan_settings.
type scanSettingsRow struct {
	AdapterConfig *string `gorm:"column:adapter_config"`
}

type scanSettingsRepositoryService struct {
	db *gorm.DB
}

// NewScanSettingsReader constructs a ScanSettingsReader backed by the
// company_scan_settings table. Returns nil adapter_config when none is set.
func NewScanSettingsReader(db *gorm.DB) ScanSettingsReader {
	return &scanSettingsRepositoryService{db: db}
}

func (s *scanSettingsRepositoryService) GetAdapterConfig(ctx context.Context, companyID string) (_ *string, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.scan_settings.GetAdapterConfig")
	defer tracing.End(span, &err)

	var row scanSettingsRow
	result := s.db.WithContext(ctx).
		Table("company_scan_settings").
		Select("adapter_config").
		Where("company_id = ?", companyID).
		First(&row)

	if result.Error != nil {
		if errors.IsNotFound(result.Error) {
			return nil, nil
		}
		return nil, errors.ErrInternal.WithMessage("GetAdapterConfig: " + result.Error.Error())
	}
	return row.AdapterConfig, nil
}
