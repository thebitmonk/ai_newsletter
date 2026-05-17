.PHONY: help up down logs migrate-up migrate-down migrate-create migrate-version build run test tidy

DATABASE_URL ?= postgres://ai_newsletter:ai_newsletter@localhost:5433/ai_newsletter?sslmode=disable
MIGRATIONS_DIR := db/migrations
MIGRATE := go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate

help:
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?##"} {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

up: ## Bring up local Postgres
	docker compose up -d

down: ## Bring down local Postgres
	docker compose down

logs: ## Tail Postgres logs
	docker compose logs -f postgres

migrate-up: ## Apply all pending migrations
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

migrate-down: ## Roll back the most recent migration
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

migrate-create: ## Create a new migration (NAME=description_in_snake_case)
	@if [ -z "$(NAME)" ]; then echo "NAME is required: make migrate-create NAME=add_publications_table"; exit 1; fi
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)

migrate-version: ## Show the current migration version
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" version

build: ## Build the server binary
	go build -o bin/server ./cmd/server

run: ## Run the server (no env loading — pass vars yourself)
	go run ./cmd/server

run-env: ## Run the server with .env loaded
	@set -a && . ./.env && set +a && go run ./cmd/server

test: ## Run all tests
	go test ./...

tidy: ## Tidy go modules
	go mod tidy

dev: ## Run backend + frontend concurrently for local dev (Ctrl-C stops both)
	@command -v npm >/dev/null || { echo "npm required"; exit 1; }
	@cd web && [ -d node_modules ] || npm install
	@( set -a && . ./.env && set +a && go run ./cmd/server ) & \
	  GO_PID=$$!; \
	  ( cd web && npm run dev ) & \
	  NUXT_PID=$$!; \
	  trap "kill $$GO_PID $$NUXT_PID 2>/dev/null" INT TERM; \
	  wait

web-install: ## Install frontend dependencies
	cd web && npm install

web-test: ## Run frontend vitest suite
	cd web && npm test

e2e-up: ## Bring up the full stack for Playwright e2e
	./scripts/e2e-up.sh

e2e-down: ## Tear down the e2e stack
	./scripts/e2e-down.sh

e2e: ## Run Playwright e2e against an already-up stack (run e2e-up first)
	cd web && FIREBASE_PROJECT_ID=$${FIREBASE_PROJECT_ID:-ai-newsletter-dev} npx playwright test
