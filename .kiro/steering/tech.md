# Tech Stack & Build System

## Language & Runtime

- **Go 1.25** (workspace mode via `go.work`)
- Each service is an independent Go module under `services/{name}/`

## Core Libraries

| Library | Purpose |
|---|---|
| `google.golang.org/grpc` | gRPC server and client |
| `google.golang.org/protobuf` | Protobuf serialization |
| `github.com/jackc/pgx/v5` | PostgreSQL driver (pgxpool for connection pooling) |
| `github.com/redis/go-redis/v9` | Redis client (inventory locks, holds, idempotency, caching) |
| `github.com/rabbitmq/amqp091-go` | RabbitMQ AMQP client (event-driven services) |
| `github.com/rs/zerolog` | Structured logging |
| `github.com/google/uuid` | UUID generation |
| `github.com/sony/gobreaker/v2` | Circuit breaker (Payment, Notification) |

## Communication

- **Service-to-service**: gRPC with mTLS (Istio mesh)
- **Event-driven**: RabbitMQ (Consistent Hash Exchange, partitioned by `spot_id`)
- **Client-facing**: REST via Kong Gateway → gRPC translation
- **Presence**: gRPC bidirectional stream or MQTT bridge

## Infrastructure

- **Orchestration**: Kubernetes (EKS) with Istio service mesh
- **Database**: PostgreSQL per service (database-per-service pattern), read replica for Search
- **Cache/Locks**: Redis Cluster (SETNX distributed locks, TTL-based expiry, availability cache)
- **Message Broker**: RabbitMQ
- **API Gateway**: Kong (JWT auth, rate limiting, REST routing)
- **CI/CD**: GitHub Actions (per-service workflows triggered by path filters)
- **Container Registry**: Amazon ECR
- **Observability**: Prometheus, Grafana, Loki, Tempo, OpenTelemetry

## Proto Definitions

Shared protobuf files live in `proto/{service}/` at the repo root. Generated Go stubs are placed in each service's `pkg/proto/` directory.

```
# Generate gRPC stubs (from repo root)
protoc --go_out=. --go-grpc_out=. proto/{service}/{service}.proto
```

## Common Commands

```bash
# Build a service
cd services/{service}
go build ./cmd

# Run tests for a service
cd services/{service}
go test -v -race -short ./...

# Run tests with coverage
cd services/{service}
go test -v -race -short -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Lint (requires golangci-lint)
cd services/{service}
golangci-lint run

# Build Docker image
docker build -f services/{service}/build/Dockerfile -t parkir-pintar/{service}-service services/{service}

# Download dependencies for all services (from repo root)
go work sync
```

## CI Pipeline (per service)

Each service has its own GitHub Actions workflow (`.github/workflows/ci-{service}.yml`) triggered only on changes to `services/{service}/**` or `proto/{service}/**`. Pipeline stages:

1. Code Quality — secret detection, Dockerfile lint, dependency scan
2. Verify — golangci-lint, SonarCloud
3. Security Scan
4. Unit Tests & Coverage
5. Build & Push to ECR (main/develop only)
6. Deploy to EKS (main only)
