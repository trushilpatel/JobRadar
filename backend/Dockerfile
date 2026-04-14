FROM golang:1.24-alpine AS deps

ENV GOTOOLCHAIN=auto

WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ── Parallel binary builds ────────────────────────────────────────────────────
# BuildKit runs each FROM stage in parallel when they share no output dependency.

FROM deps AS build-api
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/api ./cmd/api

FROM deps AS build-migrate
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/migrate ./cmd/migrate

FROM deps AS build-scheduler
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/scheduler ./cmd/company_scan_scheduler

FROM deps AS build-filter
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/filter ./cmd/job_listing_relevance_filter

FROM deps AS build-enricher
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/enricher ./cmd/job_post_analyzer

# ── Final image ───────────────────────────────────────────────────────────────

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=build-api      /out/api       /app/
COPY --from=build-migrate  /out/migrate   /app/
COPY --from=build-scheduler /out/scheduler /app/
COPY --from=build-filter   /out/filter    /app/
COPY --from=build-enricher /out/enricher  /app/
COPY migrations/ /app/migrations/
