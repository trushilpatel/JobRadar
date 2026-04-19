package dto

// PipelineCounts holds the current depth of each pipeline stage.
type PipelineCounts struct {
	Pending int64 `json:"pending"`
	Errors  int64 `json:"errors"`
}
