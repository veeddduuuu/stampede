# My Implementation Journey: Building a Concurrent Seat Booking System

This document is a living record of my journey building the Concurrent Seat Booking System. It's a story about how the architecture evolved, the walls I hit, how I tested my assumptions, and the solutions I ultimately pieced together.

I'll keep updating this story as I add new features or migrate to new storage layers.

---

## Chapter 1: The Naive In-Memory Solution

When I first started, I wanted to get something up and running quickly. I built a very simple, naive in-memory repository (`InmemRepository`). I used a standard Go map (`map[string]Booking`) to store all my booking records. My logic was straightforward: when a user attempted to book a seat, I checked if the `Booking.ID` existed in the map. If it didn't, I added it. Easy, right?

To see if this naive approach could handle real-world traffic—where multiple users try to book seats at the exact same millisecond—I wrote a stress test. In `inmem_test.go`, I launched 100 concurrent goroutines using a `sync.WaitGroup`. Each goroutine simultaneously called the `Book()` method on the exact same repository instance. 

I ran the test using Go's built-in race detector (`go test -v -race ./internal/booking`), and it immediately blew up in my face. 

The test threw a `WARNING: DATA RACE` error. If this were a production environment without the race detector, it would have crashed the whole server with a `fatal error: concurrent map writes`. I quickly learned that Go maps are intentionally *not* thread-safe. When one goroutine tries to read or write to a map at the exact same time another goroutine is writing to it, the memory becomes corrupted.

Here is the exact crash report from the race detector:
```text
=== RUN   TestInmemStore_ConcurrentBookings
==================
WARNING: DATA RACE
Read at 0x00c00012f0b0 by goroutine 79:
  runtime.mapaccess2_faststr()
      /usr/local/go/src/internal/runtime/maps/runtime_faststr.go:161 +0x0
  concurrent-seat-booking-system/internal/booking.(*InmemRepository).Book()

Previous write at 0x00c00012f0b0 by goroutine 10:
  runtime.mapassign_faststr()
      /usr/local/go/src/internal/runtime/maps/runtime_faststr.go:263 +0x0
  concurrent-seat-booking-system/internal/booking.(*InmemRepository).Book()

==================
    testing.go:1712: race detected during execution of test
--- FAIL: TestInmemStore_ConcurrentBookings (0.00s)
FAIL
```

---

## Chapter 2: The Mutex Band-Aid (Resolving Local Concurrency)

I needed to stop the application from crashing every time two people clicked "Book" at the same time. To fix this, I introduced a `sync.RWMutex` (a Reader/Writer Mutual Exclusion lock). 

The concept was simple: before accessing or modifying the map, my application had to acquire a lock (`s.Lock()`). 
If Goroutine A had the lock, and Goroutine B wanted it, Goroutine B was forced to wait in line. Once Goroutine A finished its work and called `s.Unlock()`, Goroutine B got the lock and could proceed. 

This effectively serialized access to the map. The data race memory corruption was completely eliminated. The server was stable!

Here is the proof from the test run showing that the exact same stampede strategy now passes instantly with zero data races:
```text
=== RUN   TestConcurrentInmemStore_ConcurrentBookings
--- PASS: TestConcurrentInmemStore_ConcurrentBookings (0.00s)
PASS
ok      concurrent-seat-booking-system/internal/booking 1.015s
```

---

## Chapter 3: The Double Booking Disaster (Fixing Business Logic)

Even though the server was no longer crashing, I soon realized I had a massive flaw in my business logic. 

My `Book` method was checking if the generated `Booking.ID` already existed in the map. But because every single booking attempt generates a *unique* `Booking.ID` (like a UUID), two different users could successfully book the exact same `EventID` and `SeatID`! They both had unique booking IDs, so the map gladly accepted both.

To fix this, I completely changed how I tracked availability. I started using a composite key instead of the Booking ID. I created a `seatKey` using the format `EventID_SeatID` (for example, `event-1_seat-1`). Now, if my repository sees that `event-1_seat-1` is already sitting in the map, it immediately rejects the booking, correctly preventing any double bookings.

---

## Chapter 4: The Great Migration (Persistent Storage with Redis)

While my in-memory map and Mutex worked great for a single server, I knew it wouldn't scale. If the server restarted, all bookings were lost. And if I added a second server behind a load balancer, a local Go `sync.Mutex` on Server A couldn't stop Server B from double-booking a seat.

I decided it was time to move to Redis for persistent, distributed storage. I wrote a `RedisStore` that connects to a local Redis container (which I spun up using Docker Compose, after a brief battle with broken registry images and environment variables!).

But how do you handle concurrency in Redis without a Go Mutex? I turned to Redis's atomic operations. I used the `SETNX` (Set if Not Exists) command. When multiple requests hit Redis at the exact same time, Redis natively guarantees that only one request successfully sets the key, while all others receive a failure response.

**The Ultimate Stampede Test:**
I wanted to prove this `SETNX` logic could survive a true concurrent stampede. 
1. I went back to `inmem_test.go` and spawned 100 goroutines.
2. But this time, instead of letting them run loosely, I made all 100 goroutines block on a single `start := make(chan struct{})` channel.
3. Once all 100 were lined up at the starting gate, my main thread called `close(start)`. This broadcasted a signal that unblocked all 100 goroutines at the *exact same millisecond*.
4. I ran the test with the race detector (`go test -race`).

The result? It passed beautifully! Redis accepted exactly one booking and atomically rejected the other 99 simultaneous requests. This proved my distributed locking mechanism worked flawlessly under extreme pressure, completely eliminating the need for local Mutexes.

Here is the actual proof from the test run, showing exactly one successful booking out of the 100 requests:
```text
=== RUN   TestRedisStore_ConcurrentBookings
2026/07/20 03:34:22 Connected to Redis: localhost:6379
2026/07/20 03:34:22 Booking held for user user-81: &{ID:74729599-97b2-4cb7-a018-a93cd9464217 EventID:event-1 SeatID:seat-1 UserID:user-81 Status:HELD ExpiresAt:2026-07-20 03:34:23.788297743 +0530 IST m=+1.207095369}
## Phase 4: Persistent Storage (PostgreSQL & Redis)

**What we built:**
We implemented two independent distributed storage layers to move state out of Go's volatile memory:
1. **RedisStore:** Uses a Redis client to hold bookings, suitable for extremely fast, concurrent caching and locking.
2. **PostgresStore:** Uses `pgxpool` with a minimum of 10 and maximum of 50 connections to handle relational ACID transactions. We also introduced a Docker Compose `migrate` service to auto-create the `bookings` table on startup.

**How we solved Business Concurrency here:**
Instead of relying on application-level Mutexes (which don't work across multiple servers), we pushed the concurrency checks to the database layer. 
For PostgreSQL, we created a `UNIQUE(event_id, seat_id)` constraint in the schema. When 100 concurrent goroutines attempt to book the exact same seat, Postgres accepts the first `INSERT` and strictly rejects the other 99 with a `23505 Unique Violation` error.

## Phase 5: The Hybrid Store (Redis + Postgres)

While the independent Postgres and Redis stores worked, they each had flaws when standing alone:
1. **Pure Postgres** can suffer under extreme concurrency (like a Taylor Swift ticket sale). 10,000 users clicking "Book" at once would result in 10,000 database transactions hitting Postgres simultaneously, fighting for row locks and burning up CPU, even if 9,999 of them are eventually rejected by the `UNIQUE` constraint.
2. **Pure Redis** is fast, but it's fundamentally an in-memory datastore. It is not designed to be the single source of truth for permanent, relational, financial data like ticket bookings.

**What we built:**
We combined them into a `HybridStore`. When a user attempts to book a seat:
1. The `HybridStore` attempts to acquire a short-lived lock (3 minutes) in Redis.
2. If Redis rejects it, the request is killed instantly. Postgres is completely protected.
3. If Redis grants the lock, the `HybridStore` opens a Postgres transaction to permanently save the booking.
4. If the Postgres transaction fails, we delete the Redis lock to free the seat.

**The Test Results:**
We wrote a benchmark to compare them:
- **Sequential Latency:** The pure Postgres store was slightly faster (~1.25ms vs ~1.60ms) because it only requires one network hop instead of two.
- **High Concurrency (100 concurrent requests):** This is where the Hybrid Store shines. Out of 100 simultaneous requests for the exact same seat, Redis instantly blocked 97 of them. Only 3 requests made it through to Postgres, and Postgres safely rejected the remaining 2 duplicates. The database was almost completely shielded from the stampede!

## Phase 6: The API & Infrastructure Layer (Pending)
*(To be updated when we build the HTTP routers, Health checks, and Graceful Shutdown)*

## Phase 7: Idempotency (Pending)
*(To be updated when we handle network retries and idempotency keys)*
