# Requirements Document

## Introduction

ParkirPintar is a smart parking reservation and billing backend platform for a single parking facility (5 floors, 150 car spots, 250 motorcycle spots). The system provides end-to-end parking lifecycle management: driver registration, spot search, reservation (system-assigned or user-selected), check-in with wrong-spot detection, hourly billing with overnight fees, QRIS-based payment, and real-time presence streaming. The backend is implemented as a Go microservices monorepo using gRPC, PostgreSQL (database-per-service), Redis (distributed locks, caching, idempotency), and RabbitMQ (event-driven messaging). This requirements document covers the core business logic implementation for all eight services, the pricing engine, concurrency controls, and end-to-end test scenarios required by the Solution Development Assessment 2026.

## Glossary

- **Reservation_Service**: The gRPC microservice (port 50052) responsible for creating, holding, cancelling, checking in, and expiring parking reservations. Uses PostgreSQL, Redis, and RabbitMQ.
- **Billing_Service**: The gRPC microservice (port 50053) responsible for charging booking fees, starting billing sessions, applying penalties, calculating invoices, and executing checkout. Uses PostgreSQL and the Pricing_Engine.
- **Payment_Service**: The gRPC microservice (port 50054) responsible for creating QRIS payments, checking settlement status, and retrying failed payments. Uses PostgreSQL and a circuit breaker (gobreaker) for the external settlement stub.
- **User_Service**: The gRPC microservice (port 50051) responsible for driver registration, login, JWT issuance, token validation, logout (blacklist), refresh token rotation, and profile management. Uses PostgreSQL and Redis.
- **Search_Service**: The gRPC microservice (port 50055) responsible for querying spot availability per floor and vehicle type. Uses PostgreSQL read replica and Redis cache.
- **Presence_Service**: The gRPC streaming microservice (port 50056) responsible for receiving real-time driver location updates via bidirectional stream and triggering geofence-based check-in events.
- **Notification_Service**: The AMQP consumer service that consumes domain events from RabbitMQ and forwards them to an external notification provider stub.
- **Analytics_Service**: The AMQP consumer service that consumes transaction events from RabbitMQ and stores them in PostgreSQL for business monitoring.
- **Pricing_Engine**: A pure, stateless pricing calculator (gorules/JDM or fallback Go implementation) that computes fees based on duration, midnight crossing, penalties, cancellation elapsed time, and no-show status.
- **Spot_ID**: A parking spot identifier in the format `{FLOOR}-{TYPE}-{NUMBER}` (e.g., `1-CAR-01`, `3-MOTO-25`).
- **Idempotency_Key**: A client-generated UUID passed via gRPC metadata to ensure duplicate requests return the same result without side effects.
- **Redis_Lock**: A distributed lock implemented via Redis `SETNX` with TTL to prevent double-booking of parking spots.
- **Booking_Exchange**: A RabbitMQ Consistent Hash Exchange (`booking.exchange`) that routes booking messages by `spot_id` hash to ensure serial processing per spot.
- **Driver**: A registered user identified by license plate and vehicle type who uses the parking facility.
- **Invoice**: A billing summary document generated at checkout containing all fee components (booking, hourly, overnight, penalty, no-show, cancellation) and a total amount.
- **Settlement_Stub**: An external HTTP mock service simulating QRIS payment settlement (Pondo Ngopi engine).
- **Circuit_Breaker**: A `sony/gobreaker` instance wrapping HTTP calls to the Settlement_Stub, opening after 5 consecutive failures with a 30-second timeout.

## Requirements

### Requirement 1: Driver Registration

**User Story:** As a driver, I want to register with my license plate and vehicle type, so that I can use the parking system.

#### Acceptance Criteria

1. WHEN a registration request is received with a license plate, vehicle type, name, and optional phone number, THE User_Service SHALL create a new driver record and return the driver ID.
2. WHEN a registration request contains a license plate and vehicle type combination that already exists, THE User_Service SHALL return a CONFLICT error (gRPC `ALREADY_EXISTS`) without creating a duplicate record.
3. THE User_Service SHALL store the driver password using bcrypt with a cost factor of 12.
4. WHEN a registration request is missing the license plate or vehicle type field, THE User_Service SHALL return an INVALID_ARGUMENT error.

### Requirement 2: Driver Authentication

**User Story:** As a driver, I want to log in with my credentials, so that I receive a JWT token for accessing parking services.

#### Acceptance Criteria

1. WHEN valid credentials (license plate, vehicle type, password) are provided, THE User_Service SHALL return a JWT access token (HS256, 1-hour expiry, claims: `sub`=driver_id, `jti`=uuid) and a refresh token (opaque, stored in Redis with 7-day TTL).
2. WHEN invalid credentials are provided, THE User_Service SHALL return an UNAUTHENTICATED error without revealing whether the license plate exists.
3. WHEN a valid refresh token is provided, THE User_Service SHALL issue a new access token and rotate the refresh token.
4. WHEN an expired or invalid refresh token is provided, THE User_Service SHALL return an UNAUTHENTICATED error.
5. WHEN a logout request is received, THE User_Service SHALL add the JWT `jti` to a Redis blacklist with TTL equal to the token's remaining expiry time and delete the refresh token from Redis.
6. WHEN a token validation request is received for a blacklisted `jti`, THE User_Service SHALL return UNAUTHENTICATED.

### Requirement 3: Driver Profile Management

**User Story:** As a driver, I want to view and update my profile, so that I can keep my information current.

#### Acceptance Criteria

1. WHEN a GetProfile request is received with a valid driver ID, THE User_Service SHALL return the driver's license plate, vehicle type, name, and phone number.
2. WHEN an UpdateProfile request is received with a valid driver ID, THE User_Service SHALL update the name and phone number fields and return the updated profile.
3. WHEN a GetProfile or UpdateProfile request references a non-existent driver ID, THE User_Service SHALL return a NOT_FOUND error.

### Requirement 4: Spot Availability Search

**User Story:** As a driver, I want to search for available parking spots by floor and vehicle type, so that I can decide where to park.

#### Acceptance Criteria

1. WHEN a GetAvailability request is received with a floor number and vehicle type, THE Search_Service SHALL return the list of available spots and the total available count for that floor and vehicle type.
2. THE Search_Service SHALL first check the Redis cache (key `availability:{floor}:{type}`, TTL 10 seconds) before querying the PostgreSQL read replica.
3. WHEN the Redis cache contains a valid entry, THE Search_Service SHALL return the cached result without querying PostgreSQL.
4. WHEN the Redis cache misses, THE Search_Service SHALL query the PostgreSQL read replica and populate the cache with a 10-second TTL.
5. WHEN a GetFirstAvailable request is received with a vehicle type, THE Search_Service SHALL return the first available spot across all floors for that vehicle type.
6. WHEN no spots are available for the requested vehicle type, THE Search_Service SHALL return a NOT_FOUND error.

### Requirement 5: Spot Hold (User-Selected Mode)

**User Story:** As a driver, I want to temporarily hold a specific spot while I confirm my reservation, so that no other driver takes it during my decision.

#### Acceptance Criteria

1. WHEN a HoldSpot request is received with a spot ID and driver ID, THE Reservation_Service SHALL acquire a Redis lock (`hold:{spot_id}`, value=driver_id, TTL 60 seconds) using SETNX.
2. WHEN the hold lock is acquired, THE Reservation_Service SHALL return the spot ID and the hold expiry timestamp (60 seconds from now).
3. WHEN the spot is already held by another driver, THE Reservation_Service SHALL return an ALREADY_EXISTS error with reason SPOT_HELD.
4. WHEN the 60-second hold TTL expires without a reservation confirmation, THE Reservation_Service SHALL allow the spot to become available for other drivers automatically via Redis TTL expiry.

### Requirement 6: Create Reservation (System-Assigned Mode)

**User Story:** As a driver, I want the system to automatically assign me the first available spot, so that I can reserve quickly without browsing.

#### Acceptance Criteria

1. WHEN a CreateReservation request is received with mode=SYSTEM_ASSIGNED and a vehicle type, THE Reservation_Service SHALL call the Search_Service to get the first available spot for that vehicle type.
2. WHEN an available spot is found, THE Reservation_Service SHALL publish a booking message to the Booking_Exchange with routing_key=spot_id.
3. WHEN the queue worker receives the booking message, THE Reservation_Service SHALL acquire a Redis lock (`lock:{spot_id}`, TTL 1 hour) using SETNX.
4. WHEN the Redis lock is acquired, THE Reservation_Service SHALL insert a reservation record with status=RESERVED, charge a booking fee of 5,000 IDR via the Billing_Service, and store the idempotency key in Redis (TTL 24 hours).
5. WHEN the Redis lock fails (spot taken by concurrent request), THE Reservation_Service SHALL retry with the next available spot from the Search_Service.
6. WHEN no spots are available for the requested vehicle type, THE Reservation_Service SHALL return an UNAVAILABLE error.
7. WHEN a CreateReservation request contains an idempotency key that already exists in Redis, THE Reservation_Service SHALL return the previously created reservation without creating a duplicate.

### Requirement 7: Create Reservation (User-Selected Mode)

**User Story:** As a driver, I want to reserve a specific spot that I have selected, so that I can park in my preferred location.

#### Acceptance Criteria

1. WHEN a CreateReservation request is received with mode=USER_SELECTED and a spot ID, THE Reservation_Service SHALL verify that the requesting driver owns the active hold on that spot.
2. WHEN the hold is valid and owned by the requesting driver, THE Reservation_Service SHALL publish a booking message to the Booking_Exchange with routing_key=spot_id.
3. WHEN the queue worker acquires the Redis lock (`lock:{spot_id}`, TTL 1 hour), THE Reservation_Service SHALL delete the hold key, insert a reservation record with status=RESERVED, charge a booking fee of 5,000 IDR via the Billing_Service, and store the idempotency key in Redis (TTL 24 hours).
4. WHEN the hold has expired or is owned by a different driver, THE Reservation_Service SHALL return a FAILED_PRECONDITION error with reason HOLD_EXPIRED.
5. WHEN a CreateReservation request contains an idempotency key that already exists in Redis, THE Reservation_Service SHALL return the previously created reservation without creating a duplicate.

### Requirement 8: Check-In

**User Story:** As a driver, I want to check in when I arrive at the parking facility, so that my billing session starts.

#### Acceptance Criteria

1. WHEN a CheckIn request is received with a reservation ID and actual spot ID, THE Reservation_Service SHALL verify the reservation exists and has status=RESERVED.
2. WHEN the actual spot ID matches the reserved spot ID, THE Reservation_Service SHALL update the reservation status to ACTIVE, set checkin_at to the current timestamp, release the inventory lock from Redis, and call the Billing_Service to start a billing session.
3. WHEN the actual spot ID does not match the reserved spot ID, THE Reservation_Service SHALL update the reservation status to ACTIVE, call the Billing_Service to apply a wrong-spot penalty of 200,000 IDR, and then start the billing session.
4. WHEN a CheckIn request references a reservation that is not in RESERVED status, THE Reservation_Service SHALL return a FAILED_PRECONDITION error.
5. THE Reservation_Service SHALL publish a `checkin.confirmed` event (or `penalty.applied` event if wrong spot) to RabbitMQ for the Notification_Service.

### Requirement 9: Cancellation

**User Story:** As a driver, I want to cancel my reservation, so that the spot is released and I am charged the appropriate cancellation fee.

#### Acceptance Criteria

1. WHEN a CancelReservation request is received for a reservation with status=RESERVED and the elapsed time since confirmed_at is 2 minutes or less, THE Reservation_Service SHALL cancel the reservation with a cancellation fee of 0 IDR.
2. WHEN a CancelReservation request is received for a reservation with status=RESERVED and the elapsed time since confirmed_at is greater than 2 minutes, THE Reservation_Service SHALL cancel the reservation with a cancellation fee of 5,000 IDR.
3. WHEN a reservation is cancelled, THE Reservation_Service SHALL update the reservation status to CANCELLED, release the Redis lock (`lock:{spot_id}`), call the Billing_Service to record the cancellation fee, and publish a `reservation.cancelled` event to RabbitMQ.
4. WHEN a CancelReservation request references a reservation that is not in RESERVED status, THE Reservation_Service SHALL return a FAILED_PRECONDITION error.

### Requirement 10: Reservation Expiry (No-Show)

**User Story:** As a parking operator, I want reservations to automatically expire after 1 hour without check-in, so that spots are released for other drivers.

#### Acceptance Criteria

1. WHEN the Redis lock TTL (1 hour) for a reservation expires and the reservation status is still RESERVED, THE Reservation_Service expiry worker SHALL update the reservation status to EXPIRED.
2. WHEN a reservation expires, THE Reservation_Service SHALL call the Billing_Service to apply a no-show fee of 10,000 IDR and release the spot to available inventory.
3. WHEN the Redis lock TTL expires but the reservation status is ACTIVE or COMPLETED, THE Reservation_Service expiry worker SHALL take no action.
4. THE Reservation_Service SHALL publish a `reservation.expired` event to RabbitMQ for the Notification_Service.

### Requirement 11: Billing Session Management

**User Story:** As a parking operator, I want the billing system to track session start and end times, so that accurate hourly fees are calculated.

#### Acceptance Criteria

1. WHEN a ChargeBookingFee request is received, THE Billing_Service SHALL create a billing record with a booking fee of 5,000 IDR and status=PENDING.
2. WHEN a StartBillingSession request is received with a reservation ID and check-in timestamp, THE Billing_Service SHALL update the billing record's session_start to the provided check-in timestamp.
3. WHEN an ApplyPenalty request is received with a reservation ID, reason, and amount, THE Billing_Service SHALL add the penalty amount to the billing record's penalty field.
4. THE Billing_Service SHALL maintain separate fee fields for booking_fee, hourly_fee, overnight_fee, penalty, noshow_fee, and cancellation_fee on each billing record.

### Requirement 12: Pricing Engine

**User Story:** As a parking operator, I want pricing rules to be configurable and hot-reloadable, so that fee structures can change without redeploying the service.

#### Acceptance Criteria

1. THE Pricing_Engine SHALL calculate the hourly fee as `ceil(duration_in_hours) * 5,000 IDR` where duration_in_hours is the elapsed time between session start and session end.
2. WHEN a parking session crosses midnight (00:00), THE Pricing_Engine SHALL add a flat overnight fee of 20,000 IDR.
3. WHEN the wrong_spot flag is true, THE Pricing_Engine SHALL add a penalty of 200,000 IDR.
4. WHEN the is_noshow flag is true, THE Pricing_Engine SHALL set the no-show fee to 10,000 IDR.
5. WHEN cancel_elapsed_minutes is greater than 2, THE Pricing_Engine SHALL set the cancellation fee to 5,000 IDR.
6. WHEN cancel_elapsed_minutes is 2 or less, THE Pricing_Engine SHALL set the cancellation fee to 0 IDR.
7. THE Pricing_Engine SHALL compute the total as the sum of booking_fee + hourly_fee + overnight_fee + penalty + cancellation_fee + noshow_fee.
8. THE Billing_Service SHALL poll the pricing_rules table every 30 seconds and reload the gorules engine when the rule version changes.
9. FOR ALL valid PricingInput values, THE Pricing_Engine SHALL produce deterministic output: evaluating the same input twice SHALL produce identical PricingOutput (round-trip property for pricing evaluation).

### Requirement 13: Checkout and Invoice Generation

**User Story:** As a driver, I want to check out and receive an invoice with a QR code for payment, so that I can pay and leave the facility.

#### Acceptance Criteria

1. WHEN a Checkout request is received with a reservation ID and idempotency key, THE Billing_Service SHALL calculate the session end time as the current timestamp, compute all fees via the Pricing_Engine, generate an invoice, and call the Payment_Service to create a QRIS payment.
2. THE Billing_Service SHALL return an InvoiceResponse containing invoice_id, reservation_id, booking_fee, hourly_fee, overnight_fee, penalty, noshow_fee, cancellation_fee, total, status, qr_code, and payment_id.
3. WHEN a Checkout request contains an idempotency key that already exists, THE Billing_Service SHALL return the previously generated invoice without creating a duplicate.
4. WHEN the Billing_Service cannot find a billing record for the given reservation ID, THE Billing_Service SHALL return a NOT_FOUND error.

### Requirement 14: QRIS Payment Processing

**User Story:** As a driver, I want to pay via QRIS after checkout, so that my parking session is completed.

#### Acceptance Criteria

1. WHEN a CreatePayment request is received with an invoice ID, amount, and idempotency key, THE Payment_Service SHALL call the Settlement_Stub to generate a QRIS QR code, create a payment record with status=PENDING, and return the payment ID and QR code.
2. WHEN a GetPaymentStatus request is received, THE Payment_Service SHALL check the Settlement_Stub for the latest status and update the payment record accordingly.
3. WHEN the settlement status is PAID, THE Payment_Service SHALL update the payment status to PAID.
4. WHEN the settlement status is FAILED, THE Payment_Service SHALL update the payment status to FAILED.
5. WHEN a RetryPayment request is received for a FAILED payment, THE Payment_Service SHALL call the Settlement_Stub to generate a new QRIS QR code and create a new payment record.
6. THE Payment_Service SHALL wrap all HTTP calls to the Settlement_Stub with a Circuit_Breaker (gobreaker: max 3 requests in half-open, 10-second interval, 30-second timeout, trip after 5 consecutive failures).
7. WHEN the Circuit_Breaker is in OPEN state, THE Payment_Service SHALL return a payment with status=PENDING as a fallback without calling the Settlement_Stub.

### Requirement 15: Presence Streaming and Geofence Detection

**User Story:** As a driver, I want my location to be streamed in real time, so that the system can auto-detect my arrival and trigger check-in.

#### Acceptance Criteria

1. WHEN a bidirectional gRPC stream is established, THE Presence_Service SHALL receive LocationUpdate messages containing reservation_id, latitude, longitude, and timestamp.
2. THE Presence_Service SHALL evaluate each location update against the reserved spot's geofence coordinates.
3. WHEN a driver enters the geofence of the reserved spot, THE Presence_Service SHALL emit a GEOFENCE_ENTERED PresenceEvent and trigger a check-in via the Reservation_Service.
4. WHEN a driver enters the geofence of a spot different from the reserved spot, THE Presence_Service SHALL emit a WRONG_SPOT_DETECTED PresenceEvent.
5. WHEN a driver exits the geofence, THE Presence_Service SHALL emit a GEOFENCE_EXITED PresenceEvent.

### Requirement 16: Event-Driven Notification

**User Story:** As a driver, I want to receive notifications about my reservation status changes, so that I stay informed throughout the parking lifecycle.

#### Acceptance Criteria

1. WHEN a domain event (reservation.confirmed, checkin.confirmed, penalty.applied, reservation.cancelled, reservation.expired, checkout.completed, checkout.failed) is published to RabbitMQ, THE Notification_Service SHALL consume the event and forward it to the external notification provider stub via HTTP.
2. WHEN the external notification provider stub is unreachable, THE Notification_Service SHALL log the failure and nack the message for redelivery.
3. THE Notification_Service SHALL acknowledge (ack) each message only after the external provider stub returns a success response.

### Requirement 17: Analytics Event Consumption

**User Story:** As a parking operator, I want transaction events stored for business monitoring, so that I can analyze parking usage patterns.

#### Acceptance Criteria

1. WHEN a transaction event is published to RabbitMQ, THE Analytics_Service SHALL consume the event and insert a transaction_events record with event_type, reservation_id, driver_id, spot_id, vehicle_type, amount, and the full event payload.
2. THE Analytics_Service SHALL acknowledge (ack) each message only after the database insert succeeds.
3. WHEN the database insert fails, THE Analytics_Service SHALL nack the message for redelivery.

### Requirement 18: Idempotency Mechanism

**User Story:** As a system operator, I want duplicate requests to return the same result without side effects, so that network retries do not cause data inconsistency.

#### Acceptance Criteria

1. WHEN a CreateReservation request includes an idempotency key, THE Reservation_Service SHALL check Redis for an existing entry (`idempotency:{key}`) before processing.
2. WHEN the idempotency key exists in Redis, THE Reservation_Service SHALL return the stored reservation without creating a new one.
3. WHEN a new reservation is created, THE Reservation_Service SHALL store the idempotency key in Redis with a 24-hour TTL.
4. WHEN a Checkout request includes an idempotency key, THE Billing_Service SHALL check for an existing invoice before processing.
5. WHEN the checkout idempotency key exists, THE Billing_Service SHALL return the stored invoice without creating a new one.
6. FOR ALL idempotency keys, submitting the same request twice with the same key SHALL return identical responses (idempotence property: f(x) = f(f(x))).

### Requirement 19: Distributed Locking for Inventory

**User Story:** As a system operator, I want parking spot inventory protected by distributed locks, so that double-booking is prevented under concurrent load.

#### Acceptance Criteria

1. WHEN a reservation is being confirmed, THE Reservation_Service SHALL acquire a Redis lock using `SETNX lock:{spot_id}` with a TTL of 1 hour.
2. WHEN the SETNX command returns false (lock already held), THE Reservation_Service SHALL treat the spot as unavailable and either retry with another spot (system-assigned) or return UNAVAILABLE (user-selected).
3. WHEN a reservation is cancelled or expires, THE Reservation_Service SHALL delete the Redis lock key to release the spot.
4. WHEN a driver checks in at the correct spot, THE Reservation_Service SHALL delete the Redis lock key since the spot is now physically occupied.

### Requirement 20: War Booking Serialization via RabbitMQ

**User Story:** As a system operator, I want concurrent booking requests for the same spot to be serialized, so that race conditions are eliminated.

#### Acceptance Criteria

1. THE Reservation_Service SHALL publish all booking requests to the Booking_Exchange (type: x-consistent-hash) with routing_key set to the spot_id.
2. THE Booking_Exchange SHALL route messages to queue workers based on a consistent hash of the spot_id, ensuring all requests for the same spot are processed by the same worker serially.
3. WHEN multiple concurrent booking requests target the same spot, THE Booking_Exchange SHALL deliver them to the same queue, and the queue worker SHALL process them one at a time in FIFO order.
4. THE Reservation_Service SHALL pre-validate spot availability from Redis cache before publishing to RabbitMQ to reduce unnecessary queue load.

### Requirement 21: Service Entrypoint and Dependency Wiring

**User Story:** As a developer, I want each service's main.go to wire all dependencies and start the gRPC server, so that services are runnable and follow clean architecture.

#### Acceptance Criteria

1. THE cmd/main.go of each gRPC service SHALL initialize database connections (pgxpool), Redis clients, RabbitMQ connections (where applicable), construct repository implementations, construct usecase instances, construct handler instances, register the gRPC service, and start listening on the configured port.
2. THE cmd/main.go of each AMQP consumer service (Notification_Service, Analytics_Service) SHALL initialize the AMQP connection, construct the usecase, construct the consumer handler, and start consuming from the configured queue.
3. EACH service SHALL read configuration from environment variables with sensible defaults using an `envOr()` helper function.
4. EACH service SHALL use zerolog for structured logging with ConsoleWriter for local development.

### Requirement 22: Parking Spot Inventory Initialization

**User Story:** As a parking operator, I want the parking spot inventory pre-populated in the database, so that the system knows about all 400 spots across 5 floors.

#### Acceptance Criteria

1. THE Reservation_Service database migration SHALL create 150 car spots (5 floors × 30 spots per floor) with Spot_IDs in the format `{FLOOR}-CAR-{NUMBER}` where NUMBER is zero-padded to 2 digits (01–30).
2. THE Reservation_Service database migration SHALL create 250 motorcycle spots (5 floors × 50 spots per floor) with Spot_IDs in the format `{FLOOR}-MOTO-{NUMBER}` where NUMBER is zero-padded to 2 digits (01–50).
3. ALL spots SHALL have initial status=AVAILABLE after migration.

### Requirement 23: gRPC Auth Interceptor

**User Story:** As a system operator, I want all driver-facing gRPC requests authenticated via JWT, so that only registered drivers can access parking services.

#### Acceptance Criteria

1. EACH gRPC service SHALL implement a unary interceptor that extracts the JWT from the `authorization` metadata header.
2. THE interceptor SHALL validate the JWT signature, expiry, and check the blacklist via the User_Service.
3. WHEN the JWT is valid and not blacklisted, THE interceptor SHALL inject the driver_id into the gRPC context for downstream use.
4. WHEN the JWT is missing, expired, or blacklisted, THE interceptor SHALL return an UNAUTHENTICATED gRPC status code.
5. THE interceptor SHALL skip authentication for the Register and Login RPCs on the User_Service.

### Requirement 24: End-to-End Test Scenarios

**User Story:** As a QA engineer, I want comprehensive end-to-end tests covering all 25 business scenarios, so that the system's correctness is verified across service boundaries.

#### Acceptance Criteria

1. THE E2E test suite SHALL include a test for driver registration (POST /v1/auth/register → 201).
2. THE E2E test suite SHALL include a test for successful login returning a JWT (POST /v1/auth/login → 200).
3. THE E2E test suite SHALL include a test for login with invalid credentials returning 401.
4. THE E2E test suite SHALL include a test for accessing an endpoint without a token returning 401.
5. THE E2E test suite SHALL include a test for accessing an endpoint with an expired token returning 401.
6. THE E2E test suite SHALL include a test for refreshing a token and receiving a new access token.
7. THE E2E test suite SHALL include a test for logout followed by token blacklisting.
8. THE E2E test suite SHALL include a test for accessing an endpoint after logout returning 401.
9. THE E2E test suite SHALL include a test for getting a driver profile.
10. THE E2E test suite SHALL include a test for updating a driver profile.
11. THE E2E test suite SHALL include a test for registering a duplicate license plate + vehicle type returning 409.
12. THE E2E test suite SHALL include a happy-path test for system-assigned reservation: login → availability → reserve → check-in → checkout → payment.
13. THE E2E test suite SHALL include a happy-path test for user-selected reservation: login → availability → hold → reserve → check-in → checkout → payment.
14. THE E2E test suite SHALL include a test for double-book prevention: two concurrent reservations on the same spot, second gets 409.
15. THE E2E test suite SHALL include a test for spot contention: Driver A holds spot, Driver B tries same spot and gets 409 SPOT_HELD.
16. THE E2E test suite SHALL include a test for reservation expiry (no-show): reserve → wait TTL → GET reservation → status=EXPIRED, spot released.
17. THE E2E test suite SHALL include a test for wrong-spot penalty: check-in at different spot → penalty 200,000 IDR applied.
18. THE E2E test suite SHALL include a test for cancellation within 2 minutes (free): reserve → cancel immediately → fee=0.
19. THE E2E test suite SHALL include a test for cancellation after 2 minutes (5,000 IDR): reserve → wait → cancel → fee=5,000.
20. THE E2E test suite SHALL include a test for extended stay billing with no overstay penalty.
21. THE E2E test suite SHALL include a test for overnight fee: session crosses midnight → overnight_fee=20,000 in invoice.
22. THE E2E test suite SHALL include a test for payment success via QRIS: checkout → poll payment → status=PAID → reservation=COMPLETED.
23. THE E2E test suite SHALL include a test for payment failure and retry: checkout → poll → status=FAILED → retry → new QR code.
24. THE E2E test suite SHALL include a test for idempotent reservation: same idempotency key twice → same reservation_id returned.
25. THE E2E test suite SHALL include a test for idempotent checkout: same idempotency key twice → same invoice_id returned.

### Requirement 25: Infrastructure Configuration Files

**User Story:** As a DevOps engineer, I want Kubernetes manifests, Istio configurations, and Terraform definitions, so that the system can be deployed to AWS EKS.

#### Acceptance Criteria

1. THE `sre/kubernetes/base/` directory SHALL contain a Deployment + Service manifest per service (e.g., `reservation-service.yaml`, `billing-service.yaml`) with resource requests (cpu: 100m, memory: 128Mi), limits (cpu: 500m, memory: 256Mi), and gRPC liveness/readiness probes. This is the single source of truth for k8s manifests — CI workflows reference these files directly for deploy.
2. EACH gRPC service SHALL have a Kubernetes Service manifest with the port named `grpc` for Istio HTTP/2 detection.
3. THE `sre/kubernetes/istio/virtual-services.yaml` SHALL contain a VirtualService for each gRPC service with appropriate timeouts and retry policies (e.g., reservation: timeout=5s, 3 retries, perTryTimeout=2s, retryOn=unavailable,reset).
4. THE `sre/kubernetes/istio/destination-rules.yaml` SHALL contain a DestinationRule for each gRPC service with LEAST_CONN load balancing and outlierDetection (consecutive 5xx errors, 10s interval, base ejection time).
5. THE `sre/kubernetes/istio/peer-authentication.yaml` SHALL contain a PeerAuthentication resource with mTLS mode=STRICT for the parkir-pintar namespace.
6. THE `sre/kubernetes/base/hpa.yaml` SHALL contain HPA manifests, with the Reservation_Service having minReplicas=3, maxReplicas=20, CPU target=60%, and custom metric grpc_server_handled_total target=500.
7. THE `sre/kubernetes/base/config.yaml` SHALL contain a ConfigMap for shared config (OTEL endpoint, gRPC port) and Secrets for each service's database, Redis, and RabbitMQ credentials.
8. THE `sre/kubernetes/base/namespace.yaml` SHALL create the `parkir-pintar` namespace with `istio-injection: enabled` label.
9. THE `sre/docker-compose.yaml` SHALL provide a local development environment with PostgreSQL (single instance, 5 databases), Redis, RabbitMQ, WireMock settlement stub, and all 8 services, enabling developers to run the full stack locally via `docker compose up`.
10. THE `sre/README.md` SHALL document the terraform destroy procedure with the correct teardown order: delete k8s namespaces first, uninstall Istio, then terraform destroy.
