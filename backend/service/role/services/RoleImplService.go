package services

// implService is the business-logic layer of the role chain. Phase 1 owns
// no orchestration — Grant and Revoke are direct DB writes, and admin
// promotion is performed by the auth middleware via Grant directly. The
// layer exists so future phases can wrap audit / cache around grants
// without touching the repository.
type implService struct {
	BaseService
}

// NewImplService constructs the orchestration layer.
func NewImplService(next Service) Service {
	return &implService{BaseService: NewBaseService(next)}
}

var _ Service = (*implService)(nil)
