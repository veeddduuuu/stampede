package booking

import (
	"time"
)

type Booking struct {
	ID      string
	EventID string
	SeatID  string
	UserID  string
	Status  string
	ExpiresAt time.Time
}

type BookingRepository interface {
	Book(b Booking) error
	ListBookings(id string) ([]Booking, error)
}