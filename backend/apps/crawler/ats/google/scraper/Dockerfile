# Dockerfile for the Google Careers Playwright scraper.
# Runs as a standalone container — the Go crawler adapter calls it via subprocess
# (when co-located) or via HTTP (when running as a service).
#
# Build (from the scraper/ directory):
#   docker build -t jobradar-google-scraper .
#
# Or via docker-compose (from project root):
#   docker compose build google-scraper

FROM mcr.microsoft.com/playwright:v1.59.1-noble

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm ci --omit=dev --cache /root/.npm

COPY scrape.js ./

ARG GIT_SHA=dev
LABEL org.opencontainers.image.revision="${GIT_SHA}"

CMD ["node", "scrape.js"]
