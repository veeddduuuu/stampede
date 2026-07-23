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

## 9. Performance Engineering: Phase 2 (Nginx Load Balancing — Default Config)

**Objective:** Verify that adding an Nginx load balancer and scaling to 3 API instances (`api1`, `api2`, `api3`) successfully distributes the CPU load.
**Methodology:** Ran the stepped load test (100, 250, 500, 1000, 2000 concurrent users) hitting the Nginx proxy on port 8000. Redis was NOT flushed between steps, so conflicts accumulated from prior steps.

### Results Summary

| Concurrency | Total Requests | RPS | Avg Latency | P99 Latency | Max Latency | Errors |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **100** | 2,000 | 3,518 | 26ms | 102ms | 151ms | 0 |
| **250** | 5,000 | 4,812 | 41ms | 330ms | 374ms | 0 |
| **500** | 10,000 | 4,659 | 82ms | 625ms | 1.23s | 0 |
| **1,000** | 20,000 | 4,032 | 158ms | 2.2s | 3.5s | 95 |
| **2,000** | 40,000 | 793 | 1.4s | 10.0s | 10.0s | 4,650 (Timeouts) |

### Analysis
- **Load Distribution Success:** `docker stats` confirmed that Nginx perfectly balanced the traffic using the `least_conn` strategy. CPU usage was evenly distributed across `api1`, `api2`, and `api3` (each peaking around ~130-140%).
- **Critical Bottleneck Found:** At 2,000 concurrent users, the system catastrophically failed. RPS collapsed from ~4,000 to 793, and 4,650 requests timed out at exactly 10.0 seconds. The key clue: API containers showed **0% CPU** for most of the run — requests were never reaching them. The bottleneck was Nginx itself, running with default settings (1 worker, 512 max connections, no keepalive).

## 10. Performance Engineering: Phase 3 (Nginx + Redis Pool Tuning)

**Objective:** Eliminate the Nginx connection bottleneck and Redis pool starvation discovered in Phase 2.
**Changes Applied:**
1. **Nginx:** `worker_processes auto`, `worker_connections 4096`, `keepalive 64` to upstream, `proxy_http_version 1.1`.
2. **Redis Pool:** `PoolSize: 100`, `MinIdleConns: 20` per API container.
3. **Test Script:** Added `FLUSHALL` between steps, changed stats polling to 200ms continuous sampling.

### Results Summary

| Concurrency | Total Requests | RPS | Avg Latency | P99 Latency | Max Latency | Errors |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **100** | 2,000 | 6,876 | 13ms | 44ms | 59ms | 0 |
| **250** | 5,000 | 7,908 | 29ms | 77ms | 115ms | 0 |
| **500** | 10,000 | 8,674 | 53ms | 128ms | 183ms | 0 |
| **1,000** | 20,000 | 6,757 | 113ms | 271ms | 1.01s | 0 |
| **2,000** | 40,000 | 7,997 | 233ms | 671ms | 2.02s | **0** |

### Resource Utilization (Peak at 2000 Concurrency)

| Container | Peak CPU | Peak Memory |
| :--- | :--- | :--- |
| api1 | 196% | 68 MiB |
| api2 | 201% | 66 MiB |
| api3 | 192% | 68 MiB |
| redis | 123% | 20 MiB |

### Before vs After Comparison (2000 Concurrent Users)

| Metric | Before Tuning | After Tuning | Improvement |
| :--- | :--- | :--- | :--- |
| **RPS** | 793 | 7,997 | **10x** |
| **P99 Latency** | 10.0s (timeout) | 671ms | **15x** |
| **Max Latency** | 10.0s | 2.02s | **5x** |
| **Errors** | 4,650 | 0 | **Eliminated** |

### Analysis
- **Nginx was the entire bottleneck.** The default config (1 worker, 512 connections, no keepalive) created a tiny funnel that choked at scale. Tuning it removed the bottleneck entirely.
- **Redis pool starvation was a secondary issue.** With each `/hold` request performing two Redis operations (`SETNX` + `Publish`), the default pool of ~10 connections was insufficient. Bumping to 100 eliminated goroutine queuing.
- **New bottleneck profile:** At 2000 users, each API container peaks around ~200% CPU and Redis peaks at ~123% CPU. The system is now genuinely CPU-bound rather than connection-starved. Redis' single-threaded command execution (~123% includes I/O thread overhead) is approaching its limit.

## 11. Hot Seat Stampede (Correctness Under Contention)

**Objective:** Prove that the Redis `SETNX` atomic lock guarantees **exactly 1 winner** when N users all fight over the same seat simultaneously. This is the flagship benchmark — it validates the entire architectural reason for the hybrid store.
**Methodology:** All concurrent users attempt to hold the exact same seat (`seat-stampede-{timestamp}`). Redis `FLUSHALL` before each step.

### Results Summary

| Concurrency | Total Requests | Successes (201) | Conflicts (409) | Errors | RPS |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **100** | 2,000 | **1** | 1,999 | 0 | 5,207 |
| **500** | 10,000 | **1** | 9,999 | 0 | 6,082 |
| **1,000** | 20,000 | **1** | 19,999 | 0 | 6,234 |
| **2,000** | 40,000 | **1** | 39,999 | 0 | 5,706 |

### Analysis
- **Perfect correctness at every scale.** Exactly 1 success out of N requests, every single time. Zero double bookings, zero errors, zero data corruption.
- **Redis absorbs the stampede entirely.** The 39,999 losing requests never touch Postgres — they are rejected in-memory by Redis `SETNX` returning `nil`. Postgres only sees the 1 winning booking.
- **Stampede is faster than throughput** because 99.99% of requests are instant Redis rejects (no write, no Publish). Only 1 request does the full `SETNX` + `Publish` flow.

## 12. Combined Benchmark Suite (Stampede + Throughput)

**Objective:** Run both benchmarks back-to-back to measure system behavior under sustained load.
**Key Finding:** Initially, the 2,000-user throughput test collapsed when run after the stampede suite. We traced this to **ephemeral port exhaustion** on the load testing client, not the server.

### The Client-Side Bottleneck (TIME_WAIT Exhaustion)
Our load testing script was creating a new `http.Client` for every goroutine, which used Go's default `http.Transport` (`MaxIdleConnsPerHost: 2`). As a result, 99.9% of the connections were closed after a single use and entered the `TIME_WAIT` state. 

By the time the test reached the final 2000-user step, it had opened over 100,000 connections, completely exhausting the Linux ephemeral port range (~28,000 ports). New connections couldn't be made, resulting in timeouts.

**The Fix:** We configured a shared `http.Transport` in the load testing tool with a proper connection pool (`MaxIdleConnsPerHost: 2100`), ensuring TCP connections were reused across the entire test suite.

### Final Throughput Results (Run After Stampede)

After fixing the load-generator, the system performed flawlessly across the entire combined suite:

| Concurrency | Total Requests | RPS | Avg Latency | P99 Latency | Max Latency | Errors |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **100** | 2,000 | 7,885 | 11ms | 28ms | 38ms | 0 |
| **250** | 5,000 | 9,127 | 25ms | 67ms | 108ms | 0 |
| **500** | 10,000 | 9,186 | 51ms | 113ms | 190ms | 0 |
| **1,000** | 20,000 | 9,211 | 99ms | 324ms | 475ms | 0 |
| **2,000** | 40,000 | **9,276** | 205ms | 508ms | 1.52s | **0** |

### Analysis
- **Sustained Performance:** The cluster maintained an incredible **~9,200 RPS** at 2000 concurrent users even after grinding through the 70,000 requests of the stampede benchmark beforehand.
- **Hardware Limits Reached:** At 2,000 users, each API container peaked at ~230% CPU, and Redis peaked at ~120% CPU. The system is perfectly balanced and utilizing all available resources.
- **The Ultimate Takeaway:** Our architecture (Nginx load balancer + 3 Go API Nodes + Redis SETNX/PubSub + Postgres) is robust, scales horizontally, handles brutal contention flawlessly, and sustains high throughput without breaking a sweat.
