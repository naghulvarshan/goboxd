.PHONY: build run test integration run-integration lint

COMPOSE    ?= docker compose
TOOLS      := $(COMPOSE) --profile tools run --rm tools
GOBOXD_URL ?= http://localhost:8080

build:
	$(COMPOSE) build goboxd --build-arg GIT_COMMIT=$(shell git rev-parse --short HEAD)

run:
	$(COMPOSE) up goboxd

test:
	$(TOOLS) go test ./...

integration:
	$(TOOLS) go test -tags=integration ./tests/... -timeout 300s

run-integration:
	@cd tests && GOBOXD_URL=$(GOBOXD_URL) go test -tags=integration -count=1 -v $(if $(TEST_FLAG),-run TestIntegration/$(TEST_FLAG)) -timeout 300s .

lint:
	$(TOOLS) golangci-lint run ./...

shell:
	@docker compose run --rm goboxd bash

build_and_run: build run
