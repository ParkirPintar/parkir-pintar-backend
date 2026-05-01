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

CREATE INDEX idx_billing_reservation_id ON billing_records(reservation_id);
CREATE INDEX idx_billing_status ON billing_records(status);
CREATE INDEX idx_billing_payment_id ON billing_records(payment_id);

CREATE TABLE pricing_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version    INT NOT NULL,
    name       VARCHAR(100) NOT NULL,
    content    JSONB NOT NULL,
    is_active  BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pricing_rules_active_version ON pricing_rules(is_active, version DESC);

INSERT INTO pricing_rules (version, name, content)
VALUES (
    1,
    'ParkirPintar Default Pricing (JDM)',
    '{"description":"ParkirPintar pricing rules","parameters":{"booking_fee":5000,"hourly_rate":5000,"overnight_fee_per_crossing":20000,"noshow_fee":10000,"cancel_fee_after_2min":5000,"wrong_spot":"BLOCKED"},"jdm_file":"pricing.json","engine":"gorules/zen-go"}'::jsonb
);
