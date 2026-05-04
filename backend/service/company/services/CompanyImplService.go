package services

// publicImplService is a pass-through CompanyService decorator. It exists
// so the public chain has a consistent business-logic insertion point — if
// a future feature needs cross-cutting public read logic (e.g. fan-in to
// scan_history aggregation, projection masking), it goes here without
// touching the cache or repository layers.
type publicImplService struct {
	BasePublicService
}

// NewImplService wraps next as a pure pass-through. All CompanyService
// methods auto-delegate via BasePublicService.
func NewImplService(next CompanyService) CompanyService {
	return &publicImplService{BasePublicService: NewBasePublicService(next)}
}

// Compile-time interface assertion to keep the file honest as the
// interface evolves.
var _ CompanyService = (*publicImplService)(nil)
