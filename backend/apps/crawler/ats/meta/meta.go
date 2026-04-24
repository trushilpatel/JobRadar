package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
)

// Config holds the Meta scraper settings sourced from the central config.
type Config struct {
	MinDelayMs int // min delay between page navigations in ms
	MaxDelayMs int // max delay between page navigations in ms
	TimeoutMs  int // overall scraper timeout in ms
	MaxJobs    int // max jobs to fetch, 0 = all
}

// defaultConfig returns sensible production defaults.
func defaultConfig() Config {
	return Config{
		MinDelayMs: 3000,    // 3s between jobs
		MaxDelayMs: 8000,    // 8s between jobs
		TimeoutMs:  3600000, // 1 hour total
		MaxJobs:    0,       // no limit
	}
}

// Meta implements types.Adapter by shelling out to a Node.js Playwright
// scraper that navigates metacareers.com/jobs/, scrolls to load all US jobs,
// then visits each job detail page to extract full content.
//
// This is a full-detail adapter: FullContent is populated at scan time by the
// scraper. The job_post_analyzer skips the HTTP fetch and goes straight to LLM
// enrichment. Jobs where the scraper timed out before visiting their detail page
// will have FullContent.Description == "" and fall back to HTTP fetch in the analyzer.
type Meta struct {
	cfg Config
	// scraperDir overrides the path to the scraper/ directory.
	// Leave empty for production (auto-detected relative to this file).
	scraperDir string
}

// New creates a Meta adapter with the given config.
func New(cfg Config) *Meta {
	return &Meta{cfg: cfg}
}

// NewDefault creates a Meta adapter with default settings.
// Used in tests and when no config is available.
func NewDefault() *Meta {
	return &Meta{cfg: defaultConfig()}
}

// CareersURL returns the Meta Careers jobs search page.
func (m *Meta) CareersURL(_ string) string {
	return "https://www.metacareers.com/jobsearch"
}

// ── Scraper output types ────────────────────────────────────────────────────

// scraperJob mirrors the JSON output from scrape.js.
type scraperJob struct {
	ID          string              `json:"id"` // "meta-123456"
	Title       string              `json:"title"`
	URL         string              `json:"url"`
	Location    string              `json:"location"`
	EmployType  string              `json:"employ_type"` // normalized: full_time, contract, etc.
	FullContent *scraperFullContent `json:"full_content,omitempty"`
}

// scraperFullContent mirrors the full_content object in the scraper JSON output.
// Fields map 1:1 to types.FullJobContent.
type scraperFullContent struct {
	Description   string `json:"description"`
	Requirements  string `json:"requirements"`
	Compensation  string `json:"compensation"`
	WorkplaceType string `json:"workplace_type"`
	Department    string `json:"department"`
	PublishedDate string `json:"published_date"`
}

// scraperError is returned when the scraper exits with an error payload.
type scraperError struct {
	Error string `json:"error"`
}

// ── Adapter implementation ──────────────────────────────────────────────────

// FetchJobs launches the Playwright scraper and returns all Meta job listings
// with full content populated by the scraper.
// The companyID parameter is unused since the scraper always targets metacareers.com.
func (m *Meta) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	scraperPath := m.resolveScraperDir()
	scriptPath := filepath.Join(scraperPath, "scrape.js")

	// Verify scraper exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return types.FetchJobsResponse{}, fmt.Errorf("meta: scraper not found at %s — run install.sh first", scriptPath)
	}

	// Run via xvfb-run for a virtual display so Chrome runs in headed mode.
	// Headed Chrome has a more realistic browser fingerprint than headless mode.
	// Falls back to plain node if xvfb-run is not installed (e.g. local dev).
	var cmd *exec.Cmd
	if _, err := exec.LookPath("xvfb-run"); err == nil {
		cmd = exec.CommandContext(ctx, "xvfb-run", "--auto-servernum", "--server-args=-screen 0 1366x768x24", "node", scriptPath)
	} else {
		cmd = exec.CommandContext(ctx, "node", scriptPath)
	}
	cmd.Dir = scraperPath

	// Pass config values directly to the Node process as environment variables.
	// Values come from the central Go config — no separate .env file needed.
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("META_SCRAPER_MIN_DELAY=%s", itoa(m.cfg.MinDelayMs)),
		fmt.Sprintf("META_SCRAPER_MAX_DELAY=%s", itoa(m.cfg.MaxDelayMs)),
		fmt.Sprintf("META_SCRAPER_TIMEOUT_MS=%s", itoa(m.cfg.TimeoutMs)),
		fmt.Sprintf("META_SCRAPER_MAX_JOBS=%s", itoa(m.cfg.MaxJobs)),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta: scraper failed: %w\nstderr: %s", err, stderr.String())
	}

	// Extract the result payload from stdout. Playwright/Chromium may write
	// diagnostic lines to fd 1 before the scraper's final JSON write, producing
	// NDJSON-like output. We scan backwards for the last non-empty line that
	// begins with '[' (jobs array) or '{' (error object) — the scraper always
	// writes its payload last and prefixes it with '\n' to guarantee isolation.
	raw := stdout.Bytes()
	payload, err := extractScraperPayload(raw)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta: failed to locate scraper payload: %w\nraw (first 500 bytes): %s", err, truncate(string(raw), 500))
	}

	// Check for error response
	var scraperErr scraperError
	if err := json.Unmarshal(payload, &scraperErr); err == nil && scraperErr.Error != "" {
		return types.FetchJobsResponse{}, fmt.Errorf("meta: scraper error: %s", scraperErr.Error)
	}

	// Parse jobs array
	var scraperJobs []scraperJob
	if err := json.Unmarshal(payload, &scraperJobs); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta: failed to parse scraper output: %w\nraw (first 500 bytes): %s", err, truncate(string(raw), 500))
	}

	// Convert to pipeline types. FullContent is set when the scraper visited the
	// detail page; nil when the scraper timed out before reaching that job.
	items := make([]types.FetchJobsItem, 0, len(scraperJobs))
	for _, sj := range scraperJobs {
		var fc *types.FullJobContent
		if sj.FullContent != nil {
			fc = &types.FullJobContent{
				Description:   sj.FullContent.Description,
				Requirements:  sj.FullContent.Requirements,
				Compensation:  sj.FullContent.Compensation,
				WorkplaceType: sj.FullContent.WorkplaceType,
				Department:    sj.FullContent.Department,
				PublishedDate: sj.FullContent.PublishedDate,
			}
		}
		items = append(items, types.FetchJobsItem{
			ID:             sj.ID,
			Title:          sj.Title,
			URL:            sj.URL,
			Location:       sj.Location,
			EmploymentType: sj.EmployType,
			FullContent:    fc,
		})
	}

	return types.FetchJobsResponse{Jobs: items}, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// resolveScraperDir returns the path to the scraper/ directory.
// Priority: scraperDir override → META_SCRAPER_DIR env → executable-relative path.
func (m *Meta) resolveScraperDir() string {
	if m.scraperDir != "" {
		return m.scraperDir
	}
	// Allow override via env var (useful in Docker / non-standard layouts)
	if dir := os.Getenv("META_SCRAPER_DIR"); dir != "" {
		return dir
	}
	// Default: scraper/ next to the running executable
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "scraper")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}
	// Last resort: relative to working directory
	return "scraper"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// extractScraperPayload finds the result JSON written by the scraper at the
// end of stdout. Playwright/Chromium may emit diagnostic lines to stdout
// before the scraper's final write, producing interleaved NDJSON. This
// function scans backwards through lines and returns the last non-empty line
// that starts with '[' (jobs array) or '{' (error object).
func extractScraperPayload(raw []byte) ([]byte, error) {
	lines := bytes.Split(bytes.TrimRight(raw, "\n"), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		if line[0] == '[' || line[0] == '{' {
			return line, nil
		}
	}
	return nil, fmt.Errorf("no JSON array or object found in scraper stdout")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Compile-time interface checks.
var (
	_ types.Adapter     = (*Meta)(nil)
	_ types.URLProvider = (*Meta)(nil)
)

// Note: Meta does NOT implement types.ContentFetcher. It is a full-detail
// adapter — the Playwright scraper populates FullContent at scan time so the
// job_post_analyzer skips the HTTP fetch entirely via the FullContent fast path.
