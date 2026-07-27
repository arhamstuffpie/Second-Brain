.PHONY: build run test vet generate migrate-up migrate-down

BACKEND_DIR := backend

build run test vet generate migrate-up migrate-down:
	$(MAKE) -C $(BACKEND_DIR) $@
