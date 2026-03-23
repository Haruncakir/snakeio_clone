# Matchmaker Service
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work
COPY pkg/ pkg/
COPY services/matchmaker/ services/matchmaker/
RUN cd services/matchmaker && go build -o /matchmaker ./cmd/...

FROM alpine:3.20
COPY --from=builder /matchmaker /usr/local/bin/matchmaker
ENTRYPOINT ["matchmaker"]
