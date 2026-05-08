FROM golang:1.25 AS builder
RUN apt-get update && apt-get install -y gcc libc6-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY go.work ./
COPY services/shared/go.mod services/shared/go.sum ./services/shared/
COPY services/reservation/go.mod services/reservation/go.sum ./services/reservation/
COPY services/billing/go.mod services/billing/go.sum ./services/billing/
COPY services/payment/go.mod services/payment/go.sum ./services/payment/
COPY services/search/go.mod services/search/go.sum ./services/search/
COPY services/presence/go.mod services/presence/go.sum ./services/presence/
COPY services/notification/go.mod services/notification/go.sum ./services/notification/
COPY services/analytics/go.mod services/analytics/go.sum ./services/analytics/
RUN cd services/billing && go mod download
COPY services/ ./services/
RUN cd services/billing && CGO_ENABLED=1 GOOS=linux go build -o /service ./cmd

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /service /service
COPY --from=builder /app/services/billing/rules/ /rules/
ENTRYPOINT ["/service"]
