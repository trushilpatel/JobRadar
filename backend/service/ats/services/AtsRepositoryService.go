package services

import (
	"context"
	"errors"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atsmodel "github.com/trushilpatel/jobradar/service/ats/models"
	"gorm.io/gorm"
)

// AtsStore is the data-access contract for the ats table. Defined here so
// the repository implementation stays testable via a fake store without
// a real DB connection.
type AtsStore interface {
	ListAll(ctx context.Context) ([]atsmodel.ATS, error)
	GetByName(ctx context.Context, name string) (atsmodel.ATS, error)
	Update(ctx context.Context, name string, fields map[string]any) (atsmodel.ATS, error)
}

// atsStore is the GORM implementation of AtsStore.
type atsStore struct {
	db *gorm.DB
}

// NewAtsStore constructs the GORM store. Exported so test packages can
// instantiate it with an in-memory SQLite DB.
func NewAtsStore(db *gorm.DB) AtsStore {
	return &atsStore{db: db}
}

func (s *atsStore) ListAll(ctx context.Context) ([]atsmodel.ATS, error) {
	var rows []atsmodel.ATS
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *atsStore) GetByName(ctx context.Context, name string) (atsmodel.ATS, error) {
	var row atsmodel.ATS
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error; err != nil {
		return atsmodel.ATS{}, err
	}
	return row, nil
}

// Update applies fields to the named row and reads it back. It checks
// RowsAffected so a silent no-op (row does not exist) is surfaced as
// gorm.ErrRecordNotFound rather than silently returning an empty struct.
func (s *atsStore) Update(ctx context.Context, name string, fields map[string]any) (atsmodel.ATS, error) {
	fields["updated_at"] = time.Now().UTC()
	res := s.db.WithContext(ctx).Model(&atsmodel.ATS{}).Where("name = ?", name).Updates(fields)
	if res.Error != nil {
		return atsmodel.ATS{}, res.Error
	}
	if res.RowsAffected == 0 {
		return atsmodel.ATS{}, gorm.ErrRecordNotFound
	}
	return s.GetByName(ctx, name)
}

// atsRepositoryService is the terminal ats-domain layer.
type atsRepositoryService struct {
	store AtsStore
}

// NewAtsRepositoryService constructs the terminal AtsService implementation.
// Returning the interface keeps higher layers dependency-inverted at compile time.
func NewAtsRepositoryService(store AtsStore) AtsService {
	return &atsRepositoryService{store: store}
}

func (r *atsRepositoryService) ListATS(ctx context.Context) (_ []atsdto.ATSDTO, err error) {
	ctx, span := tracing.Start(ctx, "ats.store.ListAll")
	defer tracing.End(span, &err)

	rows, err := r.store.ListAll(ctx)
	if err != nil {
		return nil, domainerrs.ErrInternal.WithMessage("ats.repository.ListATS: " + err.Error())
	}
	dtos := make([]atsdto.ATSDTO, len(rows))
	for i, row := range rows {
		dtos[i] = toATSDTO(row)
	}
	return dtos, nil
}

func (r *atsRepositoryService) GetATS(ctx context.Context, name string) (_ atsdto.ATSDTO, err error) {
	ctx, span := tracing.Start(ctx, "ats.store.GetByName")
	defer tracing.End(span, &err)

	row, err := r.store.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return atsdto.ATSDTO{}, domainerrs.ErrNotFound.WithMessage("ats.repository.GetATS: " + name + " not found")
		}
		return atsdto.ATSDTO{}, domainerrs.ErrInternal.WithMessage("ats.repository.GetATS: " + err.Error())
	}
	return toATSDTO(row), nil
}

func (r *atsRepositoryService) UpdateATS(ctx context.Context, name string, input atsdto.UpdateATSInput) (_ atsdto.ATSDTO, err error) {
	ctx, span := tracing.Start(ctx, "ats.store.Update")
	defer tracing.End(span, &err)

	fields := updateInputToFields(input)
	row, err := r.store.Update(ctx, name, fields)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return atsdto.ATSDTO{}, domainerrs.ErrNotFound.WithMessage("ats.repository.UpdateATS: " + name + " not found")
		}
		return atsdto.ATSDTO{}, domainerrs.ErrInternal.WithMessage("ats.repository.UpdateATS: " + err.Error())
	}
	return toATSDTO(row), nil
}

// updateInputToFields converts an UpdateATSInput into a GORM update map,
// including only fields that were explicitly set (non-nil pointers).
func updateInputToFields(input atsdto.UpdateATSInput) map[string]any {
	fields := make(map[string]any)
	if input.ScanIntervalMin != nil {
		fields["scan_interval_min"] = *input.ScanIntervalMin
	}
	if input.InterCompanyDelayS != nil {
		fields["inter_company_delay_s"] = *input.InterCompanyDelayS
	}
	if input.IsActive != nil {
		fields["is_active"] = *input.IsActive
	}
	if input.IsAdminCreatable != nil {
		fields["is_admin_creatable"] = *input.IsAdminCreatable
	}
	return fields
}

// toATSDTO converts a GORM model to the service DTO.
func toATSDTO(m atsmodel.ATS) atsdto.ATSDTO {
	return atsdto.ATSDTO{
		Name:               m.Name,
		DisplayName:        m.DisplayName,
		BaseURL:            m.BaseURL,
		IsActive:           m.IsActive,
		IsAdminCreatable:   m.IsAdminCreatable,
		ScanIntervalMin:    m.ScanIntervalMin,
		InterCompanyDelayS: m.InterCompanyDelayS,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

// Compile-time assertion: atsRepositoryService implements AtsService.
var _ AtsService = (*atsRepositoryService)(nil)
