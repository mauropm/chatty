# ---- Chatty ----------------------------------------------------------------
BINARY      := chatty
PKG         := ./cmd/chatty
GO          ?= go

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n",$$1,$$2}'

.PHONY: build
build: ## Build the Go binary into ./bin/chatty
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o bin/$(BINARY) $(PKG)

.PHONY: run
run: ## Run locally (needs DATABASE_* + MIMIR_* env or an .env)
	$(GO) run $(PKG)

.PHONY: test
test: ## Run unit tests (no external services required)
	$(GO) test ./...

.PHONY: test-verbose
test-verbose: ## Run all tests verbosely
	$(GO) test -v ./...

.PHONY: cover
cover: ## Run tests with coverage summary
	$(GO) test -coverprofile=coverage.out ./... && $(GO) tool cover -func=coverage.out | tail -1

.PHONY: test-integration
test-integration: ## Integration tests against a Docker Postgres (auto up/down)
	./tests/integration.sh

.PHONY: fmt
fmt: ## gofmt + goimports-style tidy
	$(GO) fmt ./...
	$(GO) mod tidy

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: lint
lint: vet ## Static analysis

.PHONY: docker-build
docker-build: ## Build the Docker image
	DOCKER_BUILDKIT=1 docker build -t chatty:latest .

.PHONY: up
up: ## Start chatty + chatty-db (sudo if required)
	sudo docker compose up -d --build

.PHONY: down
down: ## Stop and remove containers (volumes kept)
	sudo docker compose down

.PHONY: clean-down
clean-down: ## Remove containers and volumes (DESTROYS DATA)
	sudo docker compose down -v

.PHONY: logs
logs: ## Tail application logs
	sudo docker compose logs -f chatty

.PHONY: ps
ps: ## Show container status
	sudo docker compose ps

.PHONY: health
health: ## Print /health output
	@curl -fsS http://localhost:3333/health | python3 -m json.tool
