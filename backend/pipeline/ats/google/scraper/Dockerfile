# Dockerfile for the Google Careers Playwright scraper.
# Runs as a standalone container — the Go crawler adapter calls it via subprocess
# (when co-located) or via HTTP (when running as a service).
#
# Build (from the scraper/ directory):
#   docker build -t jobradar-google-scraper .
#
# Or via docker-compose (from project root):
#   docker compose build google-scraper
#
# Run:
#   docker run --rm jobradar-google-scraper

FROM mcr.microsoft.com/playwright:v1.59.1-noble

WORKDIR /app

# Copy package files and install deps
COPY package.json ./
RUN npm install --production

# Copy scraper script
COPY scrape.js ./

# Playwright browsers are pre-installed in the base image

# Default: run the scraper
CMD ["node", "scrape.js"]
