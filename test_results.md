# Concurrent Seat Booking System - Test Results

This document contains the raw test output from the various concurrency tests implemented during the project's development. All tests run 100 simultaneous goroutines using a `start` channel to release them at the exact same millisecond. Tests are run with the Go race detector enabled (`-race`).

## 1. Naive In-Memory Store (`TestInmemStore_ConcurrentBookings`)

**Expected Result:** Fail (Data Race / Crash)
**Actual Result:** Fail (Data Race)

```text
=== RUN   TestInmemStore_ConcurrentBookings
==================
WARNING: DATA RACE
Read at 0x00c00012f0b0 by goroutine 79:
  runtime.mapaccess2_faststr()
      /usr/local/go/src/internal/runtime/maps/runtime_faststr.go:161 +0x0
  concurrent-seat-booking-system/internal/booking.(*InmemRepository).Book()
      /home/vedant/projects/concurrent-seat-booking-system/internal/booking/inmem_store.go:19 +0x2e8
  concurrent-seat-booking-system/internal/booking.TestInmemStore_ConcurrentBookings.func1()
      /home/vedant/projects/concurrent-seat-booking-system/internal/booking/inmem_test.go:31 +0x225
  concurrent-seat-booking-system/internal/booking.TestInmemStore_ConcurrentBookings.gowrap1()
      /home/vedant/projects/concurrent-seat-booking-system/internal/booking/inmem_test.go:32 +0x38

Previous write at 0x00c00012f0b0 by goroutine 10:
  runtime.mapassign_faststr()
      /usr/local/go/src/internal/runtime/maps/runtime_faststr.go:263 +0x0
  concurrent-seat-booking-system/internal/booking.(*InmemRepository).Book()
      /home/vedant/projects/concurrent-seat-booking-system/internal/booking/inmem_store.go:22 +0x36b
  concurrent-seat-booking-system/internal/booking.TestInmemStore_ConcurrentBookings.func1()
      /home/vedant/projects/concurrent-seat-booking-system/internal/booking/inmem_test.go:31 +0x225
  concurrent-seat-booking-system/internal/booking.TestInmemStore_ConcurrentBookings.gowrap1()
      /home/vedant/projects/concurrent-seat-booking-system/internal/booking/inmem_test.go:32 +0x38

==================
    testing.go:1712: race detected during execution of test
--- FAIL: TestInmemStore_ConcurrentBookings (0.00s)
FAIL
```

## 2. Concurrent In-Memory Store (`TestConcurrentInmemStore_ConcurrentBookings`)

**Expected Result:** Pass (Mutex handles concurrency)
**Actual Result:** Pass

```text
=== RUN   TestConcurrentInmemStore_ConcurrentBookings
--- PASS: TestConcurrentInmemStore_ConcurrentBookings (0.00s)
PASS
ok      concurrent-seat-booking-system/internal/booking 1.015s
```

## 3. Redis Persistent Store (`TestRedisStore_ConcurrentBookings`)

**Expected Result:** Pass (Redis SETNX handles distributed concurrency)
**Actual Result:** Pass

```text
=== RUN   TestRedisStore_ConcurrentBookings
2026/07/20 03:34:22 Connected to Redis: localhost:6379
2026/07/20 03:34:22 Booking held for user user-81: &{ID:74729599-97b2-4cb7-a018-a93cd9464217 EventID:event-1 SeatID:seat-1 UserID:user-81 Status:HELD ExpiresAt:2026-07-20 03:34:23.788297743 +0530 IST m=+1.207095369}
--- PASS: TestRedisStore_ConcurrentBookings (0.03s)
PASS
ok      concurrent-seat-booking-system/internal/booking 1.045s
```

## 4. Hybrid Store (Redis + Postgres) Latency Benchmark

**Expected Result:** Postgres is faster for sequential requests due to fewer network hops.
**Actual Result:** As expected, Postgres was ~0.35ms faster per request sequentially.

```text
=== RUN   TestLatencyComparison

--- LATENCY BENCHMARK (50 sequential inserts) ---
Postgres Store Average Latency: 1.250242ms
Hybrid Store Average Latency:   1.608932ms
--------------------------------------------------
--- PASS: TestLatencyComparison (0.17s)
PASS
ok  	concurrent-seat-booking-system/internal/booking	0.181s
```

## 5. Hybrid Store (Redis + Postgres) Concurrency Test

**Expected Result:** Pass (Exactly 1 success. Redis absorbs the vast majority of the concurrent stampede, protecting Postgres from double-booking lock contention).
**Actual Result:** Pass

```text
=== RUN   TestHybridStoreConcurrency

--- CONCURRENCY TEST (100 goroutines) ---
Total Successes: 1
Total Failures: 99
Rejected by Redis: 97
Rejected by Postgres: 2
----------------------------------------
--- PASS: TestHybridStoreConcurrency (0.08s)
PASS
ok  	concurrent-seat-booking-system/internal/booking	0.093s
```

## 6. Heavy Stampede (API Load Test)

**Scenario:** 5,000 concurrent requests (500 workers) attempting to hold the exact same seat simultaneously.
**Expected Result:** Pass (Exactly 1 success. Redis SETNX correctly blocks all 4999 other requests from reaching Postgres).
**Actual Result:** Pass (0 double bookings, ~9300 RPS, Max Latency ~207ms).

```text
--- Load Test Results ---
Total Requests: 5000
Concurrency: 500
Total Time: 536.19939ms
Requests Per Second (RPS): 9324.89

--- Status Codes ---
Successes (201): 1
Conflicts (409): 4999
Errors/Other: 0

--- Latency Distribution ---
Average: 46.608216ms
P50: 34.186132ms
P95: 132.994648ms
P99: 153.532148ms
Max: 207.559118ms
```

## 7. Heavy General Load (API Load Test)

**Scenario:** 5,000 concurrent requests (500 workers) attempting to hold 5,000 different seats.
**Expected Result:** Pass (All 5000 succeed. System correctly scales and distributes load without locking conflicts).
**Actual Result:** Pass (~9000 RPS, Max Latency ~207ms).

```text
--- Load Test Results ---
Total Requests: 5000
Concurrency: 500
Total Time: 555.580071ms
Requests Per Second (RPS): 8999.60

--- Status Codes ---
Successes (201): 5000
Conflicts (409): 0
Errors/Other: 0

--- Latency Distribution ---
Average: 48.090494ms
P50: 38.095568ms
P95: 108.600702ms
P99: 138.417248ms
Max: 206.949821ms
```

## 8. Performance Engineering: Phase 1 (Bottleneck Identification)

**Objective:** Systematically increase load on the `/events/{id}/hold` endpoint (Redis-backed locking mechanism) to identify the true breaking point of the system rather than targeting an arbitrary RPS number.
**Methodology:** Stepped concurrency (100, 250, 500, 1000, 2000 users) generating 20 requests per user. Recorded system latency, error rate, and container CPU/Memory saturation.

### Results Summary

| Concurrency | Total Requests | RPS | Avg Latency | P99 Latency | Max Latency | API CPU | Redis CPU |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **100** | 2,000 | 7,649 | 12ms | 35ms | 41ms | ~0.1% | ~16.0% |
| **250** | 5,000 | 10,197 | 22ms | 55ms | 74ms | ~0.1% | ~28.0% |
| **500** | 10,000 | 10,824 | 41ms | 114ms | 165ms | ~0.0% | ~68.8% |
| **1000** | 20,000 | 10,305 | 89ms | 257ms | 344ms | Spiked to 661% | ~75.8% |
| **2000** | 40,000 | 9,863 | 181ms | 620ms | 984ms | Spiked to 519% | ~76.7% |

*Note: The test correctly yielded a mix of `201 Created` and `409 Conflict` (for already held seats), proving the `SET NX` concurrency locks were strictly honored under load.*

### Bottleneck Analysis
- **Breaking Point**: Latency begins to meaningfully degrade between 1000 and 2000 concurrent users. At 2000 concurrency, the P99 latency spikes past 500ms (620ms) and max latency reaches nearly 1 second.
- **The Bottleneck**: The primary bottleneck is the **API Server CPU**, which completely saturates (consuming 5 to 6 full CPU cores). 
- **Secondary Constraint**: The **Redis Container CPU** is also nearing its limit for a single core (~76% utilized), indicating that even if we scaled the API server horizontally, Redis' single-threaded command execution would become the bottleneck shortly after. Memory consumption for both the API (max ~230MB) and Redis (max ~23MB) was perfectly stable and negligible.

![Docker Stats showing API at ~600% CPU and Redis at ~76%](docs/docker_stats.png)

### The "Restaurant" Analogy (Understanding the Results)
If you're new to performance engineering, here's a simple way to visualize exactly what we did and what we found:

Imagine our system is a popular new restaurant. The **API server** acts as the **Waiters** taking orders, and **Redis** acts as the **Head Chef** checking the whiteboard to ensure a meal (seat) isn't sold out.

1. **What we did**: Instead of guessing the restaurant's capacity, we sent increasingly large waves of customers (100, then 500, then 2,000) through the door at the exact same time.
2. **How we measured**: We timed how long people waited in line (Latency) and hooked up heart rate monitors to the staff (CPU usage via `docker stats`).
3. **The Metrics**:
   - **RPS (Requests per Second):** How many orders were taken per second. We peaked around 10,000!
   - **P99 Latency:** The wait time for the unluckiest 1% of customers in line. It jumped to 620ms when 2,000 customers hit the doors at once.
4. **The Bottleneck (The Outcome)**: When the wait time spiked, we checked the heart rate monitors. The Head Chef (Redis) was working hard (76% capacity) but keeping up. The problem was the Waiters (API Server)! They were maxed out at ~600% capacity (running as fast as 6 CPU cores would allow). The line backed up because there weren't enough waiters to write down the orders.
5. **Next Steps**: To serve more customers, we don't need a faster chef yet. We need to hire more waiters (Horizontal Scaling: adding a second API server).
