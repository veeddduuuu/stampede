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

type Seat struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type BookingRepository interface {
	Book(b Booking) error
	ListBookings(id string) ([]Booking, error)
	ListEventBookings(eventID string) ([]Booking, error)
	Hold(b Booking) (*Booking, error)
	Release(b Booking) (*Booking, error)
}