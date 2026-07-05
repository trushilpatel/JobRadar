package services

// implService is currently a pure pass-through. The apply-queue domain has
// no cross-cutting orchestration beyond what RepositoryService already
// does: the existence/ownership checks live in ValidationService (via the
// EngagementChecker / ResumeOwnershipChecker narrow interfaces), and List /
// SetResume need no caching, no analytics side effects, and no cross-domain
// writes. The layer is kept explicit (rather than skipped) so the decorator
// chain stays uniform with every other slice, and so a future orchestration
// need (e.g. invalidating a cached queue page after SetResume) has an
// obvious home without reshaping the chain.
type implService struct{ BaseService }

// NewImplService constructs the terminal-adjacent business-logic layer.
func NewImplService(next Service) Service {
	return &implService{BaseService: NewBaseService(next)}
}

var _ Service = (*implService)(nil)
