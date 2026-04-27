module github.com/parkir-pintar/reservation

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.9.2
	github.com/parkir-pintar/billing v0.0.0
	github.com/parkir-pintar/search v0.0.0
	github.com/rabbitmq/amqp091-go v1.11.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/rs/zerolog v1.35.1
	google.golang.org/grpc v1.80.0
)

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/parkir-pintar/billing => ../billing
	github.com/parkir-pintar/payment => ../payment
	github.com/parkir-pintar/search => ../search
)
