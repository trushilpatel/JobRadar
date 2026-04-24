# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod ./
# go.sum may not exist before first `go mod tidy`; copy if present.
COPY . ./
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/supabase-audit-sync ./

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/supabase-audit-sync /supabase-audit-sync
USER 65532:65532
ENTRYPOINT ["/supabase-audit-sync"]
