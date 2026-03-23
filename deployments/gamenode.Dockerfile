# Game Node Service
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work
COPY pkg/ pkg/
COPY services/gamenode/ services/gamenode/
RUN cd services/gamenode && go build -o /gamenode ./cmd/...

FROM alpine:3.20
COPY --from=builder /gamenode /usr/local/bin/gamenode
ENTRYPOINT ["gamenode"]
