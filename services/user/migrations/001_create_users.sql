-- 001_create_users.sql
-- User Service: Create users table for driver registration and authentication.
-- Requirements: 1.1 (Driver Registration), 1.2 (Duplicate Prevention via UNIQUE constraint)

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    license_plate VARCHAR(20) NOT NULL,
    vehicle_type  VARCHAR(20) NOT NULL CHECK (vehicle_type IN ('CAR', 'MOTORCYCLE')),
    password_hash VARCHAR(255) NOT NULL,
    name          VARCHAR(100) NOT NULL DEFAULT '',
    phone_number  VARCHAR(20) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Composite unique constraint: one account per license plate + vehicle type.
    -- Also serves as a composite index for login lookups.
    UNIQUE(license_plate, vehicle_type)
);

-- Index on license_plate for frequent lookups during login and registration.
CREATE INDEX IF NOT EXISTS idx_users_license_plate ON users (license_plate);

-- Index on created_at for sorting and pagination queries.
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users (created_at);
