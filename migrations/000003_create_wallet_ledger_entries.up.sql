CREATE TABLE wallet_ledger_entries (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets (id),
    transaction_id UUID NOT NULL REFERENCES wager_transactions (id),
    direction TEXT NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL,
    balance_before_minor BIGINT NOT NULL,
    balance_after_minor BIGINT NOT NULL CHECK (balance_after_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT wallet_ledger_entries_wallet_transaction_unique UNIQUE (wallet_id, transaction_id),
    CONSTRAINT wallet_ledger_entries_equation CHECK (
        (direction = 'DEBIT' AND balance_after_minor = balance_before_minor - amount_minor)
        OR
        (direction = 'CREDIT' AND balance_after_minor = balance_before_minor + amount_minor)
    )
);

CREATE INDEX wallet_ledger_entries_wallet_created_idx ON wallet_ledger_entries (wallet_id, created_at, id);

CREATE FUNCTION wallet_ledger_entries_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'wallet_ledger_entries rows are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER wallet_ledger_entries_no_update
    BEFORE UPDATE ON wallet_ledger_entries
    FOR EACH ROW EXECUTE FUNCTION wallet_ledger_entries_immutable();

CREATE TRIGGER wallet_ledger_entries_no_delete
    BEFORE DELETE ON wallet_ledger_entries
    FOR EACH ROW EXECUTE FUNCTION wallet_ledger_entries_immutable();
