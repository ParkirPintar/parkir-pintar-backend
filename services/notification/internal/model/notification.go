package model

type NotificationEvent struct {
	Type    string // reservation.confirmed | checkin.confirmed | penalty.applied | etc.
	Payload map[string]string
}
