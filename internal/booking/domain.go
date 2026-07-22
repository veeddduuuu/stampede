package booking

import (
	"errors"
	"time"
)

var (
	ErrSeatAlreadyBooked = errors.New("seat is already booked for this event")
	ErrHoldExpired       = errors.New("seat hold expired or does not exist")
	ErrUnauthorizedHold  = errors.New("seat is held by another user")
	ErrSeatNotHeld       = errors.New("seat is not on hold")
)

type Booking struct {
	ID        string
	EventID   string
	SeatID    string
	UserID    string
	Status    string
	ExpiresAt time.Time
}

type Seat struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type BookingRepository interface {
	Book(b Booking) error
	GetHold(eventID string, seatID string) (*Booking, error)
	ListBookings(id string) ([]Booking, error)
	ListEventBookings(eventID string) ([]Booking, error)
	Hold(b Booking) (*Booking, error)
	Release(b Booking) (*Booking, error)
}