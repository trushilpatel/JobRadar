package google

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// Config holds the Google scraper settings sourced from the central config.
type Config struct {
	MinDelayMs  int // min delay between page navigations in ms
	MaxDelayMs  int // max delay between page navigations in ms
	TimeoutMs   int // overall scraper timeout in ms
	MaxPages    int // max pages to paginate, 0 = all
	EnrichMinMs int // min delay between detail page visits in ms
	EnrichMaxMs int // max delay between detail page visits in ms
}

// defaultConfig returns sensible production defaults.
func defaultConfig() Config {
	return Config{
		MinDelayMs:  120000,   // 2 min between pages
		MaxDelayMs:  240000,   // 4 min between pages
		TimeoutMs:   14400000, // 4 hours total
		MaxPages:    0,
		EnrichMinMs: 60000,  // 1 min between detail pages
		EnrichMaxMs: 120000, // 2 min between detail pages
	}
}

// Google implements types.Adapter by shelling out to a Node.js Playwright
// scraper that navigates careers.google.com, paginates through all result pages,
// then visits each job detail page to extract full content.
//
// This is a full-detail adapter: FullContent is populated at scan time by the
// scraper. The job_post_analyzer skips the HTTP fetch and goes straight to LLM
// enrichment. Jobs where the scraper timed out before visiting their detail page
// will have FullContent.Description == "" and fall back to HTTP fetch in the analyzer.
type Google struct {
	cfg Config
	log logger.Logger
	// scraperDir overrides the path to the scraper/ directory.
	// Leave empty for production (auto-detected relative to this file).
	scraperDir string
}

// New creates a Google adapter with the given config and logger.
func New(cfg Config, log logger.Logger) *Google {
	return &Google{cfg: cfg, log: log}
}

// NewDefault creates a Google adapter with default settings and a no-op logger.
// Used in tests and when no config is available.
func NewDefault() *Google {
	return &Google{cfg: defaultConfig(), log: logger.NewNoop()}
}

// CareersURL returns the Google Careers jobs page with pre-set filters
// for US, Early+Mid experience, Full-time, Software Engineering.
func (g *Google) CareersURL(_ string) string {
	return "https://www.google.com/about/careers/applications/jobs/results?location=United%20States&target_level=EARLY&target_level=MID&employment_type=FULL_TIME&q=Software%20Engineering"
}

// ── Scraper output types ────────────────────────────────────────────────────

// scraperJob mirrors the JSON output from scrape.js.
type scraperJob struct {
	ID          string              `json:"id"` // "google-123456"
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

// FetchJobs launches the Playwright scraper and returns all Google job listings
// as thin items (FullContent is nil). The job_post_analyzer fetches the detail
// page for jobs that pass the relevance filter. The companyID parameter is
// unused since the scraper always targets careers.google.com.
func (g *Google) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	scraperPath := g.resolveScraperDir()
	scriptPath := filepath.Join(scraperPath, "scrape.js")

	g.log.Debug("google: resolved scraper path", logger.Str("scraper_path", scraperPath))

	// Verify scraper exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return types.FetchJobsResponse{}, fmt.Errorf("google: scraper not found at %s — run install.sh first", scriptPath)
	}

	// Run via xvfb-run for a virtual display so Chrome runs in headed mode.
	// Headed Chrome has a more realistic browser fingerprint than headless mode.
	// Falls back to plain node if xvfb-run is not installed (e.g. local dev).
	var cmd *exec.Cmd
	useXvfb := false
	if _, err := exec.LookPath("xvfb-run"); err == nil {
		useXvfb = true
		cmd = exec.CommandContext(ctx, "xvfb-run", "--auto-servernum", "--server-args=-screen 0 1366x768x24", "node", scriptPath)
	} else {
		cmd = exec.CommandContext(ctx, "node", scriptPath)
	}
	cmd.Dir = scraperPath

	g.log.Info("google: starting playwright scraper",
		logger.Bool("xvfb", useXvfb),
		logger.Int("timeout_ms", g.cfg.TimeoutMs),
		logger.Int("max_pages", g.cfg.MaxPages),
		logger.Int("min_delay_ms", g.cfg.MinDelayMs),
		logger.Int("max_delay_ms", g.cfg.MaxDelayMs),
	)

	// Pass config values directly to the Node process as environment variables.
	// Values come from the central Go config — no separate .env file needed.
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOOGLE_SCRAPER_MIN_DELAY=%s", itoa(g.cfg.MinDelayMs)),
		fmt.Sprintf("GOOGLE_SCRAPER_MAX_DELAY=%s", itoa(g.cfg.MaxDelayMs)),
		fmt.Sprintf("GOOGLE_SCRAPER_TIMEOUT_MS=%s", itoa(g.cfg.TimeoutMs)),
		fmt.Sprintf("GOOGLE_SCRAPER_MAX_PAGES=%s", itoa(g.cfg.MaxPages)),
		fmt.Sprintf("GOOGLE_SCRAPER_ENRICH_MIN=%s", itoa(g.cfg.EnrichMinMs)),
		fmt.Sprintf("GOOGLE_SCRAPER_ENRICH_MAX=%s", itoa(g.cfg.EnrichMaxMs)),
	)

	// xvfb-run merges the child's stderr into its own stdout via `"$@" 2>&1`.
	// This means both the scraper's log lines (written to process.stderr) and the
	// final JSON result (written to process.stdout) arrive on Go's stdout pipe.
	//
	// Strategy: read stdout line by line.
	//   - Lines containing `"msg":` are log lines — forward to the Go logger immediately.
	//   - Lines starting with `[` or `{"error"` are the JSON result — collect for parsing.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("google: failed to create stdout pipe: %w", err)
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("google: failed to start scraper: %w", err)
	}

	// Read the merged stdout stream: log lines are forwarded live; the JSON result
	// line is collected. The goroutine closes resultCh when it finds the result line.
	resultCh := make(chan []byte, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 8*1024*1024), 8*1024*1024) // 8 MB — large jobs list
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, `"msg":`) {
				// Scraper log line — forward to Go logger in real-time.
				g.forwardScraperLog(line)
			} else if len(line) > 0 {
				// JSON result line (array or error object).
				resultCh <- []byte(line)
			}
		}
		close(resultCh)
	}()

	waitErr := cmd.Wait()

	// Collect result — may arrive before or after Wait returns.
	var raw []byte
	if b, ok := <-resultCh; ok {
		raw = b
	}

	if waitErr != nil {
		g.log.Error("google: scraper process failed",
			waitErr,
			logger.Dur(logger.FieldDuration, time.Since(start)),
		)
		return types.FetchJobsResponse{}, fmt.Errorf("google: scraper failed: %w", waitErr)
	}
	g.log.Debug("google: scraper process exited",
		logger.Dur(logger.FieldDuration, time.Since(start)),
		logger.Int("result_bytes", len(raw)),
	)

	// Check for error response
	var scraperErr scraperError
	if err := json.Unmarshal(raw, &scraperErr); err == nil && scraperErr.Error != "" {
		g.log.Warn("google: scraper returned error payload", logger.Str("scraper_error", scraperErr.Error))
		return types.FetchJobsResponse{}, fmt.Errorf("google: scraper error: %s", scraperErr.Error)
	}

	// Parse jobs array
	var scraperJobs []scraperJob
	if err := json.Unmarshal(raw, &scraperJobs); err != nil {
		g.log.Error("google: failed to parse scraper output", err,
			logger.Str("raw_prefix", truncate(string(raw), 200)),
		)
		return types.FetchJobsResponse{}, fmt.Errorf("google: failed to parse scraper output: %w\nraw (first 500 bytes): %s", err, truncate(string(raw), 500))
	}

	// Convert to pipeline types. FullContent is set when the scraper visited the
	// detail page; nil when the scraper timed out before reaching that job.
	items := make([]types.FetchJobsItem, 0, len(scraperJobs))
	enriched := 0
	for _, sj := range scraperJobs {
		var fc *types.FullJobContent
		if sj.FullContent != nil {
			enriched++
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

	thinCount := len(items) - enriched
	g.log.Info("google: scraper complete",
		logger.Dur(logger.FieldDuration, time.Since(start)),
		logger.Int(logger.FieldFetched, len(items)),
		logger.Int("full_content", enriched),
		logger.Int("thin_no_detail", thinCount),
	)
	if thinCount > 0 {
		g.log.Warn("google: some jobs have no full_content — scraper timed out before visiting their detail page",
			logger.Int("thin_no_detail", thinCount),
		)
	}

	return types.FetchJobsResponse{Jobs: items}, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// forwardScraperLog parses a single stderr line from the Node scraper
// (JSON: {"ts":"...","msg":"...",...}) and re-emits it via the Go logger.
//
// Level mapping (matches LOG_LEVEL=info in production):
//
//	Warn  — errors, failures, timeouts, unexpected empty results
//	Info  — milestones: navigation, pagination, enrichment phase transitions
//	Debug — per-item noise: delay waits, per-job enrichment steps
func (g *Google) forwardScraperLog(line string) {
	if line == "" {
		return
	}
	var entry struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		// Not JSON (e.g. Node startup noise) — forward raw at Debug.
		g.log.Debug("google[scraper]: " + line)
		return
	}
	msg := "google[scraper]: " + entry.Msg
	lower := strings.ToLower(entry.Msg)

	switch {
	case strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "no jobs found") ||
		strings.Contains(lower, "empty page"):
		g.log.Warn(msg, logger.Str("raw", line))

	case strings.HasPrefix(lower, "waiting") ||
		strings.HasPrefix(lower, "enriching job detail") ||
		strings.HasPrefix(lower, "job enriched"):
		// High-frequency per-item events — suppress at info level.
		g.log.Debug(msg, logger.Str("raw", line))

	default:
		// Milestones: starting, navigating, page scraped, pagination complete,
		// starting detail page enrichment, enrichment complete, scrape complete.
		g.log.Info(msg, logger.Str("raw", line))
	}
}

// resolveScraperDir returns the path to the scraper/ directory.
// Priority: scraperDir override → GOOGLE_SCRAPER_DIR env → executable-relative path.
func (g *Google) resolveScraperDir() string {
	if g.scraperDir != "" {
		return g.scraperDir
	}
	// Allow override via env var (useful in Docker / non-standard layouts)
	if dir := os.Getenv("GOOGLE_SCRAPER_DIR"); dir != "" {
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Compile-time interface checks.
var (
	_ types.Adapter     = (*Google)(nil)
	_ types.URLProvider = (*Google)(nil)
)

// Note: Google does NOT implement types.ContentFetcher. It is a full-detail
// adapter — the Playwright scraper populates FullContent at scan time so the
// job_post_analyzer skips the HTTP fetch entirely via the FullContent fast path.
