.PHONY: up down restart logs psql dev-up dev-down dev-restart dev-logs migrate-up migrate-down migrate-create run-backend run-frontend dev buildx-setup images-local images-push sync-docs check-docs-sync

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

# --- Shipped docs ---

# CLAUDE.md is //go:embed-ed so the agent can answer questions about nib itself,
# but the backend image is built from the `backend` context, which cannot reach
# the repository root. The copy is therefore synced before every image build and
# guarded by TestRepoGuideMatchesRepoRoot. The copy is not named claude.md so a
# CLAUDE.md line in .gitignore cannot swallow it on a case-insensitive checkout.
EMBEDDED_CLAUDE_MD = backend/internal/nibdocs/docs/repo-guide.md

sync-docs: ## Copy repo-root CLAUDE.md into the backend build context
	@if [ -f CLAUDE.md ]; then \
		cp CLAUDE.md $(EMBEDDED_CLAUDE_MD) && echo "synced CLAUDE.md -> $(EMBEDDED_CLAUDE_MD)"; \
	else \
		echo "CLAUDE.md is not in this tree; keeping the embedded copy unchanged"; \
	fi

check-docs-sync: ## Fail when the embedded copy of CLAUDE.md has drifted
	diff -u $(EMBEDDED_CLAUDE_MD) CLAUDE.md

# --- Docker images ---

VERSION   ?= $(shell tr -d '[:space:]' < VERSION)
REGISTRY  ?= javdet
PLATFORMS ?= linux/amd64,linux/arm64
BUILDER   ?= nib-multiarch

buildx-setup: ## Create the docker-container builder needed for multi-arch builds
	@docker buildx inspect $(BUILDER) >/dev/null 2>&1 \
		|| docker buildx create --name $(BUILDER) --driver docker-container --bootstrap

images-local: sync-docs ## Build all three images for THIS machine's arch into the local daemon
	docker buildx build --load -t $(REGISTRY)/nib-backend:$(VERSION) \
		--build-arg VERSION=$(VERSION) -f backend/Dockerfile backend
	docker buildx build --load -t $(REGISTRY)/nib-kb:$(VERSION) \
		-f backend/Dockerfile.kb backend
	docker buildx build --load -t $(REGISTRY)/nib-frontend:$(VERSION) \
		-f frontend/Dockerfile frontend

images-push: buildx-setup sync-docs ## Build all three images for $(PLATFORMS) and push (needs `docker login`)
	docker buildx build --builder $(BUILDER) --platform $(PLATFORMS) --push \
		-t $(REGISTRY)/nib-backend:$(VERSION) -t $(REGISTRY)/nib-backend:latest \
		--build-arg VERSION=$(VERSION) -f backend/Dockerfile backend
	docker buildx build --builder $(BUILDER) --platform $(PLATFORMS) --push \
		-t $(REGISTRY)/nib-kb:$(VERSION) -t $(REGISTRY)/nib-kb:latest \
		-f backend/Dockerfile.kb backend
	docker buildx build --builder $(BUILDER) --platform $(PLATFORMS) --push \
		-t $(REGISTRY)/nib-frontend:$(VERSION) -t $(REGISTRY)/nib-frontend:latest \
		-f frontend/Dockerfile frontend

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
