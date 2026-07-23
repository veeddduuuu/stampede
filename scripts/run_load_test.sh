#!/bin/bash

set -e

# Increase open files limit for the load generator
ulimit -n 20000

# Optional: uncomment to restart containers before the full suite
# docker compose down
# docker compose up -d --build
# sleep 5

CONCURRENCIES=(100 250 500 1000 2000)

echo "Starting Phased Load Testing..."
echo "Target URL: http://localhost:8000/events/event-1/hold"

for c in "${CONCURRENCIES[@]}"; do
    reqs=$((c * 20))
    echo ""
    echo "====================================================="
    echo "🔥 STEP: $c Concurrent Users ($reqs total requests)"
    echo "====================================================="
    
    echo "Flushing Redis holds before step..."
    docker compose exec -T redis redis-cli FLUSHALL
    
    # Capture metrics in the background while the test runs
    # We will poll docker stats every 0.2s and save to a temporary file
    rm -f .docker_stats.tmp
    (
        while true; do
            docker stats --no-stream api1 api2 api3 redis --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" >> .docker_stats.tmp 2>/dev/null || true
            sleep 0.2
        done
    ) &
    STATS_PID=$!

    # Run the load test
    go run cmd/loadtest/main.go -requests $reqs -concurrency $c -scenario general -url "http://localhost:8000/events/event-1/hold"
    
    # Stop the stats collection now that the test is done
    kill $STATS_PID
    wait $STATS_PID 2>/dev/null || true
    
    echo ""
    echo "--- Resource Utilization (sampled during run) ---"
    cat .docker_stats.tmp | sort | uniq
    echo "-------------------------------------------------"
    
    # Sleep a bit to let the system cool down before the next burst
    sleep 5
done

rm -f .docker_stats.tmp
echo "Load Testing Complete."
