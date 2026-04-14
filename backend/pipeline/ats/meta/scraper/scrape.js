#!/usr/bin/env node
/**
 * Meta Careers Playwright Scraper
 *
 * Navigates to metacareers.com/jobs/, scrolls to load all US jobs,
 * then visits each job detail page to extract full content.
 *
 * Meta's careers site uses Facebook's Comet/Relay framework. Job data is loaded
 * via POST requests with session-specific tokens. Playwright is the only viable
 * approach since these tokens can only be obtained by first loading the page.
 *
 * Uses playwright-extra + stealth plugin to avoid bot detection, with
 * configurable random delays between every action.
 *
 * Usage:
 *   node scrape.js                         # uses defaults or shell env vars
 *
 * Configuration: all values sourced from shell environment variables:
 *
 *   META_SCRAPER_TIMEOUT_MS    – overall timeout in ms (default: 3600000 = 1 hour)
 *   META_SCRAPER_MIN_DELAY     – min delay between job loads in ms (default: 3000)
 *   META_SCRAPER_MAX_DELAY     – max delay between job loads in ms (default: 8000)
 *   META_SCRAPER_MAX_JOBS      – max jobs to fetch, 0 = all (default: 0)
 *
 * Output: JSON array of job objects to stdout. Logs go to stderr.
 */

const { chromium } = require("playwright-extra");
const stealth = require("puppeteer-extra-plugin-stealth");

// Apply stealth plugin to mask automation signals
chromium.use(stealth());

// ── Configuration ────────────────────────────────────────────────────────────

const BASE_URL = "https://www.metacareers.com/jobs/";

const TIMEOUT_MS = parseInt(
  process.env.META_SCRAPER_TIMEOUT_MS || "3600000",
  10
); // 1 hour
const MIN_DELAY = parseInt(
  process.env.META_SCRAPER_MIN_DELAY || "3000",
  10
); // 3 seconds
const MAX_DELAY = parseInt(
  process.env.META_SCRAPER_MAX_DELAY || "8000",
  10
); // 8 seconds

// MAX_JOBS=0 means all jobs. MAX_JOBS=10 limits to 10 jobs (useful for testing).
const MAX_JOBS = parseInt(process.env.META_SCRAPER_MAX_JOBS || "0", 10);

// ── Helpers ──────────────────────────────────────────────────────────────────

function log(msg, data) {
  const entry = { ts: new Date().toISOString(), msg, ...data };
  process.stderr.write(JSON.stringify(entry) + "\n");
}

function randomDelay(min = MIN_DELAY, max = MAX_DELAY) {
  return min + Math.floor(Math.random() * (max - min));
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Simulate human-like mouse movement to a random position on the page.
 */
async function humanMouseMove(page) {
  const viewport = page.viewportSize();
  if (!viewport) return;
  const x = 100 + Math.floor(Math.random() * (viewport.width - 200));
  const y = 100 + Math.floor(Math.random() * (viewport.height - 200));
  await page.mouse.move(x, y, {
    steps: 5 + Math.floor(Math.random() * 10),
  });
}

/**
 * Small random scroll to mimic a human reading the page.
 */
async function humanScroll(page) {
  const amount = 200 + Math.floor(Math.random() * 400);
  await page.evaluate((px) => window.scrollBy(0, px), amount);
  await sleep(500 + Math.floor(Math.random() * 1000));
}

// ── Normalize employment type to pipeline constants ──────────────────────────

function normalizeEmployType(raw) {
  const lower = (raw || "").toLowerCase();
  if (lower.includes("full-time") || lower.includes("full time"))
    return "full_time";
  if (lower.includes("part-time") || lower.includes("part time"))
    return "part_time";
  if (lower.includes("contract")) return "contract";
  if (lower.includes("intern")) return "internship";
  return "";
}

// ── Extract job cards from the listing page ──────────────────────────────────

/**
 * Extracts job card data from Meta's job listing page.
 * Meta's DOM is heavily obfuscated, so we use resilient selectors:
 *
 * - Look for <a> elements linking to /jobs/\d+/ (job detail URLs)
 * - Extract title, location, and team from the card text
 * - Keep track of unique job IDs to avoid duplicates
 */
async function extractJobsFromPage(page) {
  return page.evaluate(() => {
    const jobs = [];
    const seenIds = new Set();

    // Find all links pointing to job detail pages
    const links = document.querySelectorAll('a[href*="/jobs/"]');

    for (const link of links) {
      try {
        const href = link.getAttribute("href") || "";

        // Extract job ID from URL: /jobs/(\d+)/
        const idMatch = href.match(/\/jobs\/(\d+)\//);
        if (!idMatch) continue;

        const jobId = idMatch[1];
        if (seenIds.has(jobId)) continue;
        seenIds.add(jobId);

        // Build absolute URL if needed
        const absoluteUrl = href.startsWith("http")
          ? href
          : "https://www.metacareers.com" + (href.startsWith("/") ? href : "/" + href);

        // Get the card container (walk up the tree)
        let card = link.closest("div[role]") || link.closest("div");
        if (!card) card = link.parentElement;
        if (!card) continue;

        // Extract text from the card for title, location, and team
        const cardText = card.innerText || card.textContent || "";
        const lines = cardText.split("\n").filter((line) => line.trim());

        // First line is typically the title
        const title = lines[0] || "";

        // Look for location pattern (e.g., "Menlo Park, CA" or "New York, NY")
        // Usually in the first few lines after title
        let location = "";
        for (let i = 1; i < Math.min(lines.length, 5); i++) {
          const line = lines[i].trim();
          // Check if it looks like a location (has a comma, or is a known US city)
          if (
            line.includes(",") ||
            line === "Remote" ||
            line === "Hybrid"
          ) {
            location = line;
            break;
          }
        }

        // Extract team/department from remaining lines
        let team = "";
        for (let i = 1; i < lines.length; i++) {
          const line = lines[i].trim();
          if (
            line &&
            !line.includes(",") &&
            line !== "Remote" &&
            line !== "Hybrid"
          ) {
            team = line;
            break;
          }
        }

        jobs.push({
          id: jobId,
          title: title || "Unknown Position",
          url: absoluteUrl,
          location: location || "Unknown Location",
          team: team || "Meta",
        });
      } catch (e) {
        // Skip malformed cards
      }
    }

    return jobs;
  });
}

// ── Extract full job content from a detail page ──────────────────────────────

/**
 * Navigates to a job detail page and extracts the full description.
 * Meta's detail pages have a main content area with description text.
 */
async function extractJobDetail(page, title, location) {
  return page.evaluate((title, location) => {
    const lines = [title, "", location, ""];

    // Look for the main content container
    const mainContent =
      document.querySelector('[role="main"]') ||
      document.querySelector("main") ||
      document.body;

    if (!mainContent) {
      return lines.join("\n").trim();
    }

    // Extract all text content from the main area
    const allText = mainContent.innerText || mainContent.textContent || "";

    // Take the first 5000 characters of substantial text (skip empty lines at start)
    const trimmed = allText.trim();
    lines.push(trimmed.substring(0, 5000));

    return lines.join("\n").trim();
  }, title, location);
}

// ── Scroll to load all jobs ──────────────────────────────────────────────────

/**
 * Scrolls the page repeatedly to trigger lazy loading until no new jobs appear.
 * Meta uses infinite scroll, so we keep scrolling until the job count stabilizes.
 */
async function scrollToLoadAllJobs(page, maxJobs = 0) {
  log("starting scroll to load all US jobs");

  let previousCount = 0;
  let scrolls = 0;
  const maxScrollsWithoutNewJobs = 5; // Stop if no new jobs after 5 scrolls
  let scrollsWithoutNewJobs = 0;

  while (true) {
    // Check timeout
    const jobs = await extractJobsFromPage(page);
    const currentCount = jobs.length;

    log("scroll check", {
      scroll_num: scrolls,
      jobs_found: currentCount,
      new_jobs: currentCount - previousCount,
    });

    // If we've reached the job limit, stop
    if (maxJobs > 0 && currentCount >= maxJobs) {
      log("reached max jobs limit", {
        limit: maxJobs,
        found: currentCount,
      });
      break;
    }

    // If no new jobs appeared, stop scrolling
    if (currentCount === previousCount) {
      scrollsWithoutNewJobs++;
      if (scrollsWithoutNewJobs >= maxScrollsWithoutNewJobs) {
        log("no new jobs after multiple scrolls, stopping");
        break;
      }
    } else {
      scrollsWithoutNewJobs = 0; // Reset counter
      previousCount = currentCount;
    }

    // Scroll down to trigger lazy loading
    await humanScroll(page);
    await sleep(1000 + Math.floor(Math.random() * 1000));

    scrolls++;

    // Safety limit on total scrolls
    if (scrolls > 100) {
      log("max scrolls reached");
      break;
    }
  }

  const finalJobs = await extractJobsFromPage(page);
  log("scroll complete", { total_jobs: finalJobs.length, scrolls });

  return finalJobs;
}

// ── Main ─────────────────────────────────────────────────────────────────────

async function main() {
  log("starting meta careers scraper", {
    url: BASE_URL,
    timeout_ms: TIMEOUT_MS,
    min_delay: MIN_DELAY,
    max_delay: MAX_DELAY,
    max_jobs: MAX_JOBS,
  });

  const startTime = Date.now();

  // Run full Chrome (headed) via Xvfb virtual display — better anti-bot fingerprint
  // than headless mode.
  const browser = await chromium.launch({
    headless: false,
    args: [
      "--no-sandbox",
      "--disable-setuid-sandbox",
      "--disable-dev-shm-usage",
      "--start-maximized",
    ],
  });

  const context = await browser.newContext({
    viewport: { width: 1366, height: 768 },
    userAgent:
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
    locale: "en-US",
    timezoneId: "America/New_York",
  });

  const page = await context.newPage();

  try {
    // ── Phase 1: Navigate to the Meta careers page ─────────────────────
    log("navigating to meta careers page");
    await page.goto(BASE_URL, {
      waitUntil: "domcontentloaded",
      timeout: 60000,
    });

    // Wait for initial content to render
    await sleep(3000 + Math.floor(Math.random() * 2000));

    // ── Phase 2: Scroll to load all US jobs ───────────────────────────
    const allJobs = await scrollToLoadAllJobs(page, MAX_JOBS);

    if (allJobs.length === 0) {
      log("no jobs found on page — page structure may have changed");
      process.stdout.write(JSON.stringify([]));
      await browser.close();
      process.exit(0);
    }

    log("listing page complete", { total_jobs: allJobs.length });

    // Limit to MAX_JOBS if specified
    let jobsToProcess = allJobs;
    if (MAX_JOBS > 0 && allJobs.length > MAX_JOBS) {
      jobsToProcess = allJobs.slice(0, MAX_JOBS);
      log("limiting to max jobs", { max: MAX_JOBS, will_process: jobsToProcess.length });
    }

    // ── Phase 3: Visit each job detail page to extract full content ──────
    log("starting detail page enrichment", { total_jobs: jobsToProcess.length });

    for (let i = 0; i < jobsToProcess.length; i++) {
      const job = jobsToProcess[i];

      // Check timeout before visiting each detail page
      if (Date.now() - startTime > TIMEOUT_MS) {
        log("timeout reached during enrichment — outputting partial results", {
          elapsed_ms: Date.now() - startTime,
          enriched: i,
          total: jobsToProcess.length,
        });
        break;
      }

      log("enriching job detail", {
        job_id: job.id,
        index: i + 1,
        total: jobsToProcess.length,
      });

      try {
        // Random delay before each detail page (skip delay before the first)
        if (i > 0) {
          const delay = randomDelay();
          log("waiting before detail page", { delay_ms: delay });
          await sleep(delay);
        }

        await humanMouseMove(page);
        await page.goto(job.url, { waitUntil: "domcontentloaded", timeout: 30000 });

        // Wait for JS to render content
        await sleep(1500 + Math.floor(Math.random() * 1500));

        job.description = await extractJobDetail(page, job.title, job.location);
        log("job enriched", {
          job_id: job.id,
          desc_length: job.description.length,
        });
      } catch (e) {
        log("detail page failed", { job_id: job.id, error: e.message });
        job.description = "";
      }
    }

    log("enrichment complete", {
      total_enriched: jobsToProcess.filter((j) => j.description).length,
    });

    // ── Output ──────────────────────────────────────────────────────────
    const elapsed = Math.round((Date.now() - startTime) / 1000);
    log("scrape complete", {
      total_jobs: jobsToProcess.length,
      elapsed_seconds: elapsed,
    });

    const output = jobsToProcess.map((job) => ({
      id: `meta-${job.id}`,
      title: job.title,
      url: job.url,
      location: job.location,
      employ_type: normalizeEmployType("full_time"),
      full_content: {
        description: job.description || "",
        requirements: "",
        compensation: "",
        workplace_type: "",
        department: job.team || "Meta",
        published_date: "",
      },
    }));

    process.stdout.write(JSON.stringify(output));
  } catch (err) {
    log("fatal error", { error: err.message, stack: err.stack });
    process.stdout.write(JSON.stringify({ error: err.message }));
    process.exit(1);
  } finally {
    await browser.close();
  }
}

main().catch((err) => {
  log("unhandled error", { error: err.message });
  process.exit(1);
});
