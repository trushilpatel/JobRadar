package services

// implService is the business-logic layer of the pipeline chain. Currently
// there is no orchestration beyond what the repository provides, so both
// PipelineService methods auto-delegate to next via BasePipelineService.
// Future business rules (e.g. real per-tier NATS introspection) land here
// without touching the repository.
type implService struct {
	BasePipelineService
}

// NewPipelineImplService constructs the orchestration layer.
func NewPipelineImplService(next PipelineService) PipelineService {
	return &implService{BasePipelineService: NewBasePipelineService(next)}
}

// Compile-time assertion: implService implements PipelineService.
var _ PipelineService = (*implService)(nil)
