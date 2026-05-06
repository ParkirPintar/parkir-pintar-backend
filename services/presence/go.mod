module github.com/parkir-pintar/presence

go 1.25.0

require (
	github.com/parkir-pintar/billing v0.0.0
	github.com/parkir-pintar/reservation v0.0.0
	github.com/rs/zerolog v1.35.1
	google.golang.org/grpc v1.80.0
)

replace (
	github.com/parkir-pintar/billing => ../billing
	github.com/parkir-pintar/payment => ../payment
	github.com/parkir-pintar/reservation => ../reservation
	github.com/parkir-pintar/search => ../search
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
