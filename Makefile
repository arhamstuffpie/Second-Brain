.PHONY: build run test vet generate migrate-validate migrate-up migrate-down speaker-up speaker-down speaker-logs face-models face-up face-down face-logs audio-models ml-up ml-down ml-logs

BACKEND_DIR := backend

build run test vet generate migrate-validate migrate-up migrate-down:
	$(MAKE) -C $(BACKEND_DIR) $@

speaker-up:
	docker compose -f speaker-embedder/compose.yaml up --build -d

speaker-down:
	docker compose -f speaker-embedder/compose.yaml down

speaker-logs:
	docker compose -f speaker-embedder/compose.yaml logs -f speaker-embedder

face-models:
	./face-embedder/download-models.sh

face-up:
	docker compose -f face-embedder/compose.yaml up --build -d

face-down:
	docker compose -f face-embedder/compose.yaml down

face-logs:
	docker compose -f face-embedder/compose.yaml logs -f face-embedder

audio-models:
	uv run --with huggingface-hub==0.36.0 python audio-analyzer/download-models.py

ml-up:
	docker compose -f compose.ml.yaml up --build -d

ml-down:
	docker compose -f compose.ml.yaml down

ml-logs:
	docker compose -f compose.ml.yaml logs -f
