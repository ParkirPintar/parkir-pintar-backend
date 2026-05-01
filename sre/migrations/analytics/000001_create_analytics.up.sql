CREATE TABLE transaction_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type     VARCHAR(50) NOT NULL,
    reservation_id UUID,
    driver_id      UUID,
    spot_id        VARCHAR(20),
    vehicle_type   VARCHAR(20),
    amount         BIGINT DEFAULT 0,
    payload        JSONB NOT NULL,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_type ON transaction_events(event_type);
CREATE INDEX idx_events_reservation ON transaction_events(reservation_id);
CREATE INDEX idx_events_driver ON transaction_events(driver_id);
CREATE INDEX idx_events_recorded_at ON transaction_events(recorded_at);
CREATE INDEX idx_events_type_recorded_at ON transaction_events(event_type, recorded_at);
