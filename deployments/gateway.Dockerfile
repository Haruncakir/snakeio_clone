# ── API Gateway Service ──────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY pkg/go.mod pkg/go.sum pkg/
RUN cd pkg && go mod download

COPY services/gateway/go.mod services/gateway/go.sum services/gateway/
RUN cd services/gateway && GOWORK=off go mod download

COPY pkg/ pkg/
COPY services/gateway/ services/gateway/

RUN cd services/gateway && \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /gateway ./cmd/...

# ── Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /gateway /usr/local/bin/gateway

EXPOSE 8080

ENTRYPOINT ["gateway"]
