# API Gateway Service
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work
COPY pkg/ pkg/
COPY services/gateway/ services/gateway/
RUN cd services/gateway && go build -o /gateway ./cmd/...

FROM alpine:3.20
COPY --from=builder /gateway /usr/local/bin/gateway
ENTRYPOINT ["gateway"]
