# Project Structure & Architecture

## Monorepo Layout

```
.
├── proto/{service}/              # Shared protobuf definitions (gRPC contracts)
├── services/{service}/           # Independent Go microservices
├── sre/                          # Infrastructure (k8s, terraform, observability, e2e)
├── .github/workflows/            # Per-service CI/CD pipelines
├── go.work                       # Go workspace linking all service modules
└── .kiro/steering/               # AI assistant steering rules
```

## Services

| Service | Transport | Port | Storage |
|---|---|---|---|
| user | gRPC | 50051 | PostgreSQL, Redis |
| reservation | gRPC | 50052 | PostgreSQL, Redis, RabbitMQ |
| billing | gRPC | 50053 | PostgreSQL |
| payment | gRPC | 50054 | PostgreSQL |
| search | gRPC | 50055 | PostgreSQL (read replica), Redis |
| presence | gRPC stream | 50056 | — |
| notification | AMQP consumer | — | — |
| analytics | AMQP consumer | — | PostgreSQL |

## Per-Service Structure (Clean Architecture)

```
services/{service}/
├── cmd/main.go                   # Entrypoint — wire dependencies, start server
├── internal/
│   ├── handler/                  # Transport layer (gRPC handlers or AMQP consumers)
│   ├── usecase/                  # Business logic (pure, no framework dependency)
│   ├── repository/               # Data access (PostgreSQL via pgx, Redis via go-redis)
│   └── model/                    # Domain models and DTOs
├── pkg/proto/                    # Generated gRPC stubs (from proto/ definitions)
├── configs/                      # Config files / env defaults
├── build/
│   ├── Dockerfile                # Multi-stage build (golang:alpine → distroless)
│   └── ci/deployment.yaml        # Kubernetes manifests
├── go.mod
└── go.sum
```

## Dependency Direction (Clean Architecture)

```
handler → usecase → repository → model
```

- `handler` depends on `usecase` (interface)
- `usecase` depends on `repository` (interface) and `model`
- `repository` depends on `model` only
- `model` has zero dependencies

Business logic in `usecase` must not depend on transport (gRPC), database drivers (pgx), or frameworks. Repository interfaces are defined in `repository/` as Go interfaces, with concrete implementations (e.g. `*_postgres.go`) in the same package.

## Conventions

- **Module path**: `github.com/parkir-pintar/{service}`
- **Naming**: Handlers are `{Service}Handler`, usecases are `{Service}Usecase`, repos are `{Service}Repository`
- **Constructor pattern**: `New{Type}(deps) *{Type}` or `New{Type}(deps) {Interface}`
- **Usecase returns interfaces**: Usecase constructors return the interface type, not the concrete struct
- **Config via env vars**: Services read config from environment variables (e.g. `GRPC_ADDR`, `DATABASE_URL`, `REDIS_ADDR`), with `envOr()` helper for defaults
- **Idempotency**: Supported via idempotency keys in gRPC metadata, stored in Redis with TTL
- **Error handling**: gRPC status codes mapped in handler layer; usecases return plain Go errors
- **Logging**: `zerolog` with `ConsoleWriter` for local dev, structured JSON in production
- **IDs**: UUIDs via `github.com/google/uuid`

## Event-Driven Services

Notification and Analytics services consume from RabbitMQ instead of exposing gRPC endpoints. They use an `AMQPConsumer` handler that reads messages, delegates to the usecase, and acks/nacks accordingly.

## Two Service Patterns

1. **gRPC services** (user, reservation, billing, payment, search, presence): Handler implements the generated `{Service}ServiceServer` interface, embeds `Unimplemented{Service}ServiceServer`
2. **AMQP consumer services** (notification, analytics): Handler wraps an `amqp.Connection`, consumes from a queue, delegates to usecase
