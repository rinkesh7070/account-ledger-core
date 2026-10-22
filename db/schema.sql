-- ==============================================================================
-- ACCOUNT LEDGER CORE — PRODUCTION MYSQL 8.0 SCHEMA
-- Storage Engine: InnoDB | Character Set: utf8mb4 | Collation: utf8mb4_0900_ai_ci
-- ==============================================================================

-- 1. ACCOUNTS TABLE
-- Tracks account metadata, currency denomination, and optimistic lock version.
CREATE TABLE IF NOT EXISTS accounts (
    account_id       VARCHAR(64)     NOT NULL,
    currency         VARCHAR(3)      NOT NULL,      -- ISO 4217 (e.g. 'AED', 'BHD')
    currency_scale   TINYINT UNSIGNED NOT NULL,     -- Minor unit scale factor (2 for AED, 3 for BHD)
    status           VARCHAR(20)     NOT NULL DEFAULT 'ACTIVE', -- 'ACTIVE', 'FROZEN', 'CLOSED'
    version          BIGINT UNSIGNED NOT NULL DEFAULT 1,        -- Optimistic concurrency control version
    created_at       TIMESTAMP(6)    NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at       TIMESTAMP(6)    NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (account_id),
    CHECK (currency IN ('AED', 'BHD'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 2. LEDGER ENTRIES TABLE (APPEND-ONLY FINANCIAL EVENT LOG)
-- Fundamental financial record. Zero in-place mutations or deletions are permitted.
-- All monetary amounts are stored in exact integer minor units (fils) as BIGINT.
CREATE TABLE IF NOT EXISTS ledger_entries (
    entry_id         VARCHAR(64)      NOT NULL,
    account_id       VARCHAR(64)      NOT NULL,
    sequence_num     BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT, -- Global monotonic sequence
    account_seq      BIGINT UNSIGNED  NOT NULL,                -- Per-account monotonic sequence
    entry_type       VARCHAR(32)      NOT NULL,                -- 'CREDIT', 'DEBIT', 'SETTLEMENT', 'REVERSAL', 'OVERDRAFT_FEE', 'INTEREST_CAPITALIZATION'
    amount_minor     BIGINT           NOT NULL,                -- Signed/Unsigned minor units (fils/cents)
    currency         VARCHAR(3)       NOT NULL,
    booking_day      SMALLINT UNSIGNED NOT NULL,               -- Simulation/calendar day when booked
    value_day        SMALLINT UNSIGNED NOT NULL,               -- Value date (when economically effective)
    auth_id          VARCHAR(64)      NULL,                    -- Reference to authorization hold if applicable
    reference_id     VARCHAR(64)      NULL,                    -- Target entry_id if reversal
    idempotency_key  VARCHAR(128)     NULL,                    -- Exactly-once delivery token
    description      TEXT             NOT NULL,
    created_at       TIMESTAMP(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (sequence_num),
    UNIQUE KEY uq_entry_id (entry_id),
    UNIQUE KEY uq_account_seq (account_id, account_seq),
    UNIQUE KEY uq_idempotency (idempotency_key),
    KEY idx_account_value_day (account_id, value_day),
    KEY idx_auth_id (auth_id),
    KEY idx_reference_id (reference_id),
    CONSTRAINT fk_ledger_account FOREIGN KEY (account_id) REFERENCES accounts(account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 3. AUTHORIZATION HOLDS TABLE
-- Manages real-time reservations against available balance.
CREATE TABLE IF NOT EXISTS authorization_holds (
    auth_id           VARCHAR(64)      NOT NULL,
    account_id        VARCHAR(64)      NOT NULL,
    hold_amount_minor BIGINT UNSIGNED  NOT NULL,
    currency          VARCHAR(3)       NOT NULL,
    requested_day     SMALLINT UNSIGNED NOT NULL,
    value_day         SMALLINT UNSIGNED NOT NULL,
    status            VARCHAR(20)      NOT NULL, -- 'PENDING', 'APPROVED', 'DECLINED', 'SETTLED', 'EXPIRED'
    settled_amount_minor BIGINT UNSIGNED NULL,
    settled_day       SMALLINT UNSIGNED NULL,
    decline_reason    VARCHAR(255)     NULL,
    expires_at        TIMESTAMP(6)     NULL,
    created_at        TIMESTAMP(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at        TIMESTAMP(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (auth_id),
    KEY idx_active_holds (account_id, status),
    CONSTRAINT fk_auth_account FOREIGN KEY (account_id) REFERENCES accounts(account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 4. DAILY CLOSING SNAPSHOTS TABLE
-- Fast read-model caching daily closing balances, interest accruals, and fee counts.
CREATE TABLE IF NOT EXISTS daily_closing_snapshots (
    account_id               VARCHAR(64)      NOT NULL,
    day                      SMALLINT UNSIGNED NOT NULL,
    closing_ledger_balance   BIGINT           NOT NULL, -- In minor units
    available_balance        BIGINT           NOT NULL, -- In minor units
    interest_accrued_minor   BIGINT           NOT NULL DEFAULT 0,
    interest_capitalized_minor BIGINT         NOT NULL DEFAULT 0,
    overdraft_fee_assessed   BOOLEAN          NOT NULL DEFAULT FALSE,
    closed_at                TIMESTAMP(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (account_id, day),
    CONSTRAINT fk_snapshot_account FOREIGN KEY (account_id) REFERENCES accounts(account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 5. TRANSACTIONAL OUTBOX TABLE
-- For CDC (Change Data Capture) or Kafka event streaming without distributed 2PC.
CREATE TABLE IF NOT EXISTS outbox_events (
    outbox_id        BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
    aggregate_type   VARCHAR(32)      NOT NULL DEFAULT 'ACCOUNT',
    aggregate_id     VARCHAR(64)      NOT NULL,
    event_type       VARCHAR(64)      NOT NULL,
    payload_json     JSON             NOT NULL,
    published        BOOLEAN          NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMP(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    published_at     TIMESTAMP(6)     NULL,
    PRIMARY KEY (outbox_id),
    KEY idx_unpublished (published, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
