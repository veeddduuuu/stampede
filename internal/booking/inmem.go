package booking

import (
	"errors"
)

type InmemRepository struct{
	booking map[string]Booking
}

func NewInmemRepository() *InmemRepository{
	return &InmemRepository{
		booking: make(map[string]Booking),
	}
}

func (s *InmemRepository) Book(b Booking) error{
	seatKey := b.EventID + "_" + b.SeatID
	if _, exists := s.booking[seatKey]; exists{
		return errors.New("seat is already booked for this event")
	}
	s.booking[seatKey] = b
	return nil
}

func (s *InmemRepository) ListBookings(id string) ([]Booking){
	var result []Booking
	for _, b:= range s.booking{
		if b.UserID == id{
			result = append(result, b)
		}
	}
	return result
}