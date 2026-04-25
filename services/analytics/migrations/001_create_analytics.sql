-- 001_create_analytics.sql
-- Analytics Service database migration
-- Creates transaction_events table with indexes for efficient querying
-- Validates: Requirements 17.1

-- =============================================================================
-- Table: transaction_events
-- Stores all transaction events consumed from RabbitMQ for business monitoring.
-- Each row represents a domain event (reservation.confirmed, checkin.confirmed,
-- penalty.applied, reservation.cancelled, reservation.expired, checkout.completed,
-- checkout.failed) with the full event payload stored as JSONB.
-- =============================================================================
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

-- Filter by event type for analytics queries (e.g., all checkout.completed events)
CREATE INDEX idx_events_type ON transaction_events(event_type);

-- Lookup events by reservation (e.g., full lifecycle of a single reservation)
CREATE INDEX idx_events_reservation ON transaction_events(reservation_id);

-- Lookup events by driver (e.g., driver activity history)
CREATE INDEX idx_events_driver ON transaction_events(driver_id);

-- Time-range queries and sorting (e.g., events in the last 24 hours)
CREATE INDEX idx_events_recorded_at ON transaction_events(recorded_at);

-- Composite index for time-filtered event type queries
-- (e.g., all checkout.completed events in the last week)
CREATE INDEX idx_events_type_recorded_at ON transaction_events(event_type, recorded_at);
