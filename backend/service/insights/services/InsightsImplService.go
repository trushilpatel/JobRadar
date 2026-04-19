package services

// implService embeds BaseService and adds no extra logic currently.
// It is kept as a layer for future business logic without modifying the chain.
type implService struct {
	BaseService
}

// NewImplService constructs an InsightsService that delegates everything to next.
func NewImplService(next InsightsService) InsightsService {
	return &implService{BaseService: NewBaseService(next)}
}
