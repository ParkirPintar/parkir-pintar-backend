package model

import "time"

type User struct {
	ID           string    `db:"id"`
	LicensePlate string    `db:"license_plate"`
	VehicleType  string    `db:"vehicle_type"`
	PasswordHash string    `db:"password_hash"`
	PhoneNumber  string    `db:"phone_number"`
	Name         string    `db:"name"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
