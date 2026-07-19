package booking

import (
	"errors"
	"sync"
)

type ConcurrentInmemRepository struct{
	booking map[string]Booking
	sync RWMutex
}

func NewConcurrentInmemRepository() *ConcurrentInmemRepository{
	return &ConcurrentInmemRepository{
		booking: make(map[string]Booking),
	}
}

func (s *ConcurrentInmemRepository) Book(b Booking) error{
	s.Lock()
	defer s.Unlock()
	seatKey := b.EventID+"_"+b.SeatID
	if _, exists := s.booking[seatKey]; exists{
		return errors.New("seat already booked")
	}
	s.booking[seatKey] = b
	return nil
}

func (s *ConcurrentInmemRepository) ListBookings(id string) ([]Booking){
	s.RLock()
	defer s.RUnlock()
	var result []Booking
	for _, b:= range s.booking{
		if b.UserID == id{
			result = append(result, b)
		}
	}
	return result
}