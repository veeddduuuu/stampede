package booking

import (
	"fmt"
	"sync"
	"testing"
	"concurrent-seat-booking-system/internal/adapters/redis"
)

func TestInmemStore_ConcurrentBookings(t *testing.T) {
	repo := NewInmemRepository()

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			
			<-start

			b := Booking{
				ID:      fmt.Sprintf("booking-%d", i),
				EventID: "event-2",
				SeatID:  "seat-2",
				UserID:  fmt.Sprintf("user-%d", i),
				Status:  "booked",
			}
			
			_ = repo.Book(b)
		}(i)
	}
	
	close(start)
	wg.Wait()
}

func TestConcurrentInmemStore_ConcurrentBookings(t *testing.T) {
	repo := NewConcurrentInmemRepository()

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			
			<-start

			b := Booking{
				ID:      fmt.Sprintf("booking-%d", i),
				EventID: "event-3",
				SeatID:  "seat-3",
				UserID:  fmt.Sprintf("user-%d", i),
				Status:  "booked",
			}
			
			_ = repo.Book(b)
		}(i)
	}
	
	close(start)
	wg.Wait()
}

func TestRedisStore_ConcurrentBookings(t *testing.T) {
	repo := NewRedisStore(redis.NewRedisClient("localhost:6379"))
	defer repo.rbd.Close()

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			
			<-start

			b := Booking{
				ID:      fmt.Sprintf("booking-%d", i),
				EventID: "event-1",
				SeatID:  "seat-1",
				UserID:  fmt.Sprintf("user-%d", i),
				Status:  "booked",
			}
			
			_ = repo.Book(b)
		}(i)
	}
	
	close(start)
	wg.Wait()
}
