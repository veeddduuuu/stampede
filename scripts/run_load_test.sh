#!/bin/bash

set -e

# Increase open files limit for the load generator
ulimit -n 20000

BASE_URL="http://localhost:8000/events/event-1/hold"

# =====================================================================
# BENCHMARK 1: HOT SEAT STAMPEDE (Correctness under Contention)
# 
# Every single request tries to grab the SAME seat.
# Expected result: Exactly 1 success (201), all others 409.
# This proves our Redis SETNX atomic lock works perfectly.
# =====================================================================

STAMPEDE_CONCURRENCIES=(100 500 1000 2000)

echo ""
echo "╔═══════════════════════════════════════════════════════╗"
echo "║  BENCHMARK 1: HOT SEAT STAMPEDE (Correctness Test)   ║"
echo "║  All users fight over ONE seat.                      ║"
echo "║  Expected: Exactly 1 success, rest 409.              ║"
echo "╚═══════════════════════════════════════════════════════╝"

for c in "${STAMPEDE_CONCURRENCIES[@]}"; do
    reqs=$((c * 20))
    echo ""
    echo "====================================================="
    echo "🔥 STAMPEDE: $c Concurrent Users ($reqs total requests)"
    echo "====================================================="
    
    echo "Flushing Redis holds before step..."
    docker compose exec -T redis redis-cli FLUSHALL > /dev/null 2>&1
    
    # Capture metrics in the background
    rm -f .docker_stats.tmp
    (
        while true; do
            docker stats --no-stream api1 api2 api3 redis --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" >> .docker_stats.tmp 2>/dev/null || true
            sleep 0.2
        done
    ) &
    STATS_PID=$!

    # Run the stampede load test (all users fight for the same seat)
    go run cmd/loadtest/main.go -requests $reqs -concurrency $c -scenario stampede -url "$BASE_URL"
    
    # Stop the stats collection
    kill $STATS_PID
    wait $STATS_PID 2>/dev/null || true
    
    echo ""
    echo "--- Resource Utilization (sampled during run) ---"
    cat .docker_stats.tmp | sort | uniq
    echo "-------------------------------------------------"
    
    sleep 3
done

# =====================================================================
# BENCHMARK 2: GENERAL THROUGHPUT (Scaling & Performance)
# 
# Every request grabs a DIFFERENT seat.
# This measures raw RPS, latency, and horizontal scaling.
# =====================================================================

THROUGHPUT_CONCURRENCIES=(100 250 500 1000 2000)

echo ""
echo "╔═══════════════════════════════════════════════════════╗"
echo "║  BENCHMARK 2: GENERAL THROUGHPUT (Performance Test)  ║"
echo "║  Each user grabs a unique seat.                      ║"
echo "║  Expected: All succeed (201). Measures RPS/Latency.  ║"
echo "╚═══════════════════════════════════════════════════════╝"

for c in "${THROUGHPUT_CONCURRENCIES[@]}"; do
    reqs=$((c * 20))
    echo ""
    echo "====================================================="
    echo "🚀 THROUGHPUT: $c Concurrent Users ($reqs total requests)"
    echo "====================================================="
    
    echo "Flushing Redis holds before step..."
    docker compose exec -T redis redis-cli FLUSHALL > /dev/null 2>&1
    
    # Capture metrics in the background
    rm -f .docker_stats.tmp
    (
        while true; do
            docker stats --no-stream api1 api2 api3 redis --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" >> .docker_stats.tmp 2>/dev/null || true
            sleep 0.2
        done
    ) &
    STATS_PID=$!

    # Run the throughput load test (each user gets a unique seat)
    go run cmd/loadtest/main.go -requests $reqs -concurrency $c -scenario general -url "$BASE_URL"
    
    # Stop the stats collection
    kill $STATS_PID
    wait $STATS_PID 2>/dev/null || true
    
    echo ""
    echo "--- Resource Utilization (sampled during run) ---"
    cat .docker_stats.tmp | sort | uniq
    echo "-------------------------------------------------"
    
    sleep 5
done

rm -f .docker_stats.tmp
echo ""
echo "╔═══════════════════════════════════════════════════════╗"
echo "║              ALL BENCHMARKS COMPLETE                  ║"
echo "╚═══════════════════════════════════════════════════════╝"
