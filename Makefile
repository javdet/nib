.PHONY: up down restart logs psql dev-up dev-down dev-restart dev-logs migrate-up migrate-down migrate-create run-backend run-frontend dev

COMPOSE_DEV = docker compose -f docker-compose.dev.yml

# --- Docker (production images) ---

up: ## Start Nib with published images (see .env.example)
	docker compose up -d

down: ## Stop production compose stack
	docker compose down

restart: ## Restart production compose stack
	docker compose down && docker compose up -d

logs: ## Tail production compose logs
	docker compose logs -f

# --- Docker (local development) ---

dev-up: ## Start dev stack (go run + bun dev)
	$(COMPOSE_DEV) up -d

dev-down: ## Stop dev stack
	$(COMPOSE_DEV) down

dev-restart: ## Restart dev stack
	$(COMPOSE_DEV) down && $(COMPOSE_DEV) up -d

dev-logs: ## Tail dev stack logs
	$(COMPOSE_DEV) logs -f

psql: ## Open psql shell (production stack)
	docker compose exec postgres psql -U nib -d nib

dev-psql: ## Open psql shell (dev stack)
	$(COMPOSE_DEV) exec postgres psql -U nib -d nib

# --- Migrations (golang-migrate) ---

MIGRATE_DSN ?= "postgres://nib:nib@localhost:5432/nib?sslmode=disable"

migrate-up: ## Run all up migrations
	migrate -path backend/migrations -database $(MIGRATE_DSN) up

migrate-down: ## Roll back the last migration
	migrate -path backend/migrations -database $(MIGRATE_DSN) down 1

migrate-create: ## Create a new migration pair (usage: make migrate-create NAME=create_foo)
	migrate create -ext sql -dir backend/migrations -seq $(NAME)

# --- Backend ---

run-backend: ## Run the Go backend
	cd backend && go run ./cmd/nib

# --- Frontend ---

run-frontend: ## Run the Vite dev server
	cd frontend && bun run dev

# --- Combined ---

dev: dev-up ## Start full dev stack in Docker (or run backend/frontend locally with make run-*)

# --- Help ---

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
