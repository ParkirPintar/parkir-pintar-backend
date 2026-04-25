-- init-db.sql — Create per-service databases for local development
-- Executed automatically by postgres container on first startup

CREATE DATABASE user_db;
CREATE DATABASE reservation_db;
CREATE DATABASE billing_db;
CREATE DATABASE payment_db;
CREATE DATABASE analytics_db;
