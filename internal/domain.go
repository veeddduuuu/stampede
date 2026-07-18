package internal

type Booking struct {
	ID      string
	MovieID string
	SeatID  string
	UserID  string
	Status  string
}

type BookingRepository interface{
	Book(b Booking) error
	ListBookings(id string) ([]Booking)
}