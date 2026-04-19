#!/usr/bin/env node
/**
 * Test a single Google job URL — dumps raw extracted content to stdout.
 * Usage: node test-single.js <url>
 */

const { chromium } = require('playwright-extra')
const stealth = require('puppeteer-extra-plugin-stealth')

chromium.use(stealth())

const url = process.argv[2]
if (!url) {
  console.error('Usage: node test-single.js <google-careers-url>')
  process.exit(1)
}

async function extractJobDetail(page) {
  return page.evaluate(() => {
    function extractSection(sectionName) {
      const h3s = document.querySelectorAll('h3')
      for (const h3 of h3s) {
        if (h3.textContent.trim().toLowerCase().includes(sectionName.toLowerCase())) {
          const parts = [h3.textContent.trim()]
          let el = h3.nextElementSibling
          while (el && el.tagName !== 'H3') {
            if (el.tagName === 'UL' || el.tagName === 'OL') {
              el.querySelectorAll('li').forEach((li) => {
                const text = li.textContent.trim()
                if (text) parts.push('- ' + text)
              })
            } else {
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

    const result = {}
    for (const section of sections) {
      const content = extractSection(section)
      result[section] = content || '(not found)'
    }

    // Also grab page title and all h3s for debugging
    result._title = document.title
    result._h3s = Array.from(document.querySelectorAll('h3')).map((h) => h.textContent.trim())

    return result
  })
}

async function main() {
  console.error(`Navigating to: ${url}`)

  const browser = await chromium.launch({
    headless: false,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--start-maximized'],
  })

  const context = await browser.newContext({
    viewport: { width: 1366, height: 768 },
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36',
    locale: 'en-US',
  })

  const page = await context.newPage()

  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 })

    // Wait for JS to render
    await page.waitForTimeout(3000)

    const result = await extractJobDetail(page)
    console.log(JSON.stringify(result, null, 2))
  } finally {
    await browser.close()
  }
}

main().catch((err) => {
  console.error('Error:', err.message)
  process.exit(1)
})
