-- 001_init.up.sql
-- GopherWallet: High-Concurrency Transaction Engine

CREATE TABLE IF NOT EXISTS accounts (
    id          BIGSERIAL PRIMARY KEY,
    user_id     VARCHAR(64) NOT NULL,
    balance     BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    currency    VARCHAR(3) NOT NULL DEFAULT 'THB',
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_user_id ON accounts(user_id);

CREATE TABLE IF NOT EXISTS transactions (
    id              BIGSERIAL PRIMARY KEY,
    from_account_id BIGINT NOT NULL REFERENCES accounts(id),
    to_account_id   BIGINT NOT NULL REFERENCES accounts(id),
    amount          BIGINT NOT NULL CHECK (amount > 0),
    currency        VARCHAR(3) NOT NULL DEFAULT 'THB',
    status          VARCHAR(10) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    idempotency_key VARCHAR(128) UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_from_account ON transactions(from_account_id);
CREATE INDEX idx_transactions_to_account ON transactions(to_account_id);
CREATE INDEX idx_transactions_idempotency ON transactions(idempotency_key);
