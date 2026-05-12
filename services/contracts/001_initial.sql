CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE source_kind AS ENUM ('merchant', 'gateway', 'bank');
CREATE TYPE overall_status AS ENUM ('matched', 'mismatch', 'degraded', 'pending');
CREATE TYPE incident_status AS ENUM ('open', 'acknowledged', 'resolved', 'auto_corrected');
CREATE TYPE incident_kind AS ENUM (
    'amount_mismatch',
    'fee_mismatch',
    'currency_mismatch',
    'missing_source',
    'duplicate',
    'source_offline'
);

CREATE TABLE events (
    internal_event_id UUID PRIMARY KEY,
    event_id          UUID NOT NULL,
    transaction_id    UUID NOT NULL,
    correlation_id    UUID NOT NULL,
    source            source_kind NOT NULL,
    amount_minor      BIGINT NOT NULL,
    fee_minor         BIGINT NOT NULL,
    currency          CHAR(3) NOT NULL,
    timestamp_ms      BIGINT NOT NULL,
    tx_type           VARCHAR(20) NOT NULL,
    merchant_id       VARCHAR(100),
    raw_payload       JSONB NOT NULL,
    received_at_ms    BIGINT NOT NULL,
    UNIQUE (event_id, source)
);

CREATE INDEX idx_events_transaction ON events(transaction_id);
CREATE INDEX idx_events_correlation ON events(correlation_id);
CREATE INDEX idx_events_received ON events(received_at_ms DESC);

CREATE TABLE transactions (
    transaction_id UUID PRIMARY KEY,
    overall_status overall_status NOT NULL,
    merchant_id    VARCHAR(100),
    tx_type        VARCHAR(20) NOT NULL DEFAULT 'card',
    currency       CHAR(3) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_status_created ON transactions(overall_status, created_at DESC);

CREATE TABLE reconciliation_details (
    transaction_id   UUID NOT NULL REFERENCES transactions(transaction_id) ON DELETE CASCADE,
    source           source_kind NOT NULL,
    amount_expected  BIGINT,
    amount_actual    BIGINT,
    fee_expected     BIGINT,
    fee_actual       BIGINT,
    is_matched       BOOLEAN NOT NULL,
    mismatch_reason  VARCHAR(40),
    received_at      TIMESTAMPTZ,
    PRIMARY KEY (transaction_id, source)
);

CREATE TABLE incidents (
    id              BIGSERIAL PRIMARY KEY,
    transaction_id  UUID NOT NULL REFERENCES transactions(transaction_id) ON DELETE CASCADE,
    incident_type   incident_kind NOT NULL,
    severity        SMALLINT NOT NULL DEFAULT 1,
    description     TEXT NOT NULL,
    status          incident_status NOT NULL DEFAULT 'open',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    escalated_at    TIMESTAMPTZ,
    auto_correction_proposed JSONB
);

CREATE INDEX idx_incidents_status_created ON incidents(status, created_at DESC);
CREATE INDEX idx_incidents_transaction ON incidents(transaction_id);