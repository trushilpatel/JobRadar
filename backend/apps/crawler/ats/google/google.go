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
		MinDelayMs:  55000,   // ~1 min between listing pages (human pacing, bot-safe)
		MaxDelayMs:  75000,   // ~1.25 min between listing pages
		TimeoutMs:   18000000, // 5 hours — enough for initial full scan of ~680 jobs
		MaxPages:    0,
		EnrichMinMs: 5000,  // 5s between detail pages
		EnrichMaxMs: 10000, // 10s between detail pages
	}
}
