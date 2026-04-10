FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /out/api             ./cmd/api
RUN go build -o /out/migrate         ./cmd/migrate
RUN go build -o /out/scheduler       ./cmd/company_scan_scheduler
RUN go build -o /out/crawler         ./cmd/job_listing_crawler
RUN go build -o /out/filter          ./cmd/job_listing_relevance_filter
RUN go build -o /out/enricher        ./cmd/job_post_analyzer

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/* /app/
COPY migrations/ /app/migrations/
