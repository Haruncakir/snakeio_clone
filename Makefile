.PHONY: proto build test lint docker-up docker-down clean

# -------------------------------------------------------
# Protobuf code generation
# Outputs all generated Go code to pkg/gen/pb/
# -------------------------------------------------------
PROTO_SRC_DIR  := proto
PROTO_OUT_DIR  := pkg/gen/pb

proto:
	@echo "==> Generating Protobuf Go code..."
	@mkdir -p $(PROTO_OUT_DIR)
	protoc \
		--proto_path=$(PROTO_SRC_DIR) \
		--go_out=$(PROTO_OUT_DIR) \
		--go_opt=module=github.com/Haruncakir/snakeio_clone/pkg/gen/pb \
		--go-grpc_out=$(PROTO_OUT_DIR) \
		--go-grpc_opt=module=github.com/Haruncakir/snakeio_clone/pkg/gen/pb \
		$(PROTO_SRC_DIR)/matchmaker/matchmaker.proto \
		$(PROTO_SRC_DIR)/gamenode/gamenode.proto \
		$(PROTO_SRC_DIR)/game/state.proto
	@echo "==> Protobuf generation complete."

# -------------------------------------------------------
# Build all services
# -------------------------------------------------------
build:
	@echo "==> Building all services..."
	go build ./services/matchmaker/cmd/...
	go build ./services/gamenode/cmd/...
	go build ./services/gateway/cmd/...
	go build ./loadtest/cmd/...
	@echo "==> Build complete."

# -------------------------------------------------------
# Test all modules
# -------------------------------------------------------
test:
	@echo "==> Running tests..."
	go test ./pkg/... ./services/... ./loadtest/... -v -race
	@echo "==> Tests complete."

# -------------------------------------------------------
# Lint (requires golangci-lint)
# -------------------------------------------------------
lint:
	@echo "==> Linting..."
	golangci-lint run ./...
	@echo "==> Lint complete."

# -------------------------------------------------------
# Docker Compose shortcuts
# -------------------------------------------------------
docker-up:
	docker compose up -d

docker-down:
	docker compose down

# -------------------------------------------------------
# Clean build artifacts
# -------------------------------------------------------
clean:
	go clean ./...
	rm -rf $(PROTO_OUT_DIR)
