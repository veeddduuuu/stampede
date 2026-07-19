package booking

import (
	"sync"
	"testing"
)

func TestInmemRepository_ConcurrentBookings(t *testing.T) {
	repo := NewInmemRepository()
	
	var wg sync.WaitGroup
	// Launch 100 goroutines to concurrently book seats
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			
			// Some might try to book the same ID, some different ones
			// Let's use the same EventID and SeatID to simulate concurrency on the same resource
			b := Booking{
				ID:      "booking-1", // They all try to book the same booking ID or different ones
				EventID: "event-1",
				SeatID:  "seat-1",
				UserID:  "user-1",
				Status:  "booked",
			}
			
			// Even if they use different IDs, concurrent map writes will crash the program.
			_ = repo.Book(b)
		}(i)
	}
	
	wg.Wait()
}
