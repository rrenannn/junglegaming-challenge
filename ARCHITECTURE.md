# Architecture

How this system is actually built — not a plan, a description of what's in
the repository. For setup and the API surface, see [README.md](README.md).

## Layers and the dependency rule

```text
                   Input adapters
             +-------------------------------+
             | HTTP Handler | SQS Handler    |
             | Ref. Worker  | Outbox Worker  |
             +---------------+---------------+
                             |
                             v
                Application Services/Use Cases
                             |
                        domain rules
                             |
                             v
                 Repository/Publisher Ports
                             ^
                             |
             +---------------+---------------+
             | PostgreSQL | SQS | Keycloak   |
             +-------------------------------+
                   Output adapters
```

Allowed: `handler/adapter -> application -> domain`,
`infrastructure -> application -> domain`, `app/Fx -> everything` (the
composition root, `internal/app/module.go`, is the only place that wires
concrete implementations together).

Forbidden, and enforced by convention rather than a linter: domain
importing application or infrastructure; an application service importing
`pgx`, the AWS SDK, Fx, or any HTTP type; a repository port depending on
the Postgres implementation; a handler calling a repository directly; one
input adapter calling another input adapter.

The one place this rule gets interesting is metrics
(`internal/application/port/metrics.go`): `ProcessWagerService` and
`ReconcileWalletService` depend on a `port.Metrics` interface, not on
Prometheus. The concrete `*observability.Metrics` (which does import
`promauto`) satisfies that interface and is handed to them by Fx, while
adapters that are already allowed to touch infrastructure (`Consumer`,
`OutboxPublisher`) hold the concrete type directly and call extra methods
the port doesn't expose (`ObserveSQSMessage`, `SetOutboxPending`).

HTTP and SQS have separate handlers, but both call the same
`ProcessWagerService` — the financial rules live in exactly one place
regardless of transport.

## Domain model

- **`Wallet`** — id, player id, currency, balance, version, timestamps.
  `(player_id, currency)` is unique. Version increments only when the
  balance changes. Balance can never go negative (enforced in the domain
  method and again by a DB `CHECK`). Opening a wallet with a positive
  balance creates an `OPENING` transaction and a ledger entry; opening at
  zero creates neither — there's nothing to reconcile for an all-zero
  wallet.
- **`WagerTransaction`** — the record of one financial operation. Kinds:
  `OPENING`, `BET` (debit), `WIN` (credit), `LOSS` (no movement, amount
  must be exactly zero), `REFUND` (credit, full reversal of a processed
  `BET`), `ROLLBACK` (inverse of a processed `BET`, `WIN`, or `REFUND`).
  Statuses: `PENDING`, `PENDING_REFERENCE`, `PROCESSED`, `REJECTED`,
  `FAILED` — the last three are terminal, enforced in the domain type
  (`TransactionStatus.Terminal()`) and mirrored by a DB `CHECK` on the
  allowed values. Rehydrating a transaction from storage never re-runs
  transitions or re-emits events — that path is distinct from creation.
- **`Money`** — `amountMinor int64` plus a validated `Currency`, no
  floats anywhere. The parser rejects empty strings, negative signs on
  external financial input, `NaN`/`Infinity`, scientific notation, more
  than two decimal places, and `int64` overflow. `BRL`, `USD`, and `EUR`
  are valid currencies today (`internal/domain/money.go`) — adding another
  one with the same two-decimal scale is a one-line addition to
  `supportedCurrencies`, not a change to any call site, since `Money`
  carries its own currency through every operation and the three HTTP/SQS
  entry points all validate through the same `domain.NewCurrency`.
- **`LedgerEntry`** — one immutable row per financial movement: direction,
  amount, balance before, balance after. `wallet_ledger_entries` has
  `UNIQUE(wallet_id, transaction_id)`, a `CHECK` that
  `balance_after = balance_before ± amount`, and triggers that raise on
  any `UPDATE` or `DELETE` — the ledger is append-only even against a bug,
  not just by convention. A rejected operation never produces a ledger
  row; neither does `LOSS`, since it has no movement.

### Reversal policy

At most one direct, successful reversal per transaction:

- a processed `BET` can receive a `REFUND` **or** a `ROLLBACK`, never both;
- a processed `WIN` can receive a `ROLLBACK`;
- a processed `REFUND` can receive a `ROLLBACK` — but that rollback
  references the `REFUND` itself, not the original `BET` again;
- partial reversals aren't accepted — the amount must match the
  referenced transaction exactly, and provider, player, wallet, currency
  and round must all agree between the reference and the reversal.

The referenced transaction is locked during processing, and
`wager_transactions_reference_processed_unique` (a partial unique index on
`reference_transaction_id` where `status = 'PROCESSED'`) makes a second
successful direct reversal impossible even if application logic ever had
a bug — the database is the final guard, not just the service.

## Concurrency

Every wallet mutation locks that wallet's row with `SELECT ... FOR UPDATE`
inside the same transaction that reads it, so two operations against the
same wallet always serialize — one blocks until the other commits or rolls
back. Operations against different wallets never contend. This was
verified under load with multiple goroutines racing on the same wallet
(`internal/application/service` tests) and with three full application
instances racing on shared Postgres/SQS (`make scale-up`): 20 concurrent
`BET`s against one wallet produced exactly the expected final balance and
exactly the expected number of ledger rows, split across replicas in
whatever order each happened to claim work, with no lost or double-applied
update.

## Idempotency — two layers

**Transport-level (inbox).** Every SQS message is identified by
`(consumer_name, message_id)`, unique in `inbox_messages`. If the same
message is redelivered (SQS's at-least-once guarantee means it will be,
eventually), the second delivery is recognized by that pair, its payload
hash is compared against what was stored the first time, and the original
result is returned without touching the wallet again. A hash mismatch for
the same `(consumer_name, message_id)` — the same message id claiming
different content — is treated as `ErrInboxHashConflict`, not silently
reprocessed, since a legitimate redelivery can never change the payload.

**Business-level.** Independent of transport, every transaction is also
identified by `(provider_id, idempotency_key)`, unique in
`wager_transactions`. This is what makes the *same logical operation*
submitted once over HTTP and once over SQS collapse into one effect, and
it's also the layer providers rely on when they retry a request they
didn't get a response for. `computeWagerHash` builds one canonical hash
from the command's fields regardless of which transport it arrived
through — the HTTP handler and the SQS handler produce identical hashes
for identical content, so the two dedup layers agree.

Both checks happen inside the same Postgres transaction as the financial
work itself (see the 12-step flow below) — there's no window where a
duplicate could slip through between the dedup check and the commit.

## Pending-reference resolution

A `REFUND` or `ROLLBACK` can legitimately arrive before the transaction it
references — providers don't guarantee ordering, and a reversal message
can simply be faster than the original. Rather than rejecting it or
blocking the caller until the reference shows up, `ProcessWagerService`
accepts it with status `PENDING_REFERENCE` and records an attempt count
and a `next_attempt_at`.

`PendingReferenceWorker` (`internal/adapter/worker/`) polls Postgres every
5 seconds for transactions in that state whose `next_attempt_at` is due,
and calls `ProcessWagerService.ResolvePendingReference` for each. Every
failed attempt backs off exponentially (base 10s, capped at 5 minutes);
after `maxPendingReferenceAttempts` (10) the transaction is rejected
outright rather than retried forever. The worker touches only Postgres and
the service — never SQS or HTTP directly — so it's a pure domain-level
retry loop, independent of whatever transport originally delivered the
reversal.

## Transactional outbox

Wallet events are never published from inside the request/message handling
transaction — `ProcessWagerService` only writes event rows to
`outbox_events` as part of the same commit that updates the wallet and the
ledger. A separate `OutboxPublisher` (`internal/adapter/sqs/worker/`)
polls that table independently, claiming a batch with
`UPDATE ... FOR UPDATE SKIP LOCKED` under a `(locked_by, locked_until)`
lease so that multiple publisher instances (one per app replica) can claim
disjoint batches without contending. A claimed event that fails to publish
is released for retry with exponential backoff and jitter; one that
publishes successfully is marked `published_at` in its own small
transaction. If the process crashes between a successful `SendMessage` and
that mark, the lease simply expires and the event gets reclaimed and sent
again — delivery is at-least-once, with `eventId` as
`MessageDeduplicationId` so the consumer on the other end can collapse
that duplicate. `outbox_pending_events` / `outbox_pending_oldest_age_seconds`
(exposed on `/metrics`) are exactly this backlog, sampled every publisher
tick.

## The 12-step transactional flow

Every call to `ProcessWagerService.Execute` runs these steps inside one
Postgres transaction (`UnitOfWork.WithinTransaction`):

1. If the source is SQS, check the inbox for `(consumer_name, message_id)`.
2. Look up the operation by `(provider_id, idempotency_key)` and by
   `(provider_id, external_transaction_id)`.
3. On a replay, compare the stored hash and return the persisted result
   without touching the wallet.
4. Lock the wallet with `SELECT ... FOR UPDATE`.
5. For `REFUND`/`ROLLBACK`, resolve and lock the referenced transaction —
   or mark the new transaction `PENDING_REFERENCE` if it isn't found yet.
6. Apply the domain rule for the transaction's kind (`applyWagerKind`).
7. Persist the transaction's new state.
8. Update the wallet's balance and version, if the operation moved money.
9. Insert the immutable ledger row, if the operation moved money.
10. Insert the outbox event rows for whatever happened.
11. Mark the inbox entry completed, if this came from SQS.
12. Commit.

No event is ever published inside this transaction — the outbox publisher
only reads committed rows, after the fact.

## Authentication and authorization

Keycloak issues OAuth2 `client_credentials` tokens, verified with
`golang-jwt/jwt/v5` against JWKS fetched via `MicahParks/keyfunc/v3`
(`internal/infrastructure/auth/keycloak.go`). Three clients exist:
`provider-a` and `provider-b` (scopes `wagering.read`, `wagering.write`,
each with its own `provider_id` claim), and `wallet-service` (scopes
`wagering.read`, `wallets.manage`, no `provider_id` — the internal caller
that manages wallets on providers' behalf). Every wagering request's body
`providerId` is checked against the token's `provider_id` claim — a
provider can submit or read only its own transactions, never another
provider's, regardless of what IDs appear in the request body.

Keycloak's `KC_HOSTNAME` is pinned to `http://keycloak:8080` in
`compose.yaml` so that the `iss` claim embedded in every token is stable
regardless of which host:port a client used to reach Keycloak (the host's
`localhost:8081` vs. the Docker network's `keycloak:8080`) — without it,
Keycloak's default behavior derives the issuer from the request's `Host`
header, which would never match the app's fixed `OIDC_ISSUER`.

## Observability

Structured JSON logs (`slog`) throughout, including Fx's own lifecycle
logging. `GET /metrics` exposes Prometheus counters/histograms/gauges for
wager outcomes, replay/conflict rates, reconciliation divergences, outbox
backlog, and SQS processing results — the full list and what each one
means is in the README. `GET /health/live` and `GET /health/ready` are
separate: liveness is unconditional, readiness runs each dependency check
(Postgres, SQS) with its *own* timeout budget so a slow or down dependency
can't starve the check for a healthy one in the same request.

## Testing strategy

- **Domain** (`internal/domain`): pure unit tests of `Money` parsing/
  arithmetic edge cases, `Wallet`/`WagerTransaction` invariants and state
  transitions, with no I/O.
- **Application services** (`internal/application/service`): tests run
  against a real Postgres instance (via the Unit of Work), not a mock —
  the invariants that matter here (locking, constraint violations,
  idempotency) only mean something against the real schema. This includes
  concurrency tests (many goroutines racing on one wallet, and racing
  across independent wallets) and schema-constraint tests that assert the
  database itself rejects what it should (negative balance, duplicate
  reference, ledger mutation).
- **Infrastructure** (`internal/infrastructure/auth`): tests fetch real
  tokens from a running Keycloak and verify acceptance/rejection,
  including a token forged with an untrusted key.
- **Multi-instance** (`make scale-up`): the one property that can't be
  exercised within a single process — three app replicas sharing
  Postgres/SQS/Keycloak, proving the consumer, outbox publisher, and
  pending-reference worker are safe under real cross-process concurrency.
- **Manual failure simulations**: Postgres/LocalStack pause-unpause,
  `SIGKILL`, and graceful `SIGTERM` shutdown — documented with exact
  commands in the README, since they're about operational behavior under
  real Docker lifecycle events, not something a unit test can represent.
- **Load** (`make loadtest`, `cmd/loadtest`): a standalone HTTP client that
  drives sustained concurrent `BET` traffic across several wallets and
  reports throughput and latency percentiles — a correctness-oriented
  concurrency test proves nothing double-applies; this answers the
  separate question of what the system does under sustained load.

## Known limitations / deliberately out of scope

- **`external_transaction_id` race under concurrent writers.** The unique
  index catches a duplicate `(provider_id, external_transaction_id)`, but
  two concurrent requests with the *same* external id and *different*
  idempotency keys would both pass the idempotency lookup before either
  commits, and one would fail on the unique constraint rather than being
  recognized as a dedup hit. This is a narrow race (requires a second
  request with a different idempotency key reusing an external id within
  the same transaction window) and isn't instrumented as a dedicated
  metric — a conflict here surfaces as a database constraint error, not as
  `wager_conflicts_total`.
- **No search by `external_transaction_id`.** Transactions are looked up
  by their own id; there's no `GET` by external id, since nothing in the
  current API needs one.
- **No OpenTelemetry tracing or dashboards.** Logs and Prometheus metrics
  cover the observability needs of this system's current scope; adding
  distributed tracing or pre-built dashboards was treated as a
  nice-to-have outside the core deliverable, not as skipped work.
