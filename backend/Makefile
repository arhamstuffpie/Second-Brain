.PHONY: build run test vet generate migrate-validate migrate-up migrate-down

DATABASE_URL ?=postgresql://postgres:mysecretpassword@localhost:5433/mysecondbrain

build:
	@mkdir -p bin
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test -race ./...

vet:
	go vet ./...

generate:
	sqlc generate

migrate-validate:
	goose -dir db/migrations validate

migrate-up:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is required"; exit 1)
	goose -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is required"; exit 1)
	goose -dir db/migrations postgres "$(DATABASE_URL)" down
