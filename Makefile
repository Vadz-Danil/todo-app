DB_DSN ?= postgres://postgres:postgres_password@localhost:5433/todo_db?sslmode=disable
MIGRATIONS_DIR ?= ./migrations

.PHONY: help migrate-create migrate-up migrate-down migrate-status run build test clean

migrate-create:
ifndef name
	$(error name is not set. Usage: make migrate-create name=your_migration_name)
endif
	@echo "Creating migration: $(name)"
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

migrate-up:
	@echo "Applying migrations..."
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" up

migrate-down:
	@echo "Rolling back last migration..."
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" down

migrate-status:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" status

run:
	go run cmd/server/main.go

build:
	go build -o bin/server cmd/server/main.go

test:
	go test -v ./...
