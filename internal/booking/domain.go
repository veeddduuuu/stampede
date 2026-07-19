package booking

type Booking struct {
	ID      string
	EventID string
	SeatID  string
	UserID  string
	Status  string
}

type BookingRepository interface{
	Book(b Booking) error
	ListBookings(id string) ([]Booking)
}