package booking

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func setupDatabases(t *testing.T) (*pgxpool.Pool, *redis.Client, func()) {
	ctx := context.Background()

	// Connect to Postgres
	pgConnString := "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	pool, err := pgxpool.New(ctx, pgConnString)
	if err != nil {
		t.Fatalf("Failed to connect to Postgres: %v", err)
	}

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Clean up tables/keys before tests
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE bookings;"); err != nil {
		t.Fatalf("Failed to truncate bookings: %v", err)
	}
	rdb.FlushDB(ctx)

	cleanup := func() {
		pool.Exec(ctx, "TRUNCATE TABLE bookings;")
		rdb.FlushDB(ctx)
		pool.Close()
		rdb.Close()
	}

	return pool, rdb, cleanup
}

func TestLatencyComparison(t *testing.T) {
	pool, rdb, cleanup := setupDatabases(t)
	defer cleanup()

	pgStore := NewPostgresStore(pool)
	hybridStore := NewHybridStore(pool, rdb)

	const iterations = 50

	// Benchmark Postgres Alone
	startPg := time.Now()
	for i := 0; i < iterations; i++ {
		b := Booking{
			ID:      fmt.Sprintf("pg-id-%d", i),
			EventID: "event-latency-test",
			SeatID:  fmt.Sprintf("pg-seat-%d", i),
			UserID:  "user-pg",
			Status:  "BOOKED",
		}
		err := pgStore.Book(b)
		if err != nil {
			t.Fatalf("Postgres failed to book: %v", err)
		}
	}
	durationPg := time.Since(startPg)
	avgPg := durationPg / iterations

	// Benchmark Hybrid Store
	startHybrid := time.Now()
	for i := 0; i < iterations; i++ {
		b := Booking{
			EventID: "event-latency-test",
			SeatID:  fmt.Sprintf("hybrid-seat-%d", i),
			UserID:  "user-hybrid",
			Status:  "BOOKED",
		}
		err := hybridStore.Book(b)
		if err != nil {
			t.Fatalf("Hybrid Store failed to book: %v", err)
		}
	}
	durationHybrid := time.Since(startHybrid)
	avgHybrid := durationHybrid / iterations

	fmt.Printf("\n--- LATENCY BENCHMARK (%d sequential inserts) ---\n", iterations)
	fmt.Printf("Postgres Store Average Latency: %v\n", avgPg)
	fmt.Printf("Hybrid Store Average Latency:   %v\n", avgHybrid)
	fmt.Println("--------------------------------------------------")
}

func TestHybridStoreConcurrency(t *testing.T) {
	pool, rdb, cleanup := setupDatabases(t)
	defer cleanup()

	hybridStore := NewHybridStore(pool, rdb)

	var wg sync.WaitGroup
	var successes int32
	var failures int32
	var redisRejections int32
	var postgresRejections int32

	const numGoroutines = 100
	seatID := "hybrid-concurrent-seat-1"
	eventID := "event-concurrent"

	fmt.Printf("\n--- CONCURRENCY TEST (%d goroutines) ---\n", numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			b := Booking{
				EventID: eventID,
				SeatID:  seatID,
				UserID:  fmt.Sprintf("user-%d", userID),
				Status:  "BOOKED",
			}

			err := hybridStore.Book(b)
			if err == nil {
				atomic.AddInt32(&successes, 1)
			} else {
				atomic.AddInt32(&failures, 1)
				// Check where it was rejected
				if err.Error() == "unable to hold seat the seat is already booked seat already booked" || err.Error() == "seat already booked" {
					atomic.AddInt32(&redisRejections, 1)
				} else {
					atomic.AddInt32(&postgresRejections, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	fmt.Printf("Total Successes: %d\n", successes)
	fmt.Printf("Total Failures: %d\n", failures)
	fmt.Printf("Rejected by Redis: %d\n", redisRejections)
	fmt.Printf("Rejected by Postgres: %d\n", postgresRejections)
	fmt.Println("----------------------------------------")

	if successes != 1 {
		t.Errorf("Expected exactly 1 successful booking, got %d", successes)
	}
	if failures != numGoroutines-1 {
		t.Errorf("Expected exactly %d failures, got %d", numGoroutines-1, failures)
	}
}
