FROM golang:1.25-alpine
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
COPY sre/e2e/integration/go.mod sre/e2e/integration/go.sum ./sre/e2e/integration/
WORKDIR /app/sre/e2e/integration
RUN GOWORK=off go mod download
WORKDIR /app
COPY services/ ./services/
COPY sre/e2e/integration/ ./sre/e2e/integration/
WORKDIR /app/sre/e2e/integration
ENTRYPOINT ["go", "test", "-v", "-tags", "integration", "-count=1", "-timeout", "180s", "./..."]
