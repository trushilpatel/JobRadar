#!/usr/bin/env node
/**
 * Google Careers Playwright Scraper
 *
 * Navigates to careers.google.com with pre-set filters (US, Early+Mid,
 * Full-time, Software Engineering), paginates through all result pages,
 * then visits each job detail page to extract full content.
 *
 * Uses playwright-extra + stealth plugin to avoid bot detection, with
 * configurable random delays between every action (default 30-45 s).
 *
 * Usage:
 *   node --env-file=.env scrape.js        # load config from .env (recommended)
 *   node scrape.js                         # uses defaults or shell env vars
 *   npm run scrape                         # same as --env-file=.env
 *
 * Configuration: edit .env in this directory. All values can also be overridden
 * by shell environment variables (shell always takes precedence over .env).
 *
 *   GOOGLE_SCRAPER_TIMEOUT_MS  – overall timeout in ms (default: 2700000 = 45 min)
 *   GOOGLE_SCRAPER_MIN_DELAY   – min delay between page navigations in ms (default: 30000)
 *   GOOGLE_SCRAPER_MAX_DELAY   – max delay between page navigations in ms (default: 60000)
 *   GOOGLE_SCRAPER_MAX_PAGES   – max pages to scrape, 0 = all (default: 0)
 *
 * Output: JSON array of job objects to stdout. Logs go to stderr.
 */

const { chromium } = require('playwright-extra')
const stealth = require('puppeteer-extra-plugin-stealth')

// Apply stealth plugin to mask automation signals
chromium.use(stealth())

// ── Configuration ────────────────────────────────────────────────────────────

const BASE_URL = 'https://www.google.com/about/careers/applications/jobs/results'

// Pre-built filter query string — discovered by testing the real page
const FILTER_PARAMS = [
  'location=United%20States',
  'target_level=EARLY',
  'target_level=MID',
  'employment_type=FULL_TIME',
  'q=Software%20Engineering',
].join('&')

const CAREERS_URL = `${BASE_URL}?${FILTER_PARAMS}`

const TIMEOUT_MS = parseInt(process.env.GOOGLE_SCRAPER_TIMEOUT_MS || '14400000', 10) // 4 hours
const MIN_DELAY = parseInt(process.env.GOOGLE_SCRAPER_MIN_DELAY || '120000', 10) // 2 min
const MAX_DELAY = parseInt(process.env.GOOGLE_SCRAPER_MAX_DELAY || '240000', 10) // 4 min

// MAX_PAGES=1 for quick local testing — limits pagination to N pages.
// Default 0 = no limit (all pages).
const MAX_PAGES = parseInt(process.env.GOOGLE_SCRAPER_MAX_PAGES || '0', 10)

// Delay between individual job detail page visits.
const ENRICH_MIN = parseInt(process.env.GOOGLE_SCRAPER_ENRICH_MIN || '60000', 10) // 1 min
const ENRICH_MAX = parseInt(process.env.GOOGLE_SCRAPER_ENRICH_MAX || '120000', 10) // 2 min

// ── Helpers ──────────────────────────────────────────────────────────────────

function log(msg, data) {
  const entry = { ts: new Date().toISOString(), msg, ...data }
  process.stderr.write(JSON.stringify(entry) + '\n')
}

function randomDelay(min = MIN_DELAY, max = MAX_DELAY) {
  return min + Math.floor(Math.random() * (max - min))
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/**
 * Simulate human-like mouse movement to a random position on the page.
 */
async function humanMouseMove(page) {
  const viewport = page.viewportSize()
  if (!viewport) return
  const x = 100 + Math.floor(Math.random() * (viewport.width - 200))
  const y = 100 + Math.floor(Math.random() * (viewport.height - 200))
  await page.mouse.move(x, y, {
    steps: 5 + Math.floor(Math.random() * 10),
  })
}

/**
 * Small random scroll to mimic a human reading the page.
 */
async function humanScroll(page) {
  const amount = 200 + Math.floor(Math.random() * 400)
  await page.evaluate((px) => window.scrollBy(0, px), amount)
  await sleep(1000 + Math.floor(Math.random() * 2000))
}

// ── Normalize employment type to pipeline constants ──────────────────────────

function normalizeEmployType(raw) {
  const lower = (raw || '').toLowerCase()
  if (lower.includes('full-time') || lower.includes('full time')) return 'full_time'
  if (lower.includes('part-time') || lower.includes('part time')) return 'part_time'
  if (lower.includes('contract')) return 'contract'
  if (lower.includes('intern')) return 'internship'
  return ''
}

// ── Extract job listings from the current results page ──────────────────────

/**
 * Extracts job card data from a listing page. Runs inside page.evaluate().
 *
 * RESILIENT SELECTOR STRATEGY — avoids obfuscated CSS class names that
 * change on every Google redeploy. Instead we anchor on:
 *
 *   Job links:   a[aria-label^="Learn more about"]  (stable accessibility attr)
 *   Job card:    closest <li> ancestor of the link
 *   Title:       h3 inside the card (semantic tag, stable)
 *   Location:    material icon "place" text → next sibling text node
 *   Experience:  material icon "bar_chart" text → next sibling text node
 *   Org:         material icon "corporate_fare" or "business" → next sibling text
 *   Job ID:      numeric ID extracted from link href path
 */
async function extractJobsFromPage(page) {
  return page.evaluate(() => {
    const jobs = []

    // Anchor: find all "Learn more about ..." links — stable aria-label
    const jobLinks = document.querySelectorAll('a[aria-label^="Learn more about"]')

    for (const linkEl of jobLinks) {
      try {
        const href = linkEl.getAttribute('href') || ''

        // Extract job ID from href path: jobs/results/{numeric-id}-{slug}
        const idMatch = href.match(/jobs\/results\/(\d+)/)
        const jobId = idMatch ? idMatch[1] : null
        if (!jobId) continue

        // Build absolute URL (strip query params from filters)
        const pathname = href.startsWith('/')
          ? href.split('?')[0]
          : `/about/careers/applications/${href.split('?')[0]}`
        const absoluteUrl = `https://www.google.com${pathname}`

        // Walk up to the containing <li> — the job card boundary
        const card = linkEl.closest('li')
        if (!card) continue

        // Title — h3 inside the card (semantic, stable)
        const titleEl = card.querySelector('h3')
        const title = titleEl ? titleEl.textContent.trim() : ''

        // Helper: find a material icon by its text content, return the
        // adjacent text from the icon's parent element (minus the icon text).
        function getTextByIcon(iconName) {
          const icons = card.querySelectorAll('i, span')
          for (const el of icons) {
            if (el.textContent.trim() === iconName) {
              // The parent element holds icon + label text
              const parent = el.parentElement
              if (!parent) continue
              let text = parent.textContent.trim()
              // Strip the icon name prefix (e.g. "placeNew York" → "New York")
              text = text.replace(new RegExp('^' + iconName), '').trim()
              if (text) return text
            }
          }
          return ''
        }

        // Location — material icon "place" precedes the location text
        const location = getTextByIcon('place')

        // Experience level — material icon "bar_chart" (e.g. "Early", "Mid")
        const experience = getTextByIcon('bar_chart')

        // Organization — material icon "corporate_fare" or "business"
        let org = getTextByIcon('corporate_fare')
        if (!org) org = getTextByIcon('business')

        jobs.push({
          id: jobId,
          title,
          url: absoluteUrl,
          location,
          experience,
          org,
        })
      } catch (e) {
        // Skip malformed cards
      }
    }
    return jobs
  })
}

// ── Check if there is a "next page" link ─────────────────────────────────────

async function hasNextPage(page) {
  return page.evaluate(() => {
    const next = document.querySelector('a[aria-label="Go to next page"]')
    return !!next
  })
}

// ── Click the "next page" link ───────────────────────────────────────────────

async function goToNextPage(page) {
  const nextLink = await page.$('a[aria-label="Go to next page"]')
  if (!nextLink) return false
  await nextLink.click()
  // Wait for navigation and new content to render
  await sleep(5000 + Math.floor(Math.random() * 3000))
  return true
}

// ── Extract full job content from a detail page ──────────────────────────────

/**
 * Navigates to a job detail page (already loaded by the caller) and extracts
 * the qualification and responsibility sections.
 *
 * Strategy: find <h3> headings by text content match, then walk siblings
 * collecting <ul>/<ol> bullet items and plain paragraphs — no CSS classes used.
 *
 * @param {import('playwright').Page} page - already navigated to the detail URL
 * @param {string} title - job title from the listing card
 * @param {string} location - location from the listing card
 * @returns {Promise<string>} plain-text description
 */
async function extractJobDetail(page, title, location) {
  return page.evaluate(
    (title, location) => {
      const lines = [title, '', location, '']

      /**
       * Find a <h3> whose text includes sectionName (case-insensitive),
       * then collect its sibling content until the next <h3>.
       * Returns formatted section text or empty string if not found.
       */
      function extractSection(sectionName) {
        const h3s = document.querySelectorAll('h3')
        for (const h3 of h3s) {
          if (h3.textContent.trim().toLowerCase().includes(sectionName.toLowerCase())) {
            const parts = [h3.textContent.trim()]
            let el = h3.nextElementSibling
            while (el && el.tagName !== 'H3') {
              // Direct UL/OL — extract li items
              if (el.tagName === 'UL' || el.tagName === 'OL') {
                el.querySelectorAll('li').forEach((li) => {
                  const text = li.textContent.trim()
                  if (text) parts.push('- ' + text)
                })
              } else {
                // Check for nested UL/OL inside container divs
                const nestedLists = el.querySelectorAll('ul, ol')
                if (nestedLists.length > 0) {
                  nestedLists.forEach((list) => {
                    list.querySelectorAll('li').forEach((li) => {
                      const text = li.textContent.trim()
                      if (text) parts.push('- ' + text)
                    })
                  })
                } else {
                  const text = (el.innerText || el.textContent || '').trim()
                  if (text) parts.push(text)
                }
              }
              el = el.nextElementSibling
            }
            return parts.join('\n')
          }
        }
        return ''
      }

      const sections = ['Minimum qualifications', 'Preferred qualifications', 'Responsibilities']

      for (const section of sections) {
        const content = extractSection(section)
        if (content) {
          lines.push(content, '')
        }
      }

      return lines.join('\n').trim()
    },
    title,
    location
  )
}

// ── Main ─────────────────────────────────────────────────────────────────────

async function main() {
  log('starting google careers scraper', {
    url: CAREERS_URL,
    timeout_ms: TIMEOUT_MS,
    min_delay: MIN_DELAY,
    max_delay: MAX_DELAY,
  })

  const startTime = Date.now()

  // Run full Chrome (headed) via Xvfb virtual display — better anti-bot fingerprint
  // than headless mode. The Playwright Docker image ships Xvfb for this purpose.
  const browser = await chromium.launch({
    headless: false,
    args: [
      '--no-sandbox',
      '--disable-setuid-sandbox',
      '--disable-dev-shm-usage',
      '--start-maximized',
    ],
  })

  const context = await browser.newContext({
    viewport: { width: 1366, height: 768 },
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36',
    locale: 'en-US',
    timezoneId: 'America/New_York',
  })

  const page = await context.newPage()

  try {
    // ── Phase 1: Navigate to filtered results ─────────────────────────────
    log('navigating to google careers with filters')
    await page.goto(CAREERS_URL, {
      waitUntil: 'domcontentloaded',
      timeout: 60000,
    })
    // Wait for initial content to render
    await sleep(5000 + Math.floor(Math.random() * 3000))

    // Verify we got results
    const initialJobs = await extractJobsFromPage(page)
    log('initial page loaded', { jobs_on_page: initialJobs.length })

    if (initialJobs.length === 0) {
      log('no jobs found on initial page — page structure may have changed')
      process.stdout.write(JSON.stringify([]))
      await browser.close()
      process.exit(0)
    }

    // ── Phase 2: Paginate through all result pages ──────────────────────
    const allJobs = [...initialJobs]
    const seenIds = new Set(initialJobs.map((j) => j.id))
    let pageNum = 1

    log('starting pagination', { page: pageNum, jobs_so_far: allJobs.length })

    while (await hasNextPage(page)) {
      // Check timeout
      if (Date.now() - startTime > TIMEOUT_MS) {
        log('timeout reached during pagination', {
          elapsed_ms: Date.now() - startTime,
          pages_scraped: pageNum,
          jobs_collected: allJobs.length,
        })
        break
      }

      // Human-like behavior before clicking next
      await humanMouseMove(page)
      await humanScroll(page)

      // Random delay (30-45s) to mimic human browsing
      const delay = randomDelay()
      log('waiting before next page', { delay_ms: delay })
      await sleep(delay)

      // Navigate to next page
      const navigated = await goToNextPage(page)
      if (!navigated) {
        log('failed to navigate to next page')
        break
      }

      pageNum++

      // Extract jobs from this page
      const pageJobs = await extractJobsFromPage(page)
      let newCount = 0

      for (const job of pageJobs) {
        if (!seenIds.has(job.id)) {
          seenIds.add(job.id)
          allJobs.push(job)
          newCount++
        }
      }

      log('page scraped', {
        page: pageNum,
        jobs_on_page: pageJobs.length,
        new_jobs: newCount,
        total_jobs: allJobs.length,
      })

      // If a page returned 0 new jobs, something is wrong — stop
      if (newCount === 0 && pageJobs.length === 0) {
        log('empty page encountered, stopping pagination')
        break
      }

      // Stop early if MAX_PAGES is set (useful for local testing)
      if (MAX_PAGES > 0 && pageNum >= MAX_PAGES) {
        log('max pages reached, stopping pagination', { max_pages: MAX_PAGES })
        break
      }
    }

    log('pagination complete', {
      pages_scraped: pageNum,
      total_listings: allJobs.length,
    })

    // ── Phase 3: Visit each job detail page to extract full content ──────
    log('starting detail page enrichment', { total_jobs: allJobs.length })

    for (let i = 0; i < allJobs.length; i++) {
      const job = allJobs[i]

      // Check timeout before visiting each detail page
      if (Date.now() - startTime > TIMEOUT_MS) {
        log('timeout reached during enrichment — outputting partial results', {
          elapsed_ms: Date.now() - startTime,
          enriched: i,
          total: allJobs.length,
        })
        break
      }

      log('enriching job detail', {
        job_id: job.id,
        index: i + 1,
        total: allJobs.length,
      })

      try {
        // Human-like delay before each detail page (skip delay before the first)
        if (i > 0) {
          const delay = ENRICH_MIN + Math.floor(Math.random() * (ENRICH_MAX - ENRICH_MIN))
          log('waiting before detail page', { delay_ms: delay })
          await sleep(delay)
        }

        await humanMouseMove(page)
        await page.goto(job.url, { waitUntil: 'domcontentloaded', timeout: 30000 })
        // Short wait for JS to render sections
        await sleep(2000 + Math.floor(Math.random() * 2000))

        job.description = await extractJobDetail(page, job.title, job.location)
        log('job enriched', {
          job_id: job.id,
          desc_length: job.description.length,
        })
      } catch (e) {
        log('detail page failed', { job_id: job.id, error: e.message })
        job.description = ''
      }
    }

    log('enrichment complete', { total_enriched: allJobs.filter((j) => j.description).length })

    // ── Output ──────────────────────────────────────────────────────────
    const elapsed = Math.round((Date.now() - startTime) / 1000)
    log('scrape complete', {
      total_jobs: allJobs.length,
      pages_scraped: pageNum,
      elapsed_seconds: elapsed,
    })

    const output = allJobs.map((job) => ({
      id: `google-${job.id}`,
      title: job.title,
      url: job.url,
      location: job.location,
      employ_type: normalizeEmployType('full_time'),
      full_content: {
        description: job.description || '',
        requirements: '',
        compensation: '',
        workplace_type: '',
        department: job.org || 'Google',
        published_date: '',
      },
    }))

    process.stdout.write(JSON.stringify(output))
  } catch (err) {
    log('fatal error', { error: err.message, stack: err.stack })
    process.stdout.write(JSON.stringify({ error: err.message }))
    process.exit(1)
  } finally {
    await browser.close()
  }
}

main().catch((err) => {
  log('unhandled error', { error: err.message })
  process.exit(1)
})
