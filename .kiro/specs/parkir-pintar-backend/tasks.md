# Implementation Plan: ParkirPintar Backend

## Overview

This plan implements all 8 Go microservices for the ParkirPintar smart parking backend. Each service already has skeleton code with handler structs, usecase interfaces, repository implementations, and domain models. The work involves filling in TODOs: database migrations, password hashing, JWT auth, RabbitMQ integration, Redis caching, gRPC client adapters, settlement stub HTTP client, auth interceptor, expiry worker, pricing engine, and comprehensive tests. Tasks are ordered so each builds on the previous, ending with full integration wiring.

## Tasks

- [ ] 1. Database migrations for all services
  - [x] 1.1 Create User Service migration (`services/user/migrations/001_create_users.sql`)
    - CREATE TABLE users with id (UUID PK), license_plate, vehicle_type, password_hash, name, phone_number, created_at, updated_at
    - Add UNIQUE(license_plate, vehicle_type) constraint (also serves as composite index for login lookups)
    - Add CHECK constraint on vehicle_type IN ('CAR', 'MOTORCYCLE')
    - Add INDEX on `license_plate` (frequent lookup by plate on login/register)
    - Add INDEX on `created_at` (sorting/pagination)
    - _Requirements: 1.1, 1.2_

  - [x] 1.2 Create Reservation Service migration (`services/reservation/migrations/001_create_reservations.sql`)
    - CREATE TABLE reservations with id, driver_id, spot_id, mode, status, booking_fee, confirmed_at, expires_at, checkin_at, idempotency_key, created_at
    - Add CHECK constraints on mode and status enums
    - Add INDEX on `driver_id` (FK-like, lookup reservations by driver)
    - Add INDEX on `spot_id` (lookup reservations by spot, double-book check)
    - Add INDEX on `status` (filter by RESERVED/ACTIVE/EXPIRED for expiry worker and queries)
    - Add INDEX on `expires_at` WHERE status='RESERVED' (partial index for expiry worker scan)
    - Add UNIQUE INDEX on `idempotency_key` (idempotency dedup)
    - CREATE TABLE spots with spot_id (PK), floor, vehicle_type, status, latitude, longitude, geofence_radius_m
    - Add INDEX on `(floor, vehicle_type, status)` (composite index for availability queries)
    - Add INDEX on `(vehicle_type, status)` (GetFirstAvailable query)
    - Seed 150 car spots (5 floors × 30) with format `{FLOOR}-CAR-{NUMBER}` (zero-padded 01–30)
    - Seed 250 motorcycle spots (5 floors × 50) with format `{FLOOR}-MOTO-{NUMBER}` (zero-padded 01–50)
    - All spots initial status=AVAILABLE
    - _Requirements: 22.1, 22.2, 22.3_

  - [x] 1.3 Create Billing Service migration (`services/billing/migrations/001_create_billing.sql`)
    - CREATE TABLE billing_records with id, reservation_id, booking_fee, hourly_fee, overnight_fee, penalty, noshow_fee, cancellation_fee, total, status, session_start, session_end, idempotency_key, payment_id, qr_code, created_at
    - Add INDEX on `reservation_id` (FK-like, lookup billing by reservation)
    - Add UNIQUE INDEX on `idempotency_key` (checkout idempotency dedup)
    - Add INDEX on `status` (filter PENDING/PAID/FAILED)
    - Add INDEX on `payment_id` (lookup by payment reference)
    - CREATE TABLE pricing_rules with id, version, name, content (JSONB), is_active, created_at
    - Add INDEX on `(is_active, version DESC)` (hot-reload query: active rule with latest version)
    - Seed a default pricing rule (version 1)
    - _Requirements: 11.4, 12.8_

  - [x] 1.4 Create Payment Service migration (`services/payment/migrations/001_create_payments.sql`)
    - CREATE TABLE payments with id, invoice_id, amount, status, method, qr_code, idempotency_key, created_at, updated_at
    - Add CHECK constraint on status IN ('PENDING', 'PAID', 'FAILED')
    - Add INDEX on `invoice_id` (FK-like, lookup payments by invoice)
    - Add INDEX on `idempotency_key` (payment idempotency dedup)
    - Add INDEX on `status` (filter PENDING payments for settlement polling)
    - Add INDEX on `created_at` (sorting/pagination)
    - _Requirements: 14.1_

  - [x] 1.5 Create Analytics Service migration (`services/analytics/migrations/001_create_analytics.sql`)
    - CREATE TABLE transaction_events with id, event_type, reservation_id, driver_id, spot_id, vehicle_type, amount, payload (JSONB), recorded_at
    - Add INDEX on `event_type` (filter by event type for analytics queries)
    - Add INDEX on `reservation_id` (lookup events by reservation)
    - Add INDEX on `driver_id` (lookup events by driver)
    - Add INDEX on `recorded_at` (time-range queries, sorting)
    - Add INDEX on `(event_type, recorded_at)` (composite for time-filtered event type queries)
    - _Requirements: 17.1_


  - [ ]* 1.6 Write property test for spot inventory initialization (Property 23)
    - **Property 23: Spot inventory initialization**
    - Verify exactly 150 CAR spots (30 per floor × 5 floors) and 250 MOTORCYCLE spots (50 per floor × 5 floors)
    - Verify all spots have status=AVAILABLE and spot_id matches `{FLOOR}-{TYPE}-{NUMBER}` format
    - Test file: `services/reservation/migrations/001_create_reservations_test.go`
    - **Validates: Requirements 22.1, 22.2, 22.3**

- [x] 2. Checkpoint — Verify migrations
  - Ensure all migration SQL files are syntactically correct and can be applied to a fresh database
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 3. User Service — Authentication and profile management
  - [x] 3.1 Add password_hash field to User model and update repository
    - Add `PasswordHash string` field to `services/user/internal/model/user.go`
    - Add `StoreRefreshToken`, `GetRefreshToken`, `DeleteRefreshToken` methods to `UserRepository` interface
    - Update `user_postgres.go`: store password_hash in Create, add refresh token methods (Redis `refresh:{token}` → driver_id, TTL 7d), update `SetTokenBlacklist` to accept dynamic TTL
    - _Requirements: 1.3, 2.1, 2.5_

  - [x] 3.2 Implement JWT helper (`services/user/internal/usecase/jwt.go`)
    - `GenerateAccessToken(driverID string) (string, error)` — HS256, 1h expiry, claims: sub=driver_id, jti=uuid
    - `ParseAccessToken(tokenStr string) (*Claims, error)` — validate signature, expiry, return claims
    - `GenerateRefreshToken() string` — opaque UUID
    - Use `golang-jwt/jwt/v5` library, signing key from `JWT_SECRET` env var
    - _Requirements: 2.1_

  - [ ]* 3.3 Write property test for JWT claims (Property 5)
    - **Property 5: JWT access token contains correct claims**
    - For any driver_id, generated token has sub=driver_id, jti is valid UUID, exp within [now+59min, now+61min]
    - Test file: `services/user/internal/usecase/jwt_test.go`
    - **Validates: Requirements 2.1**

  - [x] 3.4 Implement bcrypt password hashing and full auth logic in UserUsecase
    - Update `Register`: hash password with bcrypt cost 12, store password_hash
    - Update `Authenticate`: accept password param, compare with bcrypt, return JWT access token + refresh token
    - Add `Logout(ctx, accessToken)`: parse JWT, add jti to Redis blacklist with remaining TTL, delete refresh token
    - Add `RefreshToken(ctx, refreshToken)`: validate refresh token in Redis, issue new access + refresh tokens, delete old refresh token (rotation)
    - Add `ValidateToken(ctx, tokenStr)`: parse JWT, check blacklist, return driver_id
    - Add input validation: return error if license_plate or vehicle_type is empty
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 3.1, 3.2, 3.3_

  - [ ]* 3.5 Write property tests for User Service (Properties 1–4, 6–8)
    - **Property 1: Registration round-trip preserves driver data**
    - **Property 2: Duplicate registration is rejected**
    - **Property 3: Invalid registration input is rejected**
    - **Property 4: Password is stored as bcrypt with cost 12**
    - **Property 6: Refresh token rotation produces new tokens**
    - **Property 7: Logout blacklists token and invalidates refresh**
    - **Property 8: Profile update round-trip**
    - Test file: `services/user/internal/usecase/user_usecase_test.go`
    - Use mocked repository interface for unit-level property tests
    - **Validates: Requirements 1.1, 1.2, 1.3, 1.4, 2.1, 2.3, 2.5, 2.6, 3.1, 3.2**

  - [x] 3.6 Update User handler for Login, Logout, RefreshToken RPCs
    - Fix `Login` handler: accept password in request, call `Authenticate`, return real JWT access_token and refresh_token
    - Implement `Logout` handler: call `uc.Logout`, return success
    - Implement `RefreshToken` handler: call `uc.RefreshToken`, return new tokens
    - Fix `Register` handler: pass password to usecase, handle INVALID_ARGUMENT for missing fields
    - _Requirements: 1.1, 1.4, 2.1, 2.2, 2.3, 2.4, 2.5_

  - [x] 3.7 Implement gRPC auth interceptor (`services/user/pkg/interceptor/auth.go`)
    - Extract JWT from `authorization` gRPC metadata header
    - Validate JWT signature (HS256), expiry
    - Check blacklist via Redis (`blacklist:{jti}`)
    - Inject driver_id into gRPC context on success
    - Return UNAUTHENTICATED for missing, expired, invalid, or blacklisted tokens
    - Skip authentication for Register and Login RPCs
    - _Requirements: 23.1, 23.2, 23.3, 23.4, 23.5_

  - [ ]* 3.8 Write property test for auth interceptor (Property 22)
    - **Property 22: Auth interceptor allows valid tokens and rejects invalid ones**
    - For any valid non-blacklisted JWT → inject driver_id; for any missing/expired/invalid/blacklisted → UNAUTHENTICATED
    - Test file: `services/user/pkg/interceptor/auth_test.go`
    - **Validates: Requirements 23.1, 23.2, 23.3, 23.4, 23.5**


- [x] 4. Checkpoint — User Service complete
  - Ensure all User Service tests pass (unit + property tests)
  - Verify JWT issuance, bcrypt hashing, refresh rotation, logout/blacklist, auth interceptor
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 5. Search Service — Redis cache-aside implementation
  - [x] 5.1 Implement Redis cache-aside in Search repository
    - Update `services/search/internal/repository/spot_postgres.go`
    - `GetAvailableSpots`: check Redis key `availability:{floor}:{type}` (TTL 10s) first; on hit, deserialize and return; on miss, query PostgreSQL, serialize to Redis, return
    - `GetFirstAvailable`: similar cache-aside pattern with key `availability:{type}` (TTL 10s)
    - Use JSON serialization for cached spot lists
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6_

  - [ ]* 5.2 Write property test for availability query (Property 9)
    - **Property 9: Availability query returns only matching available spots**
    - For any floor (1–5) and vehicle_type, all returned spots have status=AVAILABLE, matching floor and type; total_available equals list length
    - Test file: `services/search/internal/usecase/search_usecase_test.go`
    - **Validates: Requirements 4.1, 4.5**

  - [x] 5.3 Wire Search Service main.go
    - Uncomment and wire pgxpool (read replica), Redis client, repo, usecase, handler
    - Register SearchServiceServer on gRPC server
    - Fix default port to `:50055`
    - Add auth interceptor (import from user service pkg)
    - _Requirements: 21.1, 21.3, 21.4_

- [ ] 6. Billing Service — Pricing engine and checkout
  - [x] 6.1 Extract and enhance pricing engine (`services/billing/internal/usecase/pricing.go`)
    - Move `evaluatePricing` function to dedicated file
    - Add gorules/JDM integration: `NewPricingEngine(ruleContent []byte)`, `Evaluate(PricingInput) PricingOutput`
    - Fallback to pure Go `evaluatePricing()` if gorules engine is unavailable
    - Fix `evaluatePricing`: separate NoshowFee from Penalty (currently both set Penalty field)
    - Add NoshowFee to PricingOutput struct
    - Ensure total = booking_fee + hourly_fee + overnight_fee + penalty + cancellation_fee + noshow_fee
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6, 12.7, 12.8, 12.9_

  - [ ]* 6.2 Write property tests for pricing engine (Properties 10–14)
    - **Property 10: Pricing engine determinism (round-trip)**
    - **Property 11: Pricing total is sum of components**
    - **Property 12: Hourly fee calculation**
    - **Property 13: Overnight fee applied when crossing midnight**
    - **Property 14: Penalty and fee flags**
    - Test file: `services/billing/internal/usecase/pricing_test.go`
    - Use gopter to generate random PricingInput values (duration 0.01–48h, boolean flags, cancel minutes 0–120)
    - **Validates: Requirements 12.1, 12.2, 12.3, 12.4, 12.5, 12.6, 12.7, 12.9**

  - [x] 6.3 Update Billing model and repository for checkout idempotency
    - Add `IdempotencyKey`, `PaymentID`, `QRCode` fields to BillingRecord in `billing.go`
    - Add `NoshowFee` field to PricingOutput
    - Add `GetByIdempotencyKey(ctx, key)` and `SetIdempotencyKey(ctx, key, invoiceID)` to BillingRepository interface
    - Implement in `billing_postgres.go` — idempotency via Redis (`idempotency:checkout:{key}` → invoice_id, TTL 24h)
    - Add Redis client dependency to billing repo
    - _Requirements: 13.3, 18.4, 18.5_

  - [x] 6.4 Implement Payment gRPC client adapter (`services/billing/internal/adapter/payment_client.go`)
    - Wrap `PaymentServiceClient.CreatePayment()` — call Payment Service to generate QRIS QR code on checkout
    - _Requirements: 13.1_

  - [x] 6.5 Implement RabbitMQ publisher for Billing (`services/billing/internal/adapter/publisher.go`)
    - Publish `checkout.completed` and `checkout.failed` events to `events.exchange` (topic)
    - _Requirements: 16.1_

  - [x] 6.6 Update BillingUsecase with full checkout logic
    - Add Payment gRPC client and RabbitMQ publisher dependencies
    - Implement idempotency check in `Checkout`: check Redis for existing invoice before processing
    - Include noshow_fee and cancellation_fee in total calculation via pricing engine
    - Call `Payment.CreatePayment` on checkout to get QR code and payment_id
    - Store payment_id and qr_code in billing record
    - Publish `checkout.completed` or `checkout.failed` event to RabbitMQ
    - Handle NOT_FOUND when billing record doesn't exist for reservation
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 13.1, 13.2, 13.3, 13.4, 18.4, 18.5_

  - [ ]* 6.7 Write property test for checkout idempotency (Property 16)
    - **Property 16: Checkout idempotency**
    - Same idempotency key twice → same invoice_id, exactly one billing record
    - Test file: `services/billing/internal/usecase/billing_usecase_test.go`
    - **Validates: Requirements 13.3, 18.4, 18.5**

  - [x] 6.8 Wire Billing Service main.go
    - Uncomment and wire pgxpool, Redis client, Payment gRPC client, RabbitMQ publisher
    - Wire repo, usecase (with pricing engine), handler
    - Register BillingServiceServer on gRPC server
    - Add auth interceptor
    - _Requirements: 21.1, 21.3, 21.4_


- [x] 7. Checkpoint — Search and Billing Services complete
  - Ensure Search Service cache-aside works (Redis hit/miss)
  - Ensure Billing pricing engine passes all property tests
  - Ensure checkout idempotency works
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 8. Payment Service — Settlement stub and circuit breaker
  - [x] 8.1 Implement settlement HTTP client (`services/payment/internal/adapter/settlement_client.go`)
    - Implement `settlementClient` interface
    - `RequestQRIS(ctx, invoiceID, amount)`: POST to `{SETTLEMENT_URL}/v1/qris/create`, parse QR code from response
    - `CheckStatus(ctx, paymentID)`: GET `{SETTLEMENT_URL}/v1/settlement/{id}`, parse status (PAID/FAILED)
    - Use `net/http` with 5s timeout
    - _Requirements: 14.1, 14.2_

  - [x] 8.2 Add circuit breaker fallback for OPEN state in PaymentUsecase
    - When circuit breaker is OPEN, return Payment with status=PENDING (no QR code) as fallback
    - Handle idempotency for CreatePayment (check existing payment by idempotency key before creating)
    - Fix gobreaker settings: trip after 5 consecutive failures (currently >5, should be >=5)
    - _Requirements: 14.6, 14.7_

  - [ ]* 8.3 Write unit tests for Payment Service
    - Test CreatePayment happy path with mocked settlement client
    - Test GetPaymentStatus with status transitions (PENDING → PAID, PENDING → FAILED)
    - Test RetryPayment creates new payment record
    - Test circuit breaker OPEN state returns PENDING fallback
    - Test file: `services/payment/internal/usecase/payment_usecase_test.go`
    - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5, 14.6, 14.7_

  - [x] 8.4 Wire Payment Service main.go
    - Uncomment and wire pgxpool, settlement HTTP client (from SETTLEMENT_URL env), repo, usecase, handler
    - Register PaymentServiceServer on gRPC server
    - Add auth interceptor
    - _Requirements: 21.1, 21.3, 21.4_

- [ ] 9. Reservation Service — War booking, holds, expiry, events
  - [x] 9.1 Add new repository methods for Reservation Service
    - Add `ReleaseLock(ctx, spotID)`, `GetHoldOwner(ctx, spotID)`, `SetCheckinAt(ctx, id, time)`, `GetExpiredReservations(ctx)` to ReservationRepository interface
    - Implement in `reservation_postgres.go`: `ReleaseLock` (Redis DEL `lock:{spot_id}`), `GetHoldOwner` (Redis GET `hold:{spot_id}`), `SetCheckinAt` (SQL UPDATE), `GetExpiredReservations` (query RESERVED with expires_at < now)
    - _Requirements: 5.1, 8.2, 9.3, 10.1, 19.3, 19.4_

  - [x] 9.2 Implement Search gRPC client adapter (`services/reservation/internal/adapter/search_client.go`)
    - Wrap `SearchServiceClient.GetFirstAvailable()` for system-assigned mode
    - _Requirements: 6.1_

  - [x] 9.3 Implement Billing gRPC client adapter (`services/reservation/internal/adapter/billing_client.go`)
    - Wrap `BillingServiceClient.ChargeBookingFee()`, `ApplyPenalty()`, `StartBillingSession()`
    - _Requirements: 6.4, 8.2, 8.3, 10.2_

  - [x] 9.4 Implement RabbitMQ publisher (`services/reservation/internal/adapter/publisher.go`)
    - Publish booking messages to `booking.exchange` (x-consistent-hash) with routing_key=spot_id
    - Publish domain events to `events.exchange` (topic): reservation.confirmed, checkin.confirmed, penalty.applied, reservation.cancelled, reservation.expired
    - _Requirements: 6.2, 8.5, 9.3, 10.4, 20.1_

  - [x] 9.5 Implement queue worker for war booking (`services/reservation/internal/usecase/queue_worker.go`)
    - AMQP consumer for booking queue: receive message → acquire Redis lock (`lock:{spot_id}`, TTL 1h) → create reservation → charge booking fee via Billing → store idempotency key → ack
    - On lock failure: nack for retry with next spot (system-assigned) or return error (user-selected)
    - Publish `reservation.confirmed` event on success
    - _Requirements: 6.3, 6.4, 6.5, 7.3, 20.2, 20.3_

  - [x] 9.6 Update ReservationUsecase with full business logic
    - Add Search gRPC client, Billing gRPC client, RabbitMQ publisher dependencies
    - System-assigned mode: call Search.GetFirstAvailable → pre-validate Redis cache → publish to booking.exchange
    - User-selected mode: validate hold ownership via `GetHoldOwner` → publish to booking.exchange
    - Hold validation: check hold exists and is owned by requesting driver; return FAILED_PRECONDITION if expired/wrong owner
    - CheckIn: verify status=RESERVED, set checkin_at, release Redis lock, call Billing.StartBillingSession; if wrong spot, call Billing.ApplyPenalty(200000); publish checkin.confirmed or penalty.applied event
    - CancelReservation: verify status=RESERVED, calculate elapsed time, release Redis lock (`lock:{spot_id}`), call Billing to record cancellation fee, publish reservation.cancelled event
    - Extract driver_id from gRPC context (auth interceptor) in handler instead of hardcoded TODO
    - Pre-validate spot availability from Redis cache before publishing to RabbitMQ
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 6.1, 6.2, 6.4, 6.5, 6.6, 6.7, 7.1, 7.2, 7.3, 7.4, 7.5, 8.1, 8.2, 8.3, 8.4, 8.5, 9.1, 9.2, 9.3, 9.4, 18.1, 18.2, 18.3, 19.1, 19.2, 19.3, 19.4, 20.1, 20.4_

  - [x] 9.7 Implement expiry worker (`services/reservation/internal/usecase/expiry_worker.go`)
    - Background goroutine: periodically scan for reservations where status=RESERVED and expires_at < now
    - For each expired reservation: update status to EXPIRED, call Billing.ApplyPenalty for no-show fee (10,000 IDR), release Redis lock, publish `reservation.expired` event
    - Skip if reservation status is ACTIVE or COMPLETED
    - _Requirements: 10.1, 10.2, 10.3, 10.4_

  - [ ]* 9.8 Write property tests for Reservation Service (Properties 15, 17–20)
    - **Property 15: Reservation idempotency** — same key twice → same reservation_id
    - **Property 17: Cancellation fee based on elapsed time** — ≤2min → 0, >2min → 5000
    - **Property 18: Check-in wrong-spot detection** — different spot → penalty 200000, same spot → 0
    - **Property 19: Hold prevents concurrent access** — second hold fails with ALREADY_EXISTS
    - **Property 20: Lock prevents double-booking** — SETNX false → spot unavailable
    - Test file: `services/reservation/internal/usecase/reservation_usecase_test.go`
    - Use mocked repository for property tests
    - **Validates: Requirements 5.1, 5.3, 6.7, 7.5, 8.2, 8.3, 9.1, 9.2, 18.1, 18.2, 19.1, 19.2**

  - [x] 9.9 Wire Reservation Service main.go
    - Wire pgxpool, Redis, RabbitMQ connection
    - Wire Search gRPC client (dial SEARCH_ADDR), Billing gRPC client (dial BILLING_ADDR)
    - Wire repo, usecase (with all adapters), handler
    - Register ReservationServiceServer on gRPC server
    - Start queue worker goroutine (consume from booking queue)
    - Start expiry worker goroutine
    - Add auth interceptor
    - _Requirements: 21.1, 21.3, 21.4_


- [x] 10. Checkpoint — Reservation and Payment Services complete
  - Ensure war booking flow works: publish → queue worker → lock → create reservation → charge fee
  - Ensure hold, cancel, check-in, expiry logic passes all property tests
  - Ensure Payment circuit breaker fallback works
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 11. Presence Service — Geofence detection and streaming
  - [x] 11.1 Implement Reservation gRPC client adapter (`services/presence/internal/adapter/reservation_client.go`)
    - Wrap `ReservationServiceClient.CheckIn()` and `GetReservation()`
    - _Requirements: 15.3_

  - [x] 11.2 Create geofence configuration (`services/presence/configs/geofences.json`)
    - Generate static geofence coordinates for all 400 spots
    - Each entry: spot_id, latitude, longitude, radius_m (default 5.0m)
    - _Requirements: 15.2_

  - [x] 11.3 Enhance PresenceUsecase with full geofence logic
    - Load geofence data from `configs/geofences.json` on startup (populate `geofences` map)
    - Track per-stream state (last known spot) to detect GEOFENCE_EXITED
    - Distinguish events: GEOFENCE_ENTERED (entered any spot geofence), CHECKIN_TRIGGERED (entered reserved spot → call Reservation.CheckIn), WRONG_SPOT_DETECTED (entered different spot)
    - Emit GEOFENCE_EXITED when driver leaves a previously entered geofence
    - Call Reservation.GetReservation to look up reserved spot_id for comparison
    - _Requirements: 15.1, 15.2, 15.3, 15.4, 15.5_

  - [ ]* 11.4 Write property test for geofence detection (Property 21)
    - **Property 21: Geofence detection correctness**
    - For any (lat, lng) and geofence (center, radius): haversine ≤ radius → detected; > radius for all → no event
    - Test file: `services/presence/internal/usecase/presence_usecase_test.go`
    - **Validates: Requirements 15.2, 15.3, 15.4**

  - [x] 11.5 Wire Presence Service main.go
    - Wire Reservation gRPC client (dial RESERVATION_ADDR), geofence config loader
    - Wire usecase, handler
    - Register PresenceServiceServer on gRPC server
    - Fix default port to `:50056`
    - Add auth interceptor (skip for StreamLocation if needed, or validate initial token)
    - _Requirements: 21.1, 21.3, 21.4_

- [ ] 12. Notification Service — RabbitMQ wiring and provider stub
  - [x] 12.1 Implement notification provider HTTP client (`services/notification/internal/adapter/notif_provider.go`)
    - Implement `notifProvider` interface
    - POST event JSON to external notification provider stub URL (from NOTIF_PROVIDER_URL env)
    - Wrap with gobreaker circuit breaker for resilience
    - _Requirements: 16.1, 16.2_

  - [x] 12.2 Wire Notification Service main.go
    - Uncomment and wire RabbitMQ connection (from RABBITMQ_URL env)
    - Wire notification provider stub client, usecase, AMQP consumer handler
    - Start consuming from `notification.queue`
    - Handle all domain events: reservation.confirmed, checkin.confirmed, penalty.applied, reservation.cancelled, reservation.expired, checkout.completed, checkout.failed
    - _Requirements: 16.1, 16.2, 16.3, 21.2, 21.3, 21.4_

- [ ] 13. Analytics Service — RabbitMQ wiring and event fields
  - [x] 13.1 Update Analytics model and repository for driver_id and vehicle_type
    - Add `DriverID` and `VehicleType` fields to `TransactionEvent` in `event.go`
    - Update `analytics_postgres.go` INSERT to include driver_id and vehicle_type columns
    - Update `analytics_usecase.go` to extract driver_id and vehicle_type from event payload
    - _Requirements: 17.1_

  - [x] 13.2 Wire Analytics Service main.go
    - Uncomment and wire pgxpool (from DATABASE_URL env), RabbitMQ connection (from RABBITMQ_URL env)
    - Wire repo, usecase, AMQP consumer handler
    - Start consuming from `analytics.queue`
    - Ack after successful DB insert, nack on failure for redelivery
    - _Requirements: 17.1, 17.2, 17.3, 21.2, 21.3, 21.4_

- [x] 14. Checkpoint — All 8 services wired
  - Verify all service main.go files compile and wire dependencies correctly
  - Verify each gRPC service registers its handler and starts listening on the correct port
  - Verify AMQP consumer services connect to RabbitMQ and start consuming
  - Ensure all tests pass, ask the user if questions arise.


- [ ] 15. Integration tests — Cross-service flows
  - [ ]* 15.1 Write integration tests for Reservation → Billing flow
    - Test CreateReservation triggers ChargeBookingFee via Billing gRPC client
    - Test CheckIn triggers StartBillingSession (correct spot) and ApplyPenalty (wrong spot)
    - Test CancelReservation triggers cancellation fee recording in Billing
    - Test expiry triggers no-show fee in Billing
    - Use mocked gRPC servers or testcontainers
    - Test file: `services/reservation/internal/usecase/reservation_usecase_test.go`
    - _Requirements: 6.4, 8.2, 8.3, 9.3, 10.2_

  - [ ]* 15.2 Write integration tests for Billing → Payment flow
    - Test Checkout calls Payment.CreatePayment and stores payment_id + qr_code
    - Test Checkout publishes checkout.completed event to RabbitMQ
    - Use mocked Payment gRPC server
    - Test file: `services/billing/internal/usecase/billing_usecase_test.go`
    - _Requirements: 13.1, 13.2_

  - [ ]* 15.3 Write integration tests for RabbitMQ message flow
    - Test Reservation publishes booking message to consistent hash exchange
    - Test queue worker consumes and processes booking message
    - Test domain events reach notification.queue and analytics.queue
    - Use testcontainers for RabbitMQ
    - Test file: `services/reservation/internal/adapter/publisher_test.go`
    - _Requirements: 20.1, 20.2, 20.3_

  - [ ]* 15.4 Write integration tests for Redis behavior
    - Test hold TTL expiry (60s) releases spot automatically
    - Test lock TTL expiry (1h) allows new reservation
    - Test idempotency key storage and retrieval (24h TTL)
    - Test availability cache hit/miss (10s TTL)
    - Use testcontainers for Redis
    - Test file: `services/reservation/internal/repository/reservation_postgres_test.go`
    - _Requirements: 4.2, 4.3, 4.4, 5.4, 18.1, 18.3, 19.1_

- [ ] 16. E2E test scenarios
  - [ ]* 16.1 Write E2E test suite covering all 25 scenarios
    - Scenario 1: Driver registration (POST /v1/auth/register → 201)
    - Scenario 2: Successful login returning JWT (POST /v1/auth/login → 200)
    - Scenario 3: Login with invalid credentials → 401
    - Scenario 4: Access endpoint without token → 401
    - Scenario 5: Access endpoint with expired token → 401
    - Scenario 6: Refresh token → new access token
    - Scenario 7: Logout → token blacklisted
    - Scenario 8: Access endpoint after logout → 401
    - Scenario 9: Get driver profile
    - Scenario 10: Update driver profile
    - Scenario 11: Register duplicate license plate + vehicle type → 409
    - Scenario 12: Happy path system-assigned: login → availability → reserve → check-in → checkout → payment
    - Scenario 13: Happy path user-selected: login → availability → hold → reserve → check-in → checkout → payment
    - Scenario 14: Double-book prevention: two concurrent reservations → second gets 409
    - Scenario 15: Spot contention: Driver A holds, Driver B tries → 409 SPOT_HELD
    - Scenario 16: Reservation expiry (no-show): reserve → wait TTL → status=EXPIRED
    - Scenario 17: Wrong-spot penalty: check-in at different spot → penalty 200,000 IDR
    - Scenario 18: Cancellation ≤ 2 min → fee=0
    - Scenario 19: Cancellation > 2 min → fee=5,000
    - Scenario 20: Extended stay billing (no overstay penalty)
    - Scenario 21: Overnight fee: session crosses midnight → overnight_fee=20,000
    - Scenario 22: Payment success QRIS: checkout → poll → PAID → COMPLETED
    - Scenario 23: Payment failure + retry: FAILED → retry → new QR code
    - Scenario 24: Idempotent reservation: same key twice → same reservation_id
    - Scenario 25: Idempotent checkout: same key twice → same invoice_id
    - Test file: `sre/e2e/parkir-pintar.postman_collection.json` or Go-based E2E test
    - _Requirements: 24.1–24.25_

- [x] 17. Final checkpoint — All tests pass
  - Run all unit tests across all services: `go test -v -race -short ./...` per service
  - Run all property-based tests
  - Verify all 8 services compile: `go build ./cmd` per service
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation after each major service is complete
- Property tests validate universal correctness properties from the design document (23 properties total)
- Unit tests validate specific examples and edge cases
- The design uses Go as the implementation language — all code follows existing project conventions (Clean Architecture, zerolog, pgxpool, go-redis, amqp091-go)
- Services are at `services/{service}/` as confirmed by `go.work`
- K8s manifests in `sre/kubernetes/base/` are already created (Requirement 25) — no implementation tasks needed for infra config files
