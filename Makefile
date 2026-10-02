GO_IMAGE ?= golang:1.27.1
GO_RUN = docker run --rm -v $(CURDIR):/workspace -w /workspace $(GO_IMAGE)

.PHONY: up down logs build tidy fmt test test-race vet check migrate-up migrate-down token-provider-a token-provider-b token-internal

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

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

KEYCLOAK_TOKEN_URL ?= http://localhost:$(or $(KEYCLOAK_PORT),8081)/realms/jungle-gaming/protocol/openid-connect/token

token-provider-a:
	@curl -s -X POST $(KEYCLOAK_TOKEN_URL) \
	  -d grant_type=client_credentials -d client_id=provider-a -d client_secret=provider-a-local-secret | jq -r .access_token

token-provider-b:
	@curl -s -X POST $(KEYCLOAK_TOKEN_URL) \
	  -d grant_type=client_credentials -d client_id=provider-b -d client_secret=provider-b-local-secret | jq -r .access_token

token-internal:
	@curl -s -X POST $(KEYCLOAK_TOKEN_URL) \
	  -d grant_type=client_credentials -d client_id=wallet-service -d client_secret=wallet-service-local-secret | jq -r .access_token
