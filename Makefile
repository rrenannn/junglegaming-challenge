GO_IMAGE ?= golang:1.27.1
GO_RUN = docker run --rm -v $(CURDIR):/workspace -w /workspace $(GO_IMAGE)

.PHONY: up down logs build tidy fmt test test-race vet check

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f app

build:
	docker compose build app

tidy:
	$(GO_RUN) go mod tidy

fmt:
	$(GO_RUN) gofmt -w cmd internal

test:
	$(GO_RUN) go test ./...

test-race:
	$(GO_RUN) go test -race ./...

vet:
	$(GO_RUN) go vet ./...

check: fmt test-race vet
