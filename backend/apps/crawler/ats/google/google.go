package google

// Config holds the Google scraper settings sourced from the central config.
//
// Pacing is split between listing-page fetches (slow, bot-safe) and
// detail-page fetches (faster — there are 750+ detail pages per scan and the
// inter-page delay there dominates total scan time).
type Config struct {
	MinDelayMs    int // min delay between listing-page fetches in ms
	MaxDelayMs    int // max delay between listing-page fetches in ms
	TimeoutMs     int // overall scraper timeout in ms
	MaxPages      int // max pages to paginate, 0 = all
	EnrichMinMs   int // min delay between detail-page fetches in ms
	EnrichMaxMs   int // max delay between detail-page fetches in ms (0 = skip enrichment entirely)
	MaxDetailJobs int // max detail pages to fetch, 0 = all newly-seen jobs
}

// defaultConfig returns sensible production defaults.
//
// Listing pacing: ~100-120 s — well-spaced GETs to avoid Google rate limiting
// across the ~38 listing pages of the filtered search.
// Detail pacing: 5-10 s — kept short so a 750-job initial scan completes
// inside the overall TimeoutMs budget.
func defaultConfig() Config {
	return Config{
		MinDelayMs:  100000,   // 100 s between listing pages
		MaxDelayMs:  120000,   // 120 s between listing pages
		TimeoutMs:   18000000, // 5 hours — full initial scan budget
		MaxPages:    0,
		EnrichMinMs: 5000,  // 5 s between detail pages
		EnrichMaxMs: 10000, // 10 s between detail pages
	}
}
