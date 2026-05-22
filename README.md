# Snake.io — Distributed Multiplayer Backend

A production-ready, horizontally scalable backend for a multiplayer Snake.io clone, built with Go microservices.

## Architecture

```
┌──────────────┐     REST      ┌───────────┐    gRPC     ┌──────────────┐
│    Client     │──────────────▶│  Gateway  │────────────▶│  Matchmaker  │
│  (Browser)   │               │  :8080    │             │  :50051      │
└──────┬───────┘               └───────────┘             └──────┬───────┘
       │                                                        │
       │  Direct WebSocket                          Registers / │
       │  (binary protobuf)                         Heartbeats  │
       │                                                        │
       ▼                                                        ▼
┌──────────────┐                                   ┌──────────────────┐
│  Game Node   │◀──────────────────────────────────│  Node Registry   │
│  :8080/:50051│   AssignPlayer (gRPC)             │  (in-memory)     │
└──────────────┘                                   └──────────────────┘
```

**Key design decisions:**
- **No WebSocket proxy** — clients connect directly to Game Nodes for zero-hop game ticks
- **Binary Protobuf** over WebSocket for minimal packet size (~50ms tick rate)
- **Least-loaded node selection** — Matchmaker picks the node with most spare capacity
- **Horizontal scaling** — spin up N Game Nodes, each auto-registers with the Matchmaker

## Services

| Service | Port(s) | Protocol | Purpose |
|---------|---------|----------|---------|
| **Gateway** | 8080 | HTTP/REST | Match requests, leaderboard, player stats |
| **Matchmaker** | 50051 | gRPC | Player queue, node registry, match assignment |
| **Game Node** | 8080, 50051 | WebSocket + gRPC | Real-time game loop, physics, state broadcast |
| **Redis** | 6379 | — | Matchmaking queue persistence |
| **PostgreSQL** | 5432 | — | Player stats, leaderboard |

## Quick Start

```bash
# Clone
git clone https://github.com/Haruncakir/snakeio_clone.git
cd snakeio_clone

# Start everything (2 game nodes by default)
docker compose up -d

# Verify
docker compose ps
curl http://localhost:8080/health

# Request a match
curl -X POST http://localhost:8080/api/match \
  -H "Content-Type: application/json" \
  -d '{"player_id": "player-1"}'

# View leaderboard
curl http://localhost:8080/api/leaderboard

# View logs
docker compose logs -f matchmaker gamenode-1

# Tear down
docker compose down -v
```

## Local Development

### Prerequisites
- Go 1.24+
- Docker & Docker Compose
- protoc (for regenerating proto files)

### Build & Test
```bash
# Build all services
go build github.com/Haruncakir/snakeio_clone/services/matchmaker/...
go build github.com/Haruncakir/snakeio_clone/services/gamenode/...
go build github.com/Haruncakir/snakeio_clone/services/gateway/...

# Run tests (30 tests with race detection)
cd services/matchmaker && go test ./... -race
cd services/gamenode && go test ./... -race

# Regenerate protobuf
make proto

# Start infra only (Redis + Postgres)
docker compose up -d redis postgres
```

### Run Services Locally
```bash
# Terminal 1: Matchmaker
cd services/matchmaker && go run ./cmd/...

# Terminal 2: Game Node
NODE_ID=local-1 MATCHMAKER_ADDR=localhost:50051 \
  go run ./services/gamenode/cmd/...

# Terminal 3: Gateway
MATCHMAKER_ADDR=localhost:50051 \
  POSTGRES_DSN="postgres://snakeio:snakeio_secret@localhost:5432/snakeio?sslmode=disable" \
  go run ./services/gateway/cmd/...
```

## Load Testing

```bash
# Against a local game node (register tokens first via gRPC)
go run ./loadtest/cmd/main.go \
  -target ws://localhost:8081/ws \
  -clients 500 \
  -duration 30s \
  -rampup 5s
```

## Project Structure

```
snakeio_clone/
├── proto/                    # Protobuf definitions
│   ├── matchmaker/           #   Matchmaker gRPC service
│   ├── gamenode/             #   GameNode gRPC service
│   └── game/                 #   Game state messages (WebSocket)
├── pkg/                      # Shared Go packages
│   ├── gen/pb/               #   Generated protobuf code
│   ├── config/               #   Environment-based config
│   ├── logger/               #   Structured JSON logging
│   └── db/                   #   Redis & Postgres factories
├── services/
│   ├── matchmaker/           # Matchmaker microservice
│   ├── gamenode/             # Game Node microservice
│   └── gateway/              # API Gateway (REST-only)
├── loadtest/                 # WebSocket load tester
├── deployments/              # Dockerfiles
├── scripts/                  # DB init scripts
├── docker-compose.yml        # Full stack deployment
├── Makefile                  # Build & codegen targets
└── go.work                   # Go workspace
```

## Configuration

All services are configured via environment variables:

| Variable | Default | Used By |
|----------|---------|---------|
| `REDIS_ADDR` | `localhost:6379` | Matchmaker |
| `POSTGRES_DSN` | `postgres://snakeio:...@localhost:5432/snakeio` | Gateway |
| `GRPC_PORT` | `50051` | Matchmaker, Game Node |
| `HTTP_PORT` | `8080` | Game Node, Gateway |
| `NODE_ID` | `node-1` | Game Node |
| `MATCHMAKER_ADDR` | `localhost:50051` | Game Node, Gateway |
| `TICK_RATE` | `50ms` | Game Node |
| `ARENA_WIDTH` | `2000` | Game Node |
| `ARENA_HEIGHT` | `2000` | Game Node |
| `MAX_PLAYERS` | `50` | Game Node |
| `HEARTBEAT_INTERVAL` | `5s` | Matchmaker, Game Node |
| `LOG_LEVEL` | `info` | All services |

## License

GPL-3.0 — see [LICENSE](LICENSE).