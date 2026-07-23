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
--- PASS: TestRedisStore_ConcurrentBookings (0.03s)
PASS
ok      concurrent-seat-booking-system/internal/booking 1.045s
```

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

## Phase 6: The API Layer
I built the HTTP API using `gorilla/mux`, fleshing out the core endpoints: `GET /healthz`, `GET /events/{id}/seats`, `POST /events/{id}/hold`, `POST /events/{id}/book`, `POST /events/{id}/release`, and `GET /users/{id}/bookings`. 
The `ListSeats` endpoint turned out quite clever. Instead of storing 100 empty seats in the database, it generates 100 virtual seats server-side on the fly, and then overlays the booking status from Postgres (for `BOOKED` seats) and Redis (for `HELD` seats). 

We also implemented graceful shutdown with SIGINT/SIGTERM handling, so the server can finish processing active requests before dying. Finally, I Dockerized everything using a multi-stage Go build. Our `docker-compose` setup is a thing of beauty: `api`, `frontend`, `postgres`, `redis`, `redis-commander`, and `migrate` services all spring to life with a single `docker compose up`. The entire system online in seconds!

## Phase 7: The Frontend — "Modiji Meetup 2026"
For the frontend, we went with React 19 and Vite 8, keeping it deliberately minimal but visually loud. 

The seat map is a 10×10 grid (rows A-J). It uses just three colors: grey for available, orange for held, and green for booked. 
The click flow is intuitive: click a grey seat, it sends a `POST hold`, and the seat turns orange with a slick TTL countdown bar. Click "Confirm", it sends a `POST book`, and the seat turns green. Click "Release", it sends a `POST release`, and it goes back to grey.

The client originally polled `GET /events/{id}/seats` every 2 seconds. A cool trick we used is that it renders the 100 default seats client-side even if the backend is temporarily offline, so the grid is always visible. 

Thematically, I went over-the-top: a saffron/Indian tricolor theme with an animated gradient title, a floating flag emoji, and a stage label for the "Modiji Meetup 2026". 

The real magic happened when we opened two browser tabs side-by-side to test real-time polling. One user holds a seat, and boom—the other tab shows it orange within 2 seconds. It felt incredibly satisfying.

## Phase 7B: The Real-Time Upgrade (WebSockets & Redis Pub/Sub)
While the 2-second HTTP polling worked, it had two fatal flaws: it generated massive unnecessary network spam when nothing was happening, and a 2-second delay in a high-concurrency ticket sale is an eternity.

We decided to migrate from short-polling to **WebSockets** for instant push updates (<5ms latency).

But this introduced a massive architectural challenge: WebSockets are *stateful* and live entirely inside the memory of the Go API server. If we horizontally scale to 5 API servers behind a load balancer, User A might connect to Server 1, and User B might connect to Server 2. When User A books a seat on Server 1, how does Server 2 know to tell User B over WebSockets?

**The Solution: Redis Pub/Sub**
We used our existing Redis instance to build a high-speed messaging bridge across all API servers.
1. When a user books a seat via a standard HTTP POST, the API saves the booking to Postgres, and then immediately publishes a JSON event payload to a Redis channel (`seat_events:<event_id>`).
2. Every single API server runs a background goroutine (`StartRedisSubscriberBridge`) that listens to this Redis channel.
3. When the Redis message arrives, the server instantly routes it to its internal WebSocket `Hub`, which broadcasts the state change to every connected browser on that specific server.

The result? Absolute real-time synchronization across an infinite number of scaled backend servers, with zero database polling.

## Phase 8: The Ghost Booking Bug
This was our crown jewel bug. I have to tell you, it drove us crazy for a while.

The bug was lurking in `hybrid_store.go`. Our `Hold()` function generated a fresh UUID (`id := uuid.New().String()`), but then we marshaled the *ORIGINAL* input `b` (which had NO ID) into Redis: 
`val, _ := json.Marshal(b)`
The brand new UUID was only ever used in the return value, not actually stored in Redis!

So Redis gleefully stored this: 
`{"ID":"", "EventID":"...", "SeatID":"...", "UserID":"..."}`

When `Book()` read the hold back from Redis, it got `held.ID = ""`. It then blindly tried to execute:
`INSERT INTO bookings (id, ...) VALUES ('', ...)`

The FIRST booking with an empty ID succeeded (because `""` is technically unique as a PRIMARY KEY). But EVERY subsequent booking attempt *ALSO* had `held.ID = ""`, hitting the `id VARCHAR(255) PRIMARY KEY` constraint — NOT the `UNIQUE(event_id, seat_id)` constraint we carefully designed for.

The error message bubbled up as "seat is already booked for this event" (because we lazily mapped ALL Postgres 23505 errors to that message), but the *ACTUAL* constraint being violated was the PRIMARY KEY collision on the empty string!

The fix was just one structural change.

Before (buggy):
```go
id := uuid.New().String()
val, _ := json.Marshal(b)  // b has no ID!
res := s.rds.SetArgs(ctx, key, val, ...)
return &Booking{ID: id, ...}, nil  // ID only in return, not in Redis
```

After (fixed):
```go
id := uuid.New().String()
held := Booking{ID: id, UserID: b.UserID, EventID: b.EventID, SeatID: b.SeatID}
val, _ := json.Marshal(held)  // held HAS the ID
res := s.rds.SetArgs(ctx, key, val, ...)
return &held, nil
```

The dramatic irony? The system was designed to prevent double bookings via atomic operations and strict unique constraints. Instead, a simple serialization oversight created GHOST bookings — invisible phantom records with empty IDs that blocked ALL future bookings. Every new booking looked like a duplicate to Postgres, but none of them were actually duplicates of each other.

We only discovered it by querying Postgres and finding a single row staring back at us with `id = ''`. The ghost in the machine.

## Phase 9: Load Testing the Hybrid Store

To truly validate our architecture, we needed to know its breaking point. I built a custom Go load testing tool (`cmd/loadtest/main.go`) to bombard the `/hold` API endpoint and measure the system's ability to survive extreme traffic.

We ran two scenarios at 5,000 requests with 500 concurrent workers:
1. **The Stampede**: 5,000 users all trying to grab the exact same seat at the exact same millisecond.
2. **General Load**: 5,000 users grabbing 5,000 different seats.

The results were phenomenal. During the stampede, the system processed all 5,000 requests in about half a second (~9,300 RPS). Most importantly, **exactly 1 request succeeded and 4,999 received a 409 Conflict**. The Redis `SETNX` lock absorbed the entire stampede in-memory, meaning Postgres never even saw the 4,999 conflicting requests! During the general load test, it handled ~9,000 RPS with 5,000 successes and an average latency under 50ms. Zero double bookings, incredible speed.

## Phase 10: Performance Engineering (Finding the Bottleneck)
We wanted to adopt a true performance engineering mindset. Instead of arbitrarily guessing how many requests per second (RPS) our system could handle, we wanted to answer one question: **"What breaks first as I increase load?"**

To do this, we updated our `cmd/loadtest/main.go` and wrapped it in a bash orchestration script to hit the `/events/{id}/hold` endpoint in massive, stepped waves (100, 250, 500, 1000, and 2000 concurrent users). While the test ran, we used `docker stats` to measure the CPU and Memory usage of the API and Redis containers.

**The "Restaurant" Analogy:**
Imagine our system is a restaurant. The **API server** represents the **Waiters** taking orders, and **Redis** represents the **Head Chef** checking the kitchen whiteboard to ensure a meal (seat) isn't sold out.

1. **The Test**: We sent increasingly large groups of customers through the door simultaneously.
2. **The Metrics**: We peaked at around ~10,000 RPS. We measured the "wait time in line" using P99 Latency (the time it took for the unluckiest 1% of customers to get served).
3. **The Breaking Point**: At 2,000 concurrent customers, the P99 wait time suddenly spiked to 620ms.
4. **The Bottleneck**: We checked the "heart rate monitors" (`docker stats`). The Head Chef (Redis) was working hard (76% CPU capacity) but keeping up. The real problem was the Waiters (API Server)! They were maxed out at ~600% CPU capacity (running as fast as 6 CPU cores would allow). The line backed up because there physically weren't enough waiters to write down the orders.

**The Solution:**
To serve more customers, we don't need a faster database just yet. We need to hire more waiters (Horizontal Scaling: adding a second API server behind a load balancer).

## Phase 11: Hiring More Waiters (Horizontal Scaling with Nginx)

Following our performance engineering discovery that our API server was the bottleneck, we put the solution into action.

We updated our `docker-compose.yml` to spin up three identical Go API containers: `api1`, `api2`, and `api3`. To manage the traffic across them, we introduced **Nginx** as a reverse proxy. 

**The Load Balancing Strategy:**
Instead of using a naive round-robin approach, we configured the Nginx `upstream` block with the `least_conn` directive. This was a crucial architectural decision. Because some requests (like a cache hit) are blazing fast, while others (like a Postgres write) take a bit longer, round-robin could accidentally pile up slow requests on a single server. `least_conn` acts like a smart host at a restaurant—it looks at all three waiters (`api1`, `api2`, `api3`) and assigns the next customer to whoever is currently juggling the fewest tables.

When we tailed the logs across all three containers and fired a rapid burst of `curl` requests at Nginx, it was incredibly satisfying to watch the traffic get perfectly distributed across the cluster. The restaurant had successfully expanded!

## Phase 12: Idempotency (Pending)
*(To be updated when we handle network retries and idempotency keys)*
