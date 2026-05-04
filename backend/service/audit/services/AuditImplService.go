package services

// implService is the business-logic layer of the audit chain. Phase 1 owns
// no orchestration — Append is a direct write — but the layer exists so
// future phases can wrap retention policy, redaction, or batched writes
// without touching the repository.
type implService struct {
	BaseService
}

// NewImplService constructs the orchestration layer.
func NewImplService(next Service) Service {
	return &implService{BaseService: NewBaseService(next)}
}

var _ Service = (*implService)(nil)
