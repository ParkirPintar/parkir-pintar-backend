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

CREATE INDEX idx_reservations_driver_id ON reservations(driver_id);
CREATE INDEX idx_reservations_spot_id ON reservations(spot_id);
CREATE INDEX idx_reservations_status ON reservations(status);
CREATE INDEX idx_reservations_expires_at_reserved ON reservations(expires_at) WHERE status = 'RESERVED';

CREATE TABLE spots (
    spot_id      VARCHAR(20) PRIMARY KEY,
    floor        INT NOT NULL CHECK (floor BETWEEN 1 AND 5),
    vehicle_type VARCHAR(20) NOT NULL CHECK (vehicle_type IN ('CAR', 'MOTORCYCLE')),
    status       VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE'
                 CHECK (status IN ('AVAILABLE', 'LOCKED', 'RESERVED', 'OCCUPIED'))
);

CREATE INDEX idx_spots_floor_type_status ON spots(floor, vehicle_type, status);
CREATE INDEX idx_spots_type_status ON spots(vehicle_type, status);

-- Seed 150 car spots (5 floors x 30)
INSERT INTO spots (spot_id, floor, vehicle_type)
SELECT f.floor || '-CAR-' || LPAD(n.num::TEXT, 2, '0'), f.floor, 'CAR'
FROM generate_series(1, 5) AS f(floor), generate_series(1, 30) AS n(num);

-- Seed 250 motorcycle spots (5 floors x 50)
INSERT INTO spots (spot_id, floor, vehicle_type)
SELECT f.floor || '-MOTO-' || LPAD(n.num::TEXT, 2, '0'), f.floor, 'MOTORCYCLE'
FROM generate_series(1, 5) AS f(floor), generate_series(1, 50) AS n(num);
