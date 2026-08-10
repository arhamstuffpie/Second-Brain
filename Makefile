.PHONY: build run test vet generate migrate-validate migrate-up migrate-down speaker-up speaker-down speaker-logs

BACKEND_DIR := backend

build run test vet generate migrate-validate migrate-up migrate-down:
	$(MAKE) -C $(BACKEND_DIR) $@

speaker-up:
	docker compose -f speaker-embedder/compose.yaml up --build -d

speaker-down:
	docker compose -f speaker-embedder/compose.yaml down

speaker-logs:
	docker compose -f speaker-embedder/compose.yaml logs -f speaker-embedder
