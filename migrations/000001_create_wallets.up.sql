CREATE TABLE wallets (
    id UUID PRIMARY KEY,
    player_id TEXT NOT NULL,
    currency TEXT NOT NULL,
    balance_minor BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT wallets_player_currency_unique UNIQUE (player_id, currency),
    CONSTRAINT wallets_balance_non_negative CHECK (balance_minor >= 0),
    CONSTRAINT wallets_version_positive CHECK (version >= 1)
);
