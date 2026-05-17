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

run: ## Run the server
	go run ./cmd/server

test: ## Run all tests
	go test ./...

tidy: ## Tidy go modules
	go mod tidy
