# ── Game Node Service ────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY pkg/go.mod pkg/go.sum pkg/
RUN cd pkg && go mod download

COPY services/gamenode/go.mod services/gamenode/go.sum services/gamenode/
RUN cd services/gamenode && GOWORK=off go mod download

COPY pkg/ pkg/
COPY services/gamenode/ services/gamenode/

RUN cd services/gamenode && \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /gamenode ./cmd/...

# ── Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /gamenode /usr/local/bin/gamenode

EXPOSE 8080 50051

ENTRYPOINT ["gamenode"]
