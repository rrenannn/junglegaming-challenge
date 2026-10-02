CREATE TABLE wager_transactions (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets (id),
    provider_id TEXT,
    player_id TEXT NOT NULL,
    round_id TEXT,
    game_id TEXT,
    kind TEXT NOT NULL CHECK (kind IN ('OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK')),
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'PENDING_REFERENCE', 'PROCESSED', 'REJECTED', 'FAILED')),
    direction TEXT NOT NULL DEFAULT 'NONE' CHECK (direction IN ('NONE', 'DEBIT', 'CREDIT')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency TEXT NOT NULL,
    balance_after_minor BIGINT,
    external_transaction_id TEXT,
    idempotency_key TEXT,
    payload_hash TEXT,
    reference_transaction_id UUID REFERENCES wager_transactions (id),
    reversed_by_transaction_id UUID REFERENCES wager_transactions (id),
    failure_code TEXT,
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT wager_transactions_opening_metadata CHECK (
        (kind = 'OPENING' AND provider_id IS NULL AND round_id IS NULL AND game_id IS NULL AND external_transaction_id IS NULL)
        OR
        (kind <> 'OPENING' AND provider_id IS NOT NULL AND round_id IS NOT NULL AND game_id IS NOT NULL)
    )
);

CREATE INDEX wager_transactions_wallet_idx ON wager_transactions (wallet_id);

CREATE UNIQUE INDEX wager_transactions_provider_idempotency_key_unique
    ON wager_transactions (provider_id, idempotency_key)
    WHERE provider_id IS NOT NULL AND idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX wager_transactions_provider_external_id_unique
    ON wager_transactions (provider_id, external_transaction_id)
    WHERE provider_id IS NOT NULL AND external_transaction_id IS NOT NULL;

CREATE UNIQUE INDEX wager_transactions_reference_processed_unique
    ON wager_transactions (reference_transaction_id)
    WHERE reference_transaction_id IS NOT NULL AND status = 'PROCESSED';

CREATE INDEX wager_transactions_pending_reference_idx
    ON wager_transactions (next_attempt_at)
    WHERE status = 'PENDING_REFERENCE';
