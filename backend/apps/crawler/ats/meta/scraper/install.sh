#!/usr/bin/env bash
# install.sh — One-time setup for the Meta Careers Playwright scraper.
#
# Prerequisites: Node.js >= 18
#
# Usage:
#   cd pipeline/ats/meta/scraper
#   bash install.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "==> Installing Node.js dependencies..."
npm install

echo "==> Installing Playwright Chromium browser..."
npx playwright install chromium

echo ""
echo "Done! The Meta Careers scraper is ready."
echo "Test it with: node scrape.js"
