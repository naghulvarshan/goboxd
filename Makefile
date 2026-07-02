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

run-integration:
	$(COMPOSE) up -d goboxd
	$(COMPOSE) --profile tools run --rm -e GOBOXD_URL=http://goboxd:8080 tools go test -tags=integration ./tests/... -v $(if $(TEST_FLAG),-run TestIntegration/$(TEST_FLAG)) -timeout 300s
	$(COMPOSE) stop goboxd

lint:
	$(TOOLS) golangci-lint run ./...

shell:
	@docker compose run --rm goboxd bash

build_and_run: build run
