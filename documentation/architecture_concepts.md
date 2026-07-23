# Architectural Concepts & Learnings

This document serves as a knowledge base for the architectural concepts, questions, and answers discussed during the development of the Concurrent Seat Booking System. It is designed for both human review and agent context.

## 1. Local vs. Distributed Concurrency

### Local Concurrency (`sync.Mutex` / `sync.RWMutex`)
* **Scope:** Exists only within the memory of a single running Go process.
* **Limitation:** If the application scales to multiple instances (e.g., behind a load balancer), a Mutex on Instance A cannot coordinate with a Mutex on Instance B.
* **Use Case:** Preventing data races on in-memory maps or coordinating goroutines within the same server.

### Distributed Concurrency (Redis / Postgres)
* **Scope:** Coordinated through a centralized external storage layer accessible by all application instances.
* **Solution 1 (Distributed Locks):** Using algorithms like **Redlock** (Redis) to ensure only one instance across the entire cluster can acquire a lock for a specific resource (like a seat).
* **Solution 2 (Atomic Operations):** Leveraging atomic database commands like `SETNX` (Set if Not Exists) in Redis, or `UNIQUE` constraints in PostgreSQL, to ensure checks and writes happen in a single, unbreakable step.

## 2. The Hybrid Architecture: Redis + PostgreSQL

In a large-scale production system, Redis and PostgreSQL are rarely used in isolation. They are combined to leverage their respective strengths.

### Caching (Speed)
* Postgres reads from disk and can be slow under heavy load. 
* Redis sits in front of Postgres to cache read-heavy data (e.g., "Available Seat Counts"). Users read from Redis, and only the final "Buy" transaction is sent to Postgres.

### Distributed Lock Manager (Concurrency)
* Redis is extremely fast at processing single-threaded commands. 
* Before booking, the application asks Redis for a lock on the seat. If granted, the application opens a Postgres transaction to permanently save the booking. This prevents Postgres from being overwhelmed by conflicting transactions.

### Asynchronous Processing (Queueing)
* During extreme traffic spikes (e.g., Taylor Swift tickets), synchronously talking to Postgres will cause the database to crash.
* The API accepts the booking request and instantly pushes a "Booking Job" to a Redis Queue (or Kafka).
* Background workers pull jobs from the queue one by one and carefully insert them into Postgres at a safe speed.

## 3. Idempotency

**Definition:** Ensuring that making the same request multiple times has the exact same effect as making it once.
**Why it matters:** In distributed systems, network drops happen. If a user clicks "Book" and their internet drops, their browser might retry the request. The system must recognize this is the *same* request (using an Idempotency Key) and safely return the existing successful result without crashing, throwing an error, or double-charging the user.

## 4. Relational Database (PostgreSQL) Concepts

When moving the persistent store to PostgreSQL, several new database-level concurrency and safety mechanisms come into play.

### Connection Pooling
* **What it is:** Instead of opening a brand new network connection to Postgres for every single user request (which is slow and can crash the database), Go maintains a "pool" of open, reusable connections in the background.
* **How Go handles it:** The `database/sql` standard library handles pooling automatically. When you call `db.Query()`, Go borrows a connection from the pool. When the query finishes, Go puts it back.
* **Key Configurations:** You must configure `db.SetMaxOpenConns()` (the maximum allowed connections to the database) and `db.SetMaxIdleConns()` (the number of connections to keep open while waiting for traffic) to prevent overwhelming Postgres.

### ACID Transactions
* **Definition:** A set of properties (Atomicity, Consistency, Isolation, Durability) that guarantee database transactions are processed reliably.
* **Why it matters:** If you need to deduct money from a wallet and book a seat, both must succeed. If the wallet deduction succeeds but the seat booking fails, you must **Rollback** the entire transaction so the user gets their money back. In Go, this is done by starting a transaction (`tx, err := db.Begin()`), running queries against `tx`, and calling `tx.Commit()` on success or `tx.Rollback()` on failure.

### Pessimistic Concurrency (Row-Level Locking)
* **What it is:** Locking a specific row in the database so no other transaction can read or write to it until the current transaction finishes.
* **How it works:** You use the SQL command `SELECT * FROM seats WHERE id = 1 FOR UPDATE;`. If Transaction A runs this, and Transaction B attempts to run the same query, Transaction B is forced to wait until Transaction A calls `COMMIT` or `ROLLBACK`. This acts just like a `sync.Mutex`, but at the database level.

### Unique Constraints
* **What it is:** A strict rule defined in the database schema that guarantees the values in a column (or a combination of columns) are absolutely unique across the entire table.
* **How it protects us:** By defining a `UNIQUE(event_id, seat_id)` constraint on the bookings table, the database acts as the ultimate source of truth. Even if two transactions manage to bypass your application locks and attempt to double-book the same seat simultaneously, Postgres will accept the first `INSERT` and reject the second one with a `Unique Violation` error.

## 5. Lock Contention & Critical Section Size

A major takeaway from testing concurrency (like running 100k goroutines with a mutex) is the impact of **Lock Contention**.

* **The Trap:** It is easy to wrap a huge block of code (e.g., an entire API request handling, logging, and database call) inside a `mu.Lock()` and `mu.Unlock()`. While this prevents data races, it destroys performance. It forces every concurrent request to wait in a single-file line, turning your multi-threaded application into a slow, synchronous one.
* **The Solution:** The code between a lock and an unlock is called the **Critical Section**. To maintain high performance, the critical section must be the *bare minimum* required to protect the shared resource. 
* **Database Equivalent:** This exact principle applies to PostgreSQL row-level locks (`SELECT FOR UPDATE`). If you lock a row, you must complete your transaction (`COMMIT`) as fast as physically possible. If you lock a row and then make a slow 3-second API call to Stripe for a payment before unlocking, you will create massive bottlenecks and crash your database throughput.

## 6. How Postgres Solved the Double-Booking (ACID & Constraints)

When 100 goroutines fired at the Postgres database simultaneously, here is exactly how Postgres handled it using the `UNIQUE` constraint and ACID properties:

### How UNIQUE helped
When we defined `UNIQUE(event_id, seat_id)`, Postgres created a specialized index (B-Tree) under the hood. When the 100 requests hit Postgres at the exact same millisecond, Postgres' internal engine serializes them (forces them into a line). 
The first request creates the row. When the other 99 requests try to insert their rows, Postgres checks the index, sees that `event-1` + `seat-1` already exists, and instantly rejects the transaction with a `Unique Violation`. This completely bypassed the need for Go `sync.Mutex` locks.

### ACID Properties in Context
* **Atomicity (All or Nothing):** A transaction is atomic. If our `Book()` function had two queries (1. Insert booking, 2. Deduct $50 from wallet), Atomicity guarantees that if the wallet deduction fails, the booking insert is instantly reversed. You never end up in a half-finished state.
* **Consistency (Following the Rules):** The database strictly enforces our schema rules. Because we set a `UNIQUE` constraint, the database guarantees it will never be put into an invalid, double-booked state.
* **Isolation (Invisibility):** While Transaction A is processing a booking, Transaction B cannot see Transaction A's half-finished work. They operate entirely isolated from each other until `COMMIT` is called.
* **Durability (Permanent Storage):** Once `tx.Commit()` succeeds, Postgres writes the booking to the physical hard drive. If someone pulls the power cord on the server 1 second later, the booking is perfectly safe and will be there when the server turns back on.

### When does the system Rollback?
A `ROLLBACK` is an "undo" button. 
In our Go code, we wrote `defer tx.Rollback(ctx)`. If *anything* goes wrong before we reach the end of the function (e.g., the Unique Constraint is violated, the network drops, or the code panics), the deferred function runs, completely wiping out any partial database changes made by that transaction. If we successfully call `tx.Commit()`, the deferred rollback becomes a harmless no-op.

### Why do we drop tables in Migrations?
We wrote `DROP TABLE IF EXISTS bookings;` inside the `.down.sql` file. 
Migrations work in pairs: `UP` (apply changes) and `DOWN` (undo changes). When the app boots, it only runs the `UP` script to create the table. We only run the `DOWN` script (which drops the table) if we made a massive mistake and need to reverse the migration to clean the database back to its previous state.

### If UNIQUE handles concurrency perfectly, why do we need Transactions?
It is true that a single `INSERT` query is inherently atomic. If our entire application *only* inserted one row and did absolutely nothing else, we wouldn't strictly need to wrap it in an explicit `db.Begin()` transaction, because the `UNIQUE` constraint handles the concurrency clash perfectly for that single insert.

However, in the real world, a booking is almost never a single query. A real booking flow looks like:
1. `INSERT INTO bookings...` (This is protected by the `UNIQUE` constraint).
2. `UPDATE users SET wallet = wallet - 50...` (Deduct the money).
3. `INSERT INTO payment_history...` (Log the receipt).

If we didn't use a Transaction, and the server crashed exactly between Step 1 and Step 2, the user would get the seat but keep their money! 
The `UNIQUE` constraint only protects the data integrity of a **single table**. We use **Transactions** to protect the integrity of the **entire business flow** across multiple tables, guaranteeing that if Step 2 fails, Step 1 is completely rolled back.

## 7. Frontend Architecture (Seat Map UI)

### Technology Stack
* **Framework:** React 19 + Vite 8 (minimal, fast dev server)
* **Styling:** Vanilla CSS with glassmorphism, saffron-themed Indian aesthetic
* **State Management:** React `useState` + `useEffect` hooks (no external state library)

### Seat Map Rendering
* The backend `ListSeats` endpoint generates 100 seats (`1`–`100`) and overlays their status from the bookings table.
* The frontend renders these as a 10×10 grid (rows A–J, columns 1–10).
* Each seat has 3 visual states: **Available** (grey), **Held** (pulsing orange — includes the current user's selection), and **Booked** (green ✓).

### User Flow (Hold → Confirm/Release)
1. **Click Available Seat → POST `/events/{id}/hold`**: The backend atomically holds the seat (Redis `SETNX` + TTL), returning an `expires_at` timestamp.
2. **Frontend starts a TTL countdown**: A progress bar drains from orange → red. If it hits zero, the hold has expired server-side.
3. **Click Confirm → POST `/events/{id}/book`**: Converts the hold into a permanent booking in Postgres.
4. **Click Release → POST `/events/{id}/release`**: Manually releases the hold before TTL expires.

### Polling Strategy (Pre-WebSocket)
* The frontend polls `GET /events/{id}/seats` every **2 seconds** to keep the seat map fresh.
* During polling, the frontend detects if a held seat transitions to `AVAILABLE` (TTL expired server-side) or `BOOKED` (confirmed by another user) and clears the local selection state accordingly.
* **Future:** This will be replaced with WebSocket push notifications for real-time updates without polling overhead.

### Vite Proxy
The Vite dev server proxies `/events/*`, `/users/*`, and `/healthz` to `http://localhost:8080` (the Go API server), allowing the frontend and backend to run on different ports during development without CORS issues.

## 8. The Ghost Booking Bug (Data Serialization Consistency)

A critical learning about ensuring data consistency across a two-phase distributed operation (Redis hold → Postgres commit).

### The Bug
The `Hold()` function generated a UUID for the booking but serialized the **input** struct `b` (which had no ID) into Redis, rather than the **output** struct that included the generated UUID. When `Book()` later read the hold from Redis and deserialized it, the `ID` field was an empty string `""`.

### The Cascade
1. The first `Book()` call succeeded — Postgres happily accepted a row with `id = ''` (an empty string is still a valid, unique primary key value).
2. Every subsequent `Book()` call also tried to INSERT with `id = ''`, hitting the `PRIMARY KEY` constraint — not the `UNIQUE(event_id, seat_id)` constraint we designed.
3. The error code was `23505` in both cases, so our error handler mapped it to "seat is already booked for this event" — a misleading message that sent us looking in the wrong direction.

### The Lesson
When a system has a multi-phase operation where Phase 1 stores intermediate state and Phase 2 consumes it:
* **Always serialize the complete, enriched struct** — not the raw input. If you generate state (like a UUID, timestamp, or computed field) during Phase 1, it must be persisted in the intermediate store.
* **Distinguish your error sources.** If a single database error code (`23505`) can be triggered by multiple constraints (`PRIMARY KEY` vs `UNIQUE`), your error handling should inspect *which* constraint was violated, not just the error code.
* **Ghost data is invisible by design.** The empty-string ID appeared valid to every individual component — Redis stored it, Postgres accepted it (once), and the API returned a success. The failure only manifested on the *second* operation, making it look like a legitimate duplicate rather than a serialization bug.

## 9. Separation of Concerns: Service vs. Store (Clean / Hexagonal Architecture)

### The Problem: Leaky Abstractions & Misplaced Logic
In early implementations or rapid prototypes, repository/store implementations (like `HybridStore`) often end up containing business logic:
* Validating user ownership (`held.UserID != b.UserID`)
* Creating business identifiers (`uuid.New()`) and hold TTL duration rules (`now.Add(defaultHoldTTL)`)
* Mapping database-specific errors (Postgres `23505`) to business error strings

Meanwhile, the `Service` becomes a pass-through wrapper for repository methods.

### The Refactored Design Standard

1. **Domain & Service Layer (`internal/booking/`)**:
   * **Responsibility:** Business rules, workflows, validation, and domain invariants.
   * **Contains:**
     * Domain models (`Booking`, `Seat`).
     * Domain errors (`ErrSeatAlreadyBooked`, `ErrHoldExpired`, `ErrUnauthorizedHold`).
     * Business validation (verifying user ownership of a hold, hold TTL policies).
     * High-level seat visualization logic (`ListSeats`).

2. **Store / Repository Layer (`internal/adapters/storage/` or `internal/booking/`):**
   * **Responsibility:** Pure data persistence and retrieval mechanism.
   * **Contains:**
     * Redis atomic key-value operations (`SETNX`, `GET`, `DEL`).
     * PostgreSQL SQL queries and transaction management (`tx.Begin()`, `tx.Exec()`, `tx.Commit()`).
     * Converting raw DB/Redis rows into domain models.

## 10. HTTP Routing Migration: Gorilla Mux to Chi Router

### The Issues During Migration
When migrating an HTTP API from `gorilla/mux` to `github.com/go-chi/chi/v5`:
1. **URL Parameter Extraction Discrepancy:** `gorilla/mux` stores URL parameters in request context accessible via `mux.Vars(r)["id"]`. `chi` uses `chi.URLParam(r, "id")`. If routes are defined with `chi.NewRouter()` but handlers still call `mux.Vars(r)`, `mux.Vars(r)` returns an empty map, leading to empty IDs in all request handlers.
2. **Middleware Ecosystem:** `chi` provides lightweight built-in middlewares (`middleware.Logger`, `middleware.Recoverer`) that integrate smoothly with standard `net/http` handlers.

## 11. Low-Compute Edge & Cloud Topology (Vercel + NeonDB + EC2)

### Deployment Architecture Strategy
Rather than running full database instances (Postgres) and static web servers on a single EC2 instance (which consumes heavy CPU/RAM/IOPS), the application adopts a decoupled hybrid architecture:

1. **Frontend (Vercel / Edge CDN):**
   * Static UI asset delivery and React client rendering offloaded to Vercel.
   * Eliminates EC2 web-serving load, provides global CDN caching, and scales automatically.

2. **Database Layer (NeonDB Serverless PostgreSQL):**
   * Persistent relational storage offloaded to NeonDB.
   * Removes PostgreSQL memory footprint, background autovacuum, and disk IOPS from the EC2 instance.

3. **Core API & In-Memory State (Go + Redis on EC2):**
   * The EC2 instance hosts only the lightweight compiled Go binary (consuming ~15-30MB RAM) and high-speed Redis for atomic seat holds (`SETNX`).
   * Optimizes EC2 resource usage so the single node can handle thousands of concurrent requests per second with minimal CPU and RAM overhead.

## 12. Real-Time Seat State Synchronization (WebSockets + Redis Pub/Sub)

### Polling vs. WebSockets Architecture
* **HTTP Short Polling:** Frontend polls `GET /events/:id/seats` every N seconds. Incur high latency (up to N seconds delay), high HTTP header overhead, and redundant database queries when no seat state has changed.
* **WebSockets (Full-Duplex TCP):** Persistent socket connection established via HTTP 101 Upgrade. Allows backend to instantly push state changes (e.g. `SEAT_HELD`, `SEAT_BOOKED`, `SEAT_RELEASED`) to connected clients with <5ms latency.

### Multi-Instance Horizontal Scaling via Redis Pub/Sub
* WebSockets are inherently stateful (connected to specific Go application memory).
* In a horizontally scaled cluster (e.g. Instance A & Instance B behind a Load Balancer), a booking processed on Instance A must be communicated to clients connected to Instance B.
* **Redis Pub/Sub Event Fanout:** When Instance A updates a seat, it publishes an event to Redis channel `seat_events:<event_id>`. All instances subscribe to this channel, receive the event message, and push it to their local WebSocket clients.

### Hybrid Initial Snapshot + Stream Delta Pattern
* To ensure consistent state on load, clients perform a standard HTTP `GET /events/:id/seats` to fetch the complete current state snapshot, then open a WebSocket connection to stream live real-time updates.

### Go WebSocket Hub & Dual-Goroutine Pump Architecture
* **Channel-Driven Event Loop (`Hub.Run()`):** Manages `clients map[*Client]bool` using channels (`register`, `unregister`, `broadcast`) inside a single `select` loop goroutine. Avoids mutex lock contention and eliminates data races without blocking on network I/O.
* **Dual Goroutine Client Lifecycle (`readPump` & `writePump`):** 
  * `readPump`: Dedicated reader goroutine per client socket. Handles inbound messages/Pings, updates deadlines, and triggers unregistration on EOF/error.
  * `writePump`: Dedicated writer goroutine per client socket. Consumes from client's buffered `send` channel and flushes frames to the TCP socket under write deadlines.
* **Slow Receiver Demolition:** If a laggy client's `send` channel buffer fills up, `Hub.Run()` drops and unregisters the client immediately via a non-blocking `select`, preventing one slow client from bottlenecking broadcasts to other connected users.

### Redis Pub/Sub Subscriber Bridge Pattern
* **Decoupled Out-of-Band Fanout:** HTTP Handlers/Services only publish events to Redis using `Publish(ctx, "seat_events:"+id, payload)`.
* **Bridge Goroutine:** Application startup spawns `StartRedisSubscriberBridge()` which runs `for msg := range pubsub.Channel()` and pushes incoming payloads directly to `hub.broadcast`. This guarantees every application instance receives all cluster seat events and forwards them to locally connected WebSockets.
* **At-Most-Once Delivery vs. HTTP Snapshot:** Because Redis Pub/Sub does not store message history (At-Most-Once delivery), edge-case network drops or server restarts are handled gracefully by having reconnecting clients fetch a fresh snapshot via `GET /events/:id/seats` before consuming WebSockets deltas.

### React WebSocket Integration & Stale Closure Prevention
* **Event-Driven Lifecycle:** Replaces `setInterval` short polling with `new WebSocket(wsUrl)` inside `useEffect`.
* **Functional Updater Pattern (`setSeats(prev => ...)`):** To prevent JavaScript closures from capturing stale state arrays when WS messages arrive, React state mutations inside `ws.onmessage` MUST use the functional state updater form `setSeats((prevSeats) => ...)` to guarantee atomic updates against the latest state array.
* **React 18 StrictMode Cleanup:** Returning `return () => ws.close()` from `useEffect` ensures that during React 18 development double-mounting, open sockets are closed before duplicate ones are initialized.

## 13. Horizontal Scaling & Load Balancing (Nginx)

### The Bottleneck
When running load tests, we discovered that while Redis and PostgreSQL could handle the throughput, the Go API server became CPU-bound under extreme concurrent load (e.g., thousands of simultaneous requests). 

### The Solution: Nginx Reverse Proxy
To solve this, we implemented horizontal scaling using an Nginx reverse proxy.
* **Architecture:** Instead of a single API container, `docker-compose` now spins up three identical API containers (`api1`, `api2`, `api3`).
* **Reverse Proxy:** Nginx sits in front of these APIs. All external traffic hits Nginx on port 80.
* **Load Balancing Strategy (`least_conn`):** We configured the Nginx `upstream` block to use the `least_conn` algorithm. Unlike default round-robin (which just blindly rotates through servers), `least_conn` dynamically routes the next incoming request to the API container that currently has the fewest active connections. This ensures optimal load distribution, especially when some requests (like database writes) take longer than others.
* **Internal Docker Networking:** The host machine only exposes the Nginx port. Nginx communicates with the three APIs over the internal Docker bridge network on their internal port `8080`, keeping the APIs isolated and secure from direct external access.

## 14. Reverse Proxy & Connection Pool Tuning

### The Hidden Nginx Bottleneck
After deploying Nginx with default settings, we discovered that the reverse proxy itself became the bottleneck under high concurrency. This is a classic and extremely common production mistake.

#### `worker_processes` & `worker_connections`
* **Default Trap:** An empty `events {}` block gives Nginx 1 worker process with 512 max connections. At 2,000 concurrent users, the TCP accept queue overflows and requests either queue or get dropped.
* **Fix:** `worker_processes auto` uses all available CPU cores. `worker_connections 4096` allows each worker to handle thousands of connections. The total maximum concurrent connections Nginx can handle is `worker_processes × worker_connections`.

#### Upstream Keepalive (Critical for Performance)
* **Default Behavior (No Keepalive):** Without `keepalive` in the `upstream` block, Nginx opens a **new TCP connection** to the backend for every single proxied request, and closes it immediately after the response. Under load, this creates massive overhead: TCP handshake latency, `TIME_WAIT` socket accumulation, and port exhaustion.
* **Fix:** `keepalive 64` tells Nginx to maintain a pool of 64 idle, reusable connections to the backend servers. Combined with `proxy_http_version 1.1` and `proxy_set_header Connection ""` (which tells the upstream to keep the connection alive instead of closing it), this dramatically reduces connection overhead.
* **Why Both Settings Are Required:** HTTP/1.0 defaults to `Connection: close`. Setting `proxy_http_version 1.1` switches to HTTP/1.1 (which defaults to keep-alive), and explicitly clearing the `Connection` header prevents Nginx from passing the client's `Connection: close` header to the backend.

### Redis Connection Pool Sizing
* **Default Trap:** The `go-redis` client defaults to `PoolSize = 10 * runtime.GOMAXPROCS(0)`. Inside a Docker container without explicit CPU limits, `GOMAXPROCS` may detect only 1 or 2 cores, resulting in a pool of just 10-20 connections.
* **The Multiplier Effect:** If each API request performs multiple Redis operations (e.g., `SETNX` for the hold + `Publish` for WebSocket fanout = 2 ops per request), the effective concurrency on Redis connections is doubled. With 500 concurrent requests and only 10 pool connections, goroutines queue up waiting for a free connection.
* **Fix:** Explicitly set `PoolSize: 100` and `MinIdleConns: 20`. The `MinIdleConns` ensures connections are pre-warmed and ready, avoiding cold-start latency spikes on the first burst of traffic after a quiet period.
* **Rule of Thumb:** `PoolSize` should be at least `max_concurrent_requests_per_instance × redis_ops_per_request`. For 500 concurrent requests doing 2 Redis ops each, you need at least 1000 connections (or accept some queuing). In practice, not all requests hit Redis simultaneously, so 100 is a good balance.



