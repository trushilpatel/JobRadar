package google

// Config holds the Google scraper settings sourced from the central config.
// The crawler is a pure listing scanner — detail-page fetching and LLM
// enrichment are handled downstream by the analyzer.
type Config struct {
	MinDelayMs int // min delay between listing-page fetches in ms
	MaxDelayMs int // max delay between listing-page fetches in ms
	TimeoutMs  int // overall scraper timeout in ms
	MaxPages   int // max pages to paginate, 0 = all
}

// defaultConfig returns sensible production defaults.
//
// Listing pacing: 20-40 s — HTTP is far less detectable than a browser;
// safe across the ~38 listing pages of the filtered search.
func defaultConfig() Config {
	return Config{
		MinDelayMs: 20000,   // 20 s between listing pages
		MaxDelayMs: 40000,   // 40 s between listing pages
		TimeoutMs:  7200000, // 2 hours — full scan budget
		MaxPages:   0,
	}
}
