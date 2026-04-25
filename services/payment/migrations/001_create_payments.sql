-- 001_create_payments.sql
-- Payment Service database migration
-- Creates payments table with indexes for QRIS payment processing
-- Validates: Requirements 14.1

-- =============================================================================
-- Table: payments
-- Tracks QRIS payment records linked to billing invoices. Each payment has a
-- status lifecycle: PENDING → PAID or PENDING → FAILED (with retry creating a
-- new record). The idempotency_key prevents duplicate payment creation from
-- network retries.
-- =============================================================================
CREATE TABLE payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id      UUID NOT NULL,
    amount          BIGINT NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING', 'PAID', 'FAILED')),
    method          VARCHAR(20) NOT NULL DEFAULT 'QRIS',
    qr_code         TEXT,
    idempotency_key VARCHAR(100),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- FK-like index: lookup payments by invoice
CREATE INDEX idx_payments_invoice_id ON payments(invoice_id);

-- Payment idempotency dedup: lookup by idempotency key to prevent duplicates
CREATE INDEX idx_payments_idempotency_key ON payments(idempotency_key);

-- Filter PENDING payments for settlement polling
CREATE INDEX idx_payments_status ON payments(status);

-- Sorting and pagination by creation time
CREATE INDEX idx_payments_created_at ON payments(created_at);
