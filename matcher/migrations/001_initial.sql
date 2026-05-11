-- 4.1. ENUM-типы
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'overall_status') THEN
    CREATE TYPE overall_status AS ENUM ('matched', 'mismatch', 'degraded', 'pending');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'incident_status') THEN
    CREATE TYPE incident_status AS ENUM ('open', 'acknowledged', 'resolved', 'auto_corrected');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'source_kind') THEN
    CREATE TYPE source_kind AS ENUM ('merchant', 'gateway', 'bank');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'incident_kind') THEN
    CREATE TYPE incident_kind AS ENUM (
      'amount_mismatch',
      'fee_mismatch',
      'currency_mismatch',
      'missing_source',
      'duplicate',
      'source_offline'
    );
  END IF;
END$$;

-- 4.2. Таблица входящих событий (для аудита и identity resolution)
CREATE TABLE IF NOT EXISTS events (
    internal_event_id UUID PRIMARY KEY,         -- UUID v7
    event_id          UUID NOT NULL,             -- ID от источника
    transaction_id    UUID NOT NULL,             -- сквозной ID (в MVP = correlation_id)
    correlation_id    UUID NOT NULL,             -- внутренний группирующий ID
    source            source_kind NOT NULL,
    amount_minor      BIGINT NOT NULL,
    fee_minor         BIGINT NOT NULL,
    currency          CHAR(3) NOT NULL,
    timestamp_ms      BIGINT NOT NULL,
    tx_type           VARCHAR(20) NOT NULL,
    merchant_id       VARCHAR(100),
    raw_payload       JSONB NOT NULL,
    received_at_ms    BIGINT NOT NULL,
    UNIQUE (event_id, source)                    -- дедупликация на уровне БД
);

CREATE INDEX IF NOT EXISTS idx_events_transaction ON events(transaction_id);
CREATE INDEX IF NOT EXISTS idx_events_correlation ON events(correlation_id);
CREATE INDEX IF NOT EXISTS idx_events_received ON events(received_at_ms DESC);

-- 4.3. Таблица транзакций (агрегаты от Reconciler)
CREATE TABLE IF NOT EXISTS transactions (
    transaction_id UUID PRIMARY KEY,
    overall_status overall_status NOT NULL,
    merchant_id    VARCHAR(100),
    tx_type        VARCHAR(20) NOT NULL DEFAULT 'card',
    currency       CHAR(3) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_transactions_status_created
    ON transactions(overall_status, created_at DESC);

-- 4.4. Детали сверки по источникам
CREATE TABLE IF NOT EXISTS reconciliation_details (
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

-- 4.5. Инциденты
CREATE TABLE IF NOT EXISTS incidents (
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

CREATE INDEX IF NOT EXISTS idx_incidents_status_created ON incidents(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_transaction ON incidents(transaction_id);
