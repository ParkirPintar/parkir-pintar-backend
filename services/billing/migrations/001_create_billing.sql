-- 001_create_billing.sql
-- Billing Service database migration
-- Creates billing_records and pricing_rules tables with indexes and seed data
-- Validates: Requirements 11.4, 12.8

-- =============================================================================
-- Table: billing_records
-- Tracks all fee components for a parking session (booking, hourly, overnight,
-- penalty, no-show, cancellation) and links to payment via payment_id/qr_code.
-- =============================================================================
CREATE TABLE billing_records (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id   UUID NOT NULL,
    booking_fee      BIGINT NOT NULL DEFAULT 5000,
    hourly_fee       BIGINT NOT NULL DEFAULT 0,
    overnight_fee    BIGINT NOT NULL DEFAULT 0,
    penalty          BIGINT NOT NULL DEFAULT 0,
    noshow_fee       BIGINT NOT NULL DEFAULT 0,
    cancellation_fee BIGINT NOT NULL DEFAULT 0,
    total            BIGINT NOT NULL DEFAULT 0,
    status           VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    session_start    TIMESTAMPTZ,
    session_end      TIMESTAMPTZ,
    idempotency_key  VARCHAR(100) UNIQUE,
    payment_id       UUID,
    qr_code          TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- FK-like index: lookup billing records by reservation
CREATE INDEX idx_billing_reservation_id ON billing_records(reservation_id);

-- Checkout idempotency dedup: the UNIQUE constraint on idempotency_key already
-- creates an implicit unique index. No additional index needed.

-- Filter billing records by status (PENDING/PAID/FAILED)
CREATE INDEX idx_billing_status ON billing_records(status);

-- Lookup billing records by payment reference
CREATE INDEX idx_billing_payment_id ON billing_records(payment_id);

-- =============================================================================
-- Table: pricing_rules
-- Stores versioned pricing rule configurations as JSONB. The billing service
-- polls this table every 30 seconds and hot-reloads when the version changes.
-- =============================================================================
CREATE TABLE pricing_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version    INT NOT NULL,
    name       VARCHAR(100) NOT NULL,
    content    JSONB NOT NULL,
    is_active  BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Composite index for hot-reload query: find the active rule with the latest version
CREATE INDEX idx_pricing_rules_active_version ON pricing_rules(is_active, version DESC);

-- =============================================================================
-- Seed data: Default pricing rule (version 1)
-- ParkirPintar standard pricing parameters as JDM (JSON Decision Model)
-- The full JDM graph is stored in the content column and loaded by the
-- billing service's hot-reload mechanism via gorules/zen-go engine.
-- For the complete JDM file, see: services/billing/rules/pricing.json
-- =============================================================================
INSERT INTO pricing_rules (version, name, content)
VALUES (
    1,
    'ParkirPintar Default Pricing (JDM)',
    '{
        "description": "ParkirPintar pricing rules — JDM format for gorules/zen-go engine",
        "parameters": {
            "booking_fee": 5000,
            "hourly_rate": 5000,
            "overnight_fee_per_crossing": 20000,
            "noshow_fee": 10000,
            "cancel_fee_after_2min": 5000,
            "wrong_spot": "BLOCKED"
        },
        "jdm_file": "pricing.json",
        "engine": "gorules/zen-go"
    }'::jsonb
);
