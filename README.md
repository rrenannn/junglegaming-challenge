# Jungle Gaming — Wallet & Wagering Service

A Go backend that manages player wallets and processes wagering operations
(bets, wins, refunds, rollbacks) for third-party game providers, with strict
balance consistency under concurrency, idempotent transaction processing
(both over HTTP and over SQS), and a transactional outbox for publishing
wallet events.

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the system is designed:
layers, domain model, concurrency, idempotency, the outbox pattern, and
known limitations.

## Prerequisites

- Docker and Docker Compose.
- `jq` and `curl`, for the walkthrough below.
- Go 1.27 locally is optional — `make test`, `make test-race`, `make vet`
  and `make fmt` all run inside a `golang` container, so a local Go
  toolchain is not required. `make migrate-up`/`make migrate-down` do
  need a local `go run` since they connect to Postgres on the host-mapped
  port.

## Quickstart

```sh
cp .env.example .env   # defaults work as-is for local use
make up                # docker compose up --build
```

This starts four containers: `postgres`, `keycloak` (OIDC provider,
backed by its own schema in the same Postgres instance), `localstack`
(SQS emulation — the FIFO queues are created automatically by
`deploy/localstack/init/`), and `app`.

Wait for `app` to log `"application started"`, then apply migrations:

```sh
make migrate-up
```

The app itself does **not** apply migrations on startup — this is a
deliberate choice so that schema changes are an explicit, reviewable step,
never implicit in a container restart. `make migrate-down` reverts exactly
one migration step at a time (`migrate.Steps(-1)`), not the whole schema.

Check it's alive:

```sh
curl localhost:8080/health/live
curl localhost:8080/health/ready   # checks Postgres and SQS independently
```

## Authentication

All endpoints require a Keycloak-issued JWT (`Authorization: Bearer <token>`).
Three OIDC clients exist, each with `client_credentials` grant only:

| Client | Scopes | Used for |
|---|---|---|
| `provider-a` | `wagering.read`, `wagering.write` | a game provider submitting/reading its own wager transactions |
| `provider-b` | `wagering.read`, `wagering.write` | a second, isolated game provider |
| `wallet-service` | `wagering.read`, `wallets.manage` | the internal caller that manages wallets |

Fetch a token:

```sh
make token-provider-a     # or token-provider-b, token-internal
```

Provider tokens carry a `provider_id` claim; the wagering endpoints reject
any request whose body `providerId` doesn't match the authenticated
provider — one provider can never submit or read another provider's
transactions.

## API

| Method & path | Scope required | Description |
|---|---|---|
| `POST /wallets` | `wallets.manage` | Open a wallet for a player in a currency, with an optional opening balance |
| `GET /wallets/{walletId}` | `wallets.manage` | Get a wallet's current balance and version |
| `GET /wallets/{walletId}/ledger` | `wallets.manage` | Paginated ledger entries for a wallet (`?cursor=&limit=`) |
| `POST /wallets/{walletId}/reconciliation` | `wallets.manage` | Recompute the balance from the ledger and compare it against the stored balance |
| `POST /wagering/transactions` | `wagering.write` | Submit a wager operation (`BET`, `WIN`, `REFUND`, `ROLLBACK`) |
| `GET /wagering/transactions/{transactionId}` | `wagering.read` | Get a wager transaction by id |
| `GET /metrics` | none | Prometheus metrics |

Supported currencies: `BRL`, `USD`, `EUR` (see `internal/domain/money.go`).
A wallet is opened in one currency; every operation against it must match.
Adding another currency with a two-decimal minor unit is a one-line change
to `supportedCurrencies` — a currency with a different scale (e.g. a
three-decimal one) would also need `moneyScale`/`ParseMoney`/`Decimal` to
become per-currency.

## Walkthrough: open a wallet, place a bet, check the ledger

```sh
INTERNAL_TOKEN=$(make -s token-internal)
PROVIDER_TOKEN=$(make -s token-provider-a)

# 1. Open a wallet with an opening balance of R$ 100.00
WALLET_ID=$(curl -s -X POST localhost:8080/wallets \
  -H "Authorization: Bearer $INTERNAL_TOKEN" -H "Content-Type: application/json" \
  -d '{"playerId":"player-1","currency":"BRL","openingBalance":"100.00"}' | jq -r .id)

# 2. Place a R$ 20.00 bet as provider-a
curl -s -X POST localhost:8080/wagering/transactions \
  -H "Authorization: Bearer $PROVIDER_TOKEN" -H "Content-Type: application/json" \
  -d "{\"providerId\":\"provider-a\",\"playerId\":\"player-1\",\"walletId\":\"$WALLET_ID\",\
\"roundId\":\"round-1\",\"gameId\":\"game-1\",\"kind\":\"BET\",\"currency\":\"BRL\",\
\"amount\":\"20.00\",\"externalTransactionId\":\"ext-1\",\"idempotencyKey\":\"key-1\"}"

# 3. Check the wallet balance and the ledger
curl -s localhost:8080/wallets/$WALLET_ID -H "Authorization: Bearer $INTERNAL_TOKEN"
curl -s localhost:8080/wallets/$WALLET_ID/ledger -H "Authorization: Bearer $INTERNAL_TOKEN"
```

Resubmitting the same request body (same `idempotencyKey`) returns the
original result without debiting the wallet again — this holds for both
exact retries and re-deliveries, and is covered in detail in
[ARCHITECTURE.md](ARCHITECTURE.md#idempotency).

`REFUND` and `ROLLBACK` reference an earlier operation via
`referenceExternalTransactionId`. If that reference hasn't been seen yet,
the transaction is accepted with status `PENDING_REFERENCE` and resolved
asynchronously once the referenced transaction arrives (or expires after
enough retries) — no synchronous dependency on provider message ordering.

## Submitting a wager over SQS instead of HTTP

The same wager processing logic is exposed over the `wager-transactions.fifo`
queue (FIFO, so `MessageGroupId` is required). This is how a provider that
prefers async delivery — or a backfill/replay — would submit operations:

```sh
docker compose exec localstack awslocal sqs send-message \
  --queue-url http://localhost:4566/000000000000/wager-transactions.fifo \
  --message-group-id player-1 \
  --message-deduplication-id ext-2 \
  --message-body '{
    "providerId": "provider-a", "playerId": "player-1", "walletId": "'"$WALLET_ID"'",
    "roundId": "round-1", "gameId": "game-1", "kind": "BET",
    "currency": "BRL", "amount": "5.00",
    "externalTransactionId": "ext-2", "idempotencyKey": "key-2"
  }'
```

Every message is deduplicated twice: once at the transport level by
`(consumerName, messageId)` (so SQS's own at-least-once redelivery never
double-processes), and once at the business level by
`(providerId, idempotencyKey)` using the same canonical hash the HTTP path
uses — so the same logical operation submitted once over HTTP and once
over SQS is still only applied once.

A message that fails processing with a retryable error is redelivered up
to 5 times, then moves to `wager-transactions-dlq.fifo`. Inspect it with:

```sh
docker compose exec localstack awslocal sqs get-queue-attributes \
  --queue-url http://localhost:4566/000000000000/wager-transactions-dlq.fifo \
  --attribute-names ApproximateNumberOfMessages
```

Wallet events (`wallet.credited`, `wallet.debited`, etc.) are published to
`wager-events.fifo` via the transactional outbox — read them the same way,
substituting that queue's URL.

## Running multiple instances

```sh
make scale-up    # 3 app replicas against the same Postgres/SQS/Keycloak
make scale-down
```

This proves the SQS consumer, the outbox publisher and the pending-reference
worker are all safe under real multi-instance concurrency — not just safe
within a single process. `compose.scale.yaml` overrides the app's fixed
host port with an ephemeral one per replica (Compose merges list fields
across `-f` files by default, so this uses the `!override` merge tag rather
than relying on replacement). Use `docker compose port app <N>` or
`docker ps` to find each replica's port; container logs are read with
`docker logs jungle-gaming-app-<N>` (not `docker compose logs`, since the
service name stays `app` when scaled).

## Simulating failures

These are the exact recovery scenarios this project was manually verified
against:

```sh
# Postgres outage: the app keeps serving /health/live, /health/ready
# reports postgres unavailable, and in-flight requests fail cleanly —
# nothing crashes, and everything recovers once Postgres is back.
docker compose pause postgres
curl localhost:8080/health/ready
docker compose unpause postgres

# SQS/LocalStack outage: same shape — the consumer and outbox publisher
# log errors and keep retrying, nothing is lost.
docker compose pause localstack
curl localhost:8080/health/ready
docker compose unpause localstack

# Crash during processing: SIGKILL loses no committed work, because every
# state change is committed transactionally before being acknowledged.
docker kill -s SIGKILL jungle-gaming-app-1
make up

# Graceful shutdown: SIGTERM lets in-flight requests and worker loops
# finish before the process exits (stop_grace_period: 20s in compose.yaml).
docker compose stop app
```

## Load testing

```sh
make loadtest   # 30s, 20 workers, 10 wallets — override with LOADTEST_*
```

`cmd/loadtest` opens `LOADTEST_WALLETS` wallets with a large balance, then
has `LOADTEST_CONCURRENCY` workers submit unique `BET`s (unique
idempotency key and external id each — this measures raw throughput, not
dedup) round-robin across those wallets for `LOADTEST_DURATION`, and
reports throughput and latency percentiles. Spreading load across several
wallets avoids making the per-wallet lock the only thing being measured —
contention *within* one wallet is already covered by the race-detected
concurrency tests.

A representative local run (`LOADTEST_DURATION=15s LOADTEST_CONCURRENCY=20
LOADTEST_WALLETS=10`, against the same `docker compose up --build` stack
described above, 2026):

```
requests: 52886 (success=52886, failed=0)
throughput: 3525.7 req/s
latency: p50=4.4ms p95=12.9ms p99=23.3ms max=80.9ms
```

## Metrics

`GET /metrics` exposes, among the standard Go/process metrics:

| Metric | Type | Meaning |
|---|---|---|
| `wager_operations_total{kind,status}` | counter | wager operations by kind and final status |
| `wager_operation_duration_seconds{kind}` | histogram | end-to-end processing time per wager operation |
| `wager_replays_total` | counter | requests that were a deduplicated replay of an already-processed operation |
| `wager_conflicts_total{reason}` | counter | idempotency key or inbox hash conflicts (same key, different content) |
| `reconciliation_divergences_total` | counter | reconciliations that found the ledger and stored balance disagree |
| `outbox_pending_events` / `outbox_pending_oldest_age_seconds` | gauge | backlog and staleness of unpublished outbox events |
| `sqs_messages_processed_total{result}` | counter | SQS messages processed, by success/error |

Not instrumented, documented here instead: `external_transaction_id`
conflicts under concurrent writers (a rare race, see
[ARCHITECTURE.md](ARCHITECTURE.md#known-limitations)) and DLQ depth (not
observed by the app — inspect it directly, as shown above).

## Environment variables

See [`.env.example`](.env.example) for the full list with working local
defaults — application config (`APP_*`, `HTTP_*`), database connection,
LocalStack/SQS endpoint and queue names, and the Keycloak/OIDC settings
for all three clients.

## Tests

```sh
make test        # go test ./...
make test-race   # go test -race ./... (includes concurrency/scaling tests)
make vet
make fmt
make check       # fmt + test-race + vet
```
