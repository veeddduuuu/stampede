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

