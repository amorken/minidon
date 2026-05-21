.PHONY: build run test lint web dev-web dev-go docker compose-up compose-down clean help

# ── Configuration ─────────────────────────────────────────────────────────────
BINARY     := bin/minidon
CMD        := ./cmd/minidon
GOFLAGS    := CGO_ENABLED=0
LDFLAGS    := -trimpath -ldflags="-s -w"
WEB_DIR    := web

# ── Default target ────────────────────────────────────────────────────────────
all: web build

## build: compile the Go binary (embeds web/dist)
build:
	@mkdir -p bin
	$(GOFLAGS) go build $(LDFLAGS) -o $(BINARY) $(CMD)
	@echo "Built → $(BINARY)"

## run: build and run the binary (set MINIDON_MASTODON_INSTANCE before running)
run: build
	./$(BINARY)

## test: run all Go tests
test:
	go test ./...

## lint: run go vet and staticcheck
lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed; skipping"

## web: install Node deps and build the frontend
web:
	cd $(WEB_DIR) && npm ci && npm run build

## dev-web: start the Vite dev server (hot module replacement)
dev-web:
	cd $(WEB_DIR) && npm run dev

## dev-go: run the Go server directly (for development, alongside Vite)
dev-go:
	go run $(CMD)

## docker: build the Docker image
docker:
	docker build -f deploy/Dockerfile -t minidon:latest .

## compose-up: start all services with Docker Compose
compose-up:
	docker compose -f deploy/docker-compose.yml up --build

## compose-down: stop and remove Compose services
compose-down:
	docker compose -f deploy/docker-compose.yml down

## clean: remove build artifacts
clean:
	rm -rf bin/ $(WEB_DIR)/dist/ $(WEB_DIR)/.vite/

## help: show this help message
help:
	@grep -E '^## ' Makefile | sed 's/^## /  /'
