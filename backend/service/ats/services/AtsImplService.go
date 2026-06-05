package services

// implService is the business-logic layer of the ats chain. Currently there
// is no orchestration beyond what the repository provides, so all three
// AtsService methods auto-delegate to next via BaseAtsService. Future
// business rules (e.g. cross-ATS validation, audit events) land here without
// touching the repository.
type implService struct {
	BaseAtsService
}

// NewAtsImplService constructs the orchestration layer.
func NewAtsImplService(next AtsService) AtsService {
	return &implService{BaseAtsService: NewBaseAtsService(next)}
}

// Compile-time assertion: implService implements AtsService.
var _ AtsService = (*implService)(nil)
