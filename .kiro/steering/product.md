# ParkirPintar — Smart Parking Marketplace

ParkirPintar is a smart parking reservation and billing platform for a single parking facility (5 floors, 150 car spots, 250 motorcycle spots).

## Core Capabilities

- **Reservation**: System-assigned or user-selected spot booking with hold queues, expiry, and cancellation
- **Check-in/Check-out**: Geofence or manual check-in with wrong-spot penalty detection
- **Billing**: Hourly pricing engine (gorules/JDM), overnight fees, penalties, invoice generation
- **Payment**: QRIS-based post-session payment with idempotent checkout and retry
- **Presence**: Real-time driver location via gRPC bidirectional stream or MQTT bridge
- **Search**: Spot availability queries per floor and vehicle type (Redis cache + read replica)
- **User**: Driver registration/login by license plate + vehicle type, JWT auth
- **Notification**: Event-driven internal notifications (RabbitMQ consumer, external provider is a stub)
- **Analytics**: Transaction event consumption and storage for business monitoring

## Key Business Rules

| Rule | Value |
|---|---|
| Booking fee | 5,000 IDR on confirm |
| Hourly rate | 5,000 IDR per started hour |
| Overnight fee | 20,000 IDR flat (crosses midnight) |
| Wrong spot penalty | 200,000 IDR |
| Cancel ≤ 2 min | Free |
| Cancel > 2 min | 5,000 IDR |
| No-show (> 1 hour) | 10,000 IDR |
| Overstay | No penalty, standard hourly rate |

## Spot ID Format

`{FLOOR}-{TYPE}-{NUMBER}` — e.g. `1-CAR-01`, `3-MOTO-25`

## Domain Language

The codebase and README use a mix of Indonesian and English. Code identifiers, proto definitions, and variable names are in English. Comments and documentation may contain Indonesian.
