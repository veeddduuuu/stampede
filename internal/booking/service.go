package booking

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const defaultHoldTTL = 3 * time.Minute

type Service struct {
	book BookingRepository
}

func NewService(book BookingRepository) *Service {
	return &Service{
		book: book,
	}
}

func (s *Service) Book(b Booking) error {
	held, err := s.book.GetHold(b.EventID, b.SeatID)
	if err != nil {
		return err
	}

	if held.UserID != b.UserID {
		return ErrUnauthorizedHold
	}

	return s.book.Book(*held)
}

func (s *Service) Hold(b Booking) (*Booking, error) {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	b.Status = "HELD"
	if b.ExpiresAt.IsZero() {
		b.ExpiresAt = time.Now().Add(defaultHoldTTL)
	}

	return s.book.Hold(b)
}

func (s *Service) ListBookings(userID string) ([]Booking, error) {
	return s.book.ListBookings(userID)
}

func (s *Service) Release(b Booking) (*Booking, error) {
	held, err := s.book.GetHold(b.EventID, b.SeatID)
	if err != nil {
		return nil, ErrSeatNotHeld
	}

	if held.UserID != b.UserID {
		return nil, ErrUnauthorizedHold
	}

	return s.book.Release(*held)
}

func (s *Service) ListSeats(eventID string) ([]Seat, error) {
	// Generate the default 100 seats
	seats := make([]Seat, 100)
	for i := 0; i < 100; i++ {
		seats[i] = Seat{
			ID:     fmt.Sprintf("%d", i+1),
			Status: "AVAILABLE",
		}
	}

	bookings, err := s.book.ListEventBookings(eventID)
	if err != nil {
		return nil, err
	}

	// Map bookings by seat ID for quick lookup
	bookingMap := make(map[string]Booking)
	for _, b := range bookings {
		// If there's a conflict between HELD and BOOKED (e.g., both present somehow),
		// we should prioritize BOOKED over HELD.
		if existing, exists := bookingMap[b.SeatID]; exists {
			if existing.Status == "BOOKED" && b.Status == "HELD" {
				continue // Keep the BOOKED status
			}
		}
		bookingMap[b.SeatID] = b
	}

	for i := range seats {
		if b, exists := bookingMap[seats[i].ID]; exists {
			seats[i].Status = b.Status
		}
	}

	return seats, nil
}