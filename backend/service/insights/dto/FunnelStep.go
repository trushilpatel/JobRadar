package dto

// FunnelStep holds a single stage in the enrichment → action funnel.
type FunnelStep struct {
	Stage string `json:"stage"`
	Count int    `json:"count"`
}
