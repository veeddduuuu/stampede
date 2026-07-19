package booking

type Service struct{
	book BookingRepository
}

func NewService(book BookingRepository) *Service{
	return &Service{
		book: book,
	}
}

func (s *Service) Book(b Booking) error{
	return s.book.Book(b)	
}


 