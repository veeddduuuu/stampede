package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Result struct {
	StatusCode int
	Latency    time.Duration
	Error      error
}

func main() {
	totalRequests := flag.Int("requests", 1000, "Total number of requests to send")
	concurrency := flag.Int("concurrency", 100, "Number of concurrent workers")
	scenario := flag.String("scenario", "stampede", "Scenario to run: 'stampede' (same seat) or 'general' (different seats)")
	targetURL := flag.String("url", "http://localhost:8080/events/event-1/book", "Target URL for booking")
	flag.Parse()

	log.Printf("Starting Load Test: %s scenario, %d requests, %d concurrency", *scenario, *totalRequests, *concurrency)

	results := make(chan Result, *totalRequests)
	var wg sync.WaitGroup

	reqsPerWorker := *totalRequests / *concurrency
	extraReqs := *totalRequests % *concurrency

	startTime := time.Now()

	// Start a WaitGroup to ensure all workers start at exactly the same time to maximize concurrent stampede
	var startWg sync.WaitGroup
	startWg.Add(1)

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		numReqs := reqsPerWorker
		if i == 0 {
			numReqs += extraReqs
		}

		go func(workerID, reqCount int) {
			defer wg.Done()

			client := &http.Client{Timeout: 10 * time.Second}
			startWg.Wait() // Wait for signal to start

			stampedeSeatID := fmt.Sprintf("seat-stampede-%d", startTime.Unix())

			for j := 0; j < reqCount; j++ {
				seatID := stampedeSeatID
				if *scenario == "general" {
					seatID = fmt.Sprintf("seat-%d", workerID*1000+j)
				}
				userID := fmt.Sprintf("user-%d-%d", workerID, j)

				payload := map[string]string{
					"seat_id": seatID,
					"user_id": userID,
				}
				jsonPayload, _ := json.Marshal(payload)

				reqStartTime := time.Now()
				req, _ := http.NewRequest(http.MethodPost, *targetURL, bytes.NewBuffer(jsonPayload))
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				latency := time.Since(reqStartTime)

				if err != nil {
					results <- Result{Error: err, Latency: latency}
					continue
				}

				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				results <- Result{StatusCode: resp.StatusCode, Latency: latency}
			}
		}(i, numReqs)
	}

	startWg.Done() // Signal all workers to start
	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)

	var successes, conflicts, errors int
	var latencies []time.Duration

	for res := range results {
		latencies = append(latencies, res.Latency)
		if res.Error != nil {
			errors++
		} else if res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusOK {
			successes++
		} else if res.StatusCode == http.StatusConflict {
			conflicts++
		} else {
			errors++
		}
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	var totalLatency time.Duration
	for _, l := range latencies {
		totalLatency += l
	}

	rps := float64(*totalRequests) / totalTime.Seconds()

	fmt.Printf("\n--- Load Test Results ---\n")
	fmt.Printf("Total Requests: %d\n", *totalRequests)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Total Time: %v\n", totalTime)
	fmt.Printf("Requests Per Second (RPS): %.2f\n", rps)
	fmt.Printf("\n--- Status Codes ---\n")
	fmt.Printf("Successes (201): %d\n", successes)
	fmt.Printf("Conflicts (409): %d\n", conflicts)
	fmt.Printf("Errors/Other: %d\n", errors)
	
	if len(latencies) > 0 {
		fmt.Printf("\n--- Latency Distribution ---\n")
		fmt.Printf("Average: %v\n", totalLatency/time.Duration(len(latencies)))
		fmt.Printf("P50: %v\n", latencies[len(latencies)*50/100])
		fmt.Printf("P95: %v\n", latencies[len(latencies)*95/100])
		fmt.Printf("P99: %v\n", latencies[len(latencies)*99/100])
		fmt.Printf("Max: %v\n", latencies[len(latencies)-1])
	}
	fmt.Println("-------------------------")
}
