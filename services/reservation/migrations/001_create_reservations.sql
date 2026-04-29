-- 001_create_reservations.sql
-- Reservation Service database migration
-- Creates reservations and spots tables with indexes and seed data
-- Validates: Requirements 22.1, 22.2, 22.3

-- =============================================================================
-- Table: reservations
-- =============================================================================
CREATE TABLE reservations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id       UUID NOT NULL,
    spot_id         VARCHAR(20) NOT NULL,
    mode            VARCHAR(20) NOT NULL CHECK (mode IN ('SYSTEM_ASSIGNED', 'USER_SELECTED')),
    status          VARCHAR(20) NOT NULL DEFAULT 'RESERVED'
                    CHECK (status IN ('RESERVED', 'ACTIVE', 'COMPLETED', 'CANCELLED', 'EXPIRED')),
    booking_fee     BIGINT NOT NULL DEFAULT 5000,
    confirmed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    checkin_at      TIMESTAMPTZ,
    idempotency_key VARCHAR(100) UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- FK-like index: lookup reservations by driver
CREATE INDEX idx_reservations_driver_id ON reservations(driver_id);

-- Lookup reservations by spot (double-book check)
CREATE INDEX idx_reservations_spot_id ON reservations(spot_id);

-- Filter by status for expiry worker and queries
CREATE INDEX idx_reservations_status ON reservations(status);

-- Partial index for expiry worker scan: only RESERVED reservations
CREATE INDEX idx_reservations_expires_at_reserved ON reservations(expires_at)
    WHERE status = 'RESERVED';

-- Idempotency dedup (UNIQUE constraint on column already creates an index,
-- but we name it explicitly for clarity)
-- Note: The UNIQUE constraint on idempotency_key already creates a unique index.

-- =============================================================================
-- Table: spots
-- =============================================================================
CREATE TABLE spots (
    spot_id           VARCHAR(20) PRIMARY KEY,
    floor             INT NOT NULL CHECK (floor BETWEEN 1 AND 5),
    vehicle_type      VARCHAR(20) NOT NULL CHECK (vehicle_type IN ('CAR', 'MOTORCYCLE')),
    status            VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE'
                      CHECK (status IN ('AVAILABLE', 'LOCKED', 'RESERVED', 'OCCUPIED'))
);

-- Composite index for availability queries (floor + vehicle_type + status)
CREATE INDEX idx_spots_floor_type_status ON spots(floor, vehicle_type, status);

-- Index for GetFirstAvailable query (vehicle_type + status)
CREATE INDEX idx_spots_type_status ON spots(vehicle_type, status);

-- =============================================================================
-- Seed data: 150 car spots (5 floors × 30 per floor)
-- Format: {FLOOR}-CAR-{NUMBER} (zero-padded 01–30)
-- =============================================================================
INSERT INTO spots (spot_id, floor, vehicle_type)
SELECT
    f.floor || '-CAR-' || LPAD(n.num::TEXT, 2, '0'),
    f.floor,
    'CAR'
FROM generate_series(1, 5) AS f(floor),
     generate_series(1, 30) AS n(num);

-- =============================================================================
-- Seed data: 250 motorcycle spots (5 floors × 50 per floor)
-- Format: {FLOOR}-MOTO-{NUMBER} (zero-padded 01–50)
-- =============================================================================
INSERT INTO spots (spot_id, floor, vehicle_type)
SELECT
    f.floor || '-MOTO-' || LPAD(n.num::TEXT, 2, '0'),
    f.floor,
    'MOTORCYCLE'
FROM generate_series(1, 5) AS f(floor),
     generate_series(1, 50) AS n(num);
