FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.work ./
COPY services/user/go.mod services/user/go.sum ./services/user/
COPY services/reservation/go.mod services/reservation/go.sum ./services/reservation/
COPY services/billing/go.mod services/billing/go.sum ./services/billing/
COPY services/payment/go.mod services/payment/go.sum ./services/payment/
COPY services/search/go.mod services/search/go.sum ./services/search/
COPY services/presence/go.mod services/presence/go.sum ./services/presence/
COPY services/notification/go.mod services/notification/go.sum ./services/notification/
COPY services/analytics/go.mod services/analytics/go.sum ./services/analytics/
RUN cd services/user && go mod download
COPY services/ ./services/
RUN cd services/user && CGO_ENABLED=0 GOOS=linux go build -o /service ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /service /service
ENTRYPOINT ["/service"]
