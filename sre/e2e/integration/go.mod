module github.com/parkir-pintar/e2e-integration

go 1.25.0

require (
	github.com/jackc/pgx/v5 v5.9.2
	github.com/parkir-pintar/billing v0.0.0
	github.com/parkir-pintar/payment v0.0.0
	github.com/parkir-pintar/reservation v0.0.0
	github.com/parkir-pintar/search v0.0.0
	github.com/redis/go-redis/v9 v9.18.0
	google.golang.org/grpc v1.80.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/parkir-pintar/billing => ../../../services/billing
	github.com/parkir-pintar/payment => ../../../services/payment
	github.com/parkir-pintar/reservation => ../../../services/reservation
	github.com/parkir-pintar/search => ../../../services/search
)
