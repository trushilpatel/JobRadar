FROM golang:1.24-alpine AS go-builder

ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0

WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# All binaries built in one RUN so they share a single go-build cache mount.
# Parallel stages each get their own isolated cache — one stage is faster.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/api       ./cmd/api && \
    go build -o /out/migrate   ./cmd/migrate && \
    go build -o /out/scheduler ./cmd/company_scan_scheduler && \
    go build -o /out/filter    ./cmd/job_listing_relevance_filter && \
    go build -o /out/enricher  ./cmd/job_post_analyzer

# ── Final image ───────────────────────────────────────────────────────────────

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /out/api       /app/
COPY --from=go-builder /out/migrate   /app/
COPY --from=go-builder /out/scheduler /app/
COPY --from=go-builder /out/filter    /app/
COPY --from=go-builder /out/enricher  /app/
COPY migrations/ /app/migrations/
