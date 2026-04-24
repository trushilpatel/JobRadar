package google

// Config holds the Google scraper settings sourced from the central config.
type Config struct {
	MinDelayMs    int    // min delay between page navigations in ms
	MaxDelayMs    int    // max delay between page navigations in ms
	TimeoutMs     int    // overall scraper timeout in ms
	MaxPages      int    // max pages to paginate, 0 = all
	EnrichMinMs   int    // min delay between detail page visits in ms
	EnrichMaxMs   int    // max delay between detail page visits in ms (0 = skip enrichment entirely)
	MaxDetailJobs int    // max detail pages to visit, 0 = all fetched jobs
	ProfileDir    string // persistent Chrome profile directory; empty = fresh profile each run
	ChromiumPath  string // explicit Chromium binary path; empty = auto-detect
}

// defaultConfig returns sensible production defaults.
func defaultConfig() Config {
	return Config{
		MinDelayMs:  15000,  // 15s between listing pages
		MaxDelayMs:  25000,  // 25s between listing pages
		TimeoutMs:   7200000, // 2 hours total
		MaxPages:    0,
		EnrichMinMs: 8000,  // 8s between detail pages
		EnrichMaxMs: 12000, // 12s between detail pages
	}
}
