# ── Matchmaker Service ───────────────────────────────────────────────
# Multi-stage build: compile in Go image, run in minimal Alpine.
# Uses GOWORK=off to build with go.mod replace directives only.

FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Copy shared pkg module first for layer caching.
COPY pkg/go.mod pkg/go.sum pkg/
RUN cd pkg && go mod download

# Copy matchmaker module.
COPY services/matchmaker/go.mod services/matchmaker/go.sum services/matchmaker/
RUN cd services/matchmaker && GOWORK=off go mod download

# Copy all source code.
COPY pkg/ pkg/
COPY services/matchmaker/ services/matchmaker/

# Build with GOWORK=off — uses replace directive in go.mod for local pkg.
RUN cd services/matchmaker && \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /matchmaker ./cmd/...

# ── Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /matchmaker /usr/local/bin/matchmaker

EXPOSE 50051

ENTRYPOINT ["matchmaker"]
