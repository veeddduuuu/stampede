package internal

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
	if _, exists := s.booking[b.ID]; exists{
		return errors.New("booking already exists")
	}
	s.booking[b.ID] = b
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