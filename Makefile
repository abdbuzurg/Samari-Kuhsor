# Samari Kuhsor platform.
#
# `make check` is the gate. Nothing moves to the next task with it red.
# See TASKS.md for the task list and docs/07-IMPLEMENTATION-PLAN.md for the decisions.

SHELL := /bin/bash
.DEFAULT_GOAL := help

DC          := docker compose
DB_URL      ?= postgres://samari:samari_dev_only@127.0.0.1:5433/samari?sslmode=disable
MIGRATIONS  := backend/migrations

export DB_URL

.PHONY: help up down db-version db-shell psql migrate-up migrate-down migrate-status \
        gen sqlc tygo seed test test-go test-web build check fmt tidy clean

## ----------------------------------------------------------------------------
## Environment
## ----------------------------------------------------------------------------

up: ## Start the dev stack (Postgres 18) and wait for it to be healthy
	@$(DC) up -d --wait
	@echo "postgres ready on 127.0.0.1:5433"

down: ## Stop the dev stack, keeping the data volume
	@$(DC) down

clean: ## Stop the dev stack and DESTROY the data volume
	@$(DC) down -v

db-version: ## Print the running server version (must be 18.x — see docs/07 C5/I7)
	@$(DC) exec -T db psql -U samari -d samari -tAc "select version();"

db-shell psql: ## Open a psql session against the dev database
	@$(DC) exec db psql -U samari -d samari

## ----------------------------------------------------------------------------
## Schema
## ----------------------------------------------------------------------------

migrate-up: ## Apply all pending migrations
	@goose -dir $(MIGRATIONS) postgres "$(DB_URL)" up

migrate-down: ## Roll back one migration
	@goose -dir $(MIGRATIONS) postgres "$(DB_URL)" down

migrate-status: ## Show migration status
	@goose -dir $(MIGRATIONS) postgres "$(DB_URL)" status

## ----------------------------------------------------------------------------
## Code generation
## ----------------------------------------------------------------------------

gen: extract sqlc tygo ## Run all code generation

extract: ## Re-extract assets, translations and design tokens from design/
	@node tools/extract-website.mjs
	@node tools/extract-crm.mjs

sqlc: ## Regenerate database access code from queries/
	@if [ -f backend/sqlc.yaml ]; then cd backend && sqlc generate; else echo "sqlc: not wired yet (T03)"; fi

tygo: ## Regenerate packages/types from Go DTOs
	@if [ -f backend/tygo.yaml ]; then cd backend && tygo generate; else echo "tygo: not wired yet (T08)"; fi

seed: ## Load reference (production-safe) seed data
	@if [ -d backend/cmd/seed ]; then cd backend && go run ./cmd/seed reference; else echo "seed: not wired yet (T09)"; fi

## ----------------------------------------------------------------------------
## Tests
## ----------------------------------------------------------------------------

test: test-go test-web ## Run all tests

test-go: ## Go unit + integration tests (spins its own Postgres 18 testcontainer)
	@cd backend && go test ./...

test-web: ## Frontend unit/component tests
	@if [ -d apps/crm/node_modules ] || [ -d node_modules ]; then npm run test --workspaces --if-present; \
	else echo "web tests: workspaces not installed yet"; fi

build: ## Build both Next.js apps
	@if [ -d node_modules ]; then npm run build --workspaces --if-present; else echo "build: workspaces not installed yet"; fi

fmt: ## Format Go code
	@cd backend && go fmt ./...

tidy: ## Tidy Go modules
	@cd backend && go mod tidy

## ----------------------------------------------------------------------------
## The gate
## ----------------------------------------------------------------------------

check: ## THE GATE — everything that must be green before the next task opens
	@set -e; \
	echo "==> go vet";        cd backend && go vet ./... && cd ..; \
	echo "==> go test";       $(MAKE) --no-print-directory test-go; \
	echo "==> extraction";    $(MAKE) --no-print-directory _check-extraction; \
	echo "==> sqlc staleness"; $(MAKE) --no-print-directory _check-sqlc; \
	echo "==> tygo staleness"; $(MAKE) --no-print-directory _check-tygo; \
	echo "==> web tests";     $(MAKE) --no-print-directory test-web; \
	echo "==> next build";    $(MAKE) --no-print-directory build; \
	echo; echo "check: GREEN"

# Everything derived from design/ must match design/. The extractors assert the
# semantic invariants (locale shape, no `tj` key, the CLAUDE.md §5 design contract)
# and exit non-zero on violation; git diff then catches silent drift in the output.
_check-extraction:
	@node tools/extract-website.mjs >/dev/null
	@node tools/extract-crm.mjs >/dev/null
	@if ! git diff --quiet -- apps/crm/messages apps/crm/app/styles/theme.css apps/web/public/assets apps/web/.reference; then \
		echo "extraction: output differs from design/ — run 'make extract' and review the diff"; exit 1; fi

# Generated code must be committed and current. A drift here is the silent bug
# docs/03-API-CONTRACT.md:265 singles out as the most likely in this architecture.
_check-sqlc:
	@if [ -f backend/sqlc.yaml ]; then \
		cd backend && sqlc diff || { echo "sqlc: generated code is stale — run 'make sqlc'"; exit 1; }; \
	else echo "   (sqlc not wired yet — T03)"; fi

_check-tygo:
	@if [ -f backend/tygo.yaml ]; then \
		$(MAKE) --no-print-directory tygo; \
		if ! git diff --quiet -- packages/types; then \
			echo "tygo: packages/types is stale — run 'make tygo' and commit"; exit 1; fi; \
	else echo "   (tygo not wired yet — T08)"; fi

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
