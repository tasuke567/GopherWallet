package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	status   int
	duration time.Duration
}

func main() {
	baseURL := "http://localhost:8080"
	if v := os.Getenv("BASE_URL"); v != "" {
		baseURL = v
	}

	if len(os.Args) < 3 {
		fmt.Println("Usage: go run ./loadtest/cmd <from_id> <to_id> [total] [concurrency]")
		os.Exit(1)
	}

	fromID, _ := strconv.ParseInt(os.Args[1], 10, 64)
	toID, _ := strconv.ParseInt(os.Args[2], 10, 64)
	total := 500
	concurrency := 20
	if len(os.Args) > 3 {
		total, _ = strconv.Atoi(os.Args[3])
	}
	if len(os.Args) > 4 {
		concurrency, _ = strconv.Atoi(os.Args[4])
	}

	fmt.Println("=== GopherWallet Transfer Load Test ===")
	fmt.Printf("Target:      %s\n", baseURL)
	fmt.Printf("Requests:    %d\n", total)
	fmt.Printf("Concurrency: %d\n", concurrency)
	fmt.Printf("From: %d -> To: %d\n\n", fromID, toID)

	results := make([]result, total)
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var completed int64
	runID := time.Now().UnixNano()

	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()

	for i := 0; i < total; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			body, _ := json.Marshal(map[string]interface{}{
				"from_account_id": fromID,
				"to_account_id":   toID,
				"amount":          1,
				"idempotency_key": fmt.Sprintf("lt-%d-%d", runID, idx),
			})

			reqStart := time.Now()
			resp, err := client.Post(
				baseURL+"/api/v1/transfers",
				"application/json",
				bytes.NewReader(body),
			)
			elapsed := time.Since(reqStart)

			if err != nil {
				results[idx] = result{status: 0, duration: elapsed}
			} else {
				results[idx] = result{status: resp.StatusCode, duration: elapsed}
				resp.Body.Close()
			}

			done := atomic.AddInt64(&completed, 1)
			step := int64(total) / 5
			if step > 0 && done%step == 0 {
				fmt.Printf("  Progress: %d/%d\n", done, total)
			}
		}(i)
	}

	wg.Wait()
	totalTime := time.Since(start)

	statusCounts := make(map[int]int)
	durations := make([]time.Duration, 0, total)
	for _, r := range results {
		statusCounts[r.status]++
		durations = append(durations, r.duration)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	fmt.Println("\n--- Results ---")
	fmt.Printf("  Total time:   %.4f secs\n", totalTime.Seconds())
	fmt.Printf("  Requests/sec: %.2f\n", float64(total)/totalTime.Seconds())
	fmt.Printf("  Fastest:      %.4f secs\n", durations[0].Seconds())
	fmt.Printf("  Slowest:      %.4f secs\n", durations[len(durations)-1].Seconds())
	fmt.Printf("  Average:      %.4f secs\n", avgDur(durations).Seconds())

	fmt.Println("\nLatency distribution:")
	fmt.Printf("  p50:  %.4f secs\n", pct(durations, 0.50).Seconds())
	fmt.Printf("  p90:  %.4f secs\n", pct(durations, 0.90).Seconds())
	fmt.Printf("  p95:  %.4f secs\n", pct(durations, 0.95).Seconds())
	fmt.Printf("  p99:  %.4f secs\n", pct(durations, 0.99).Seconds())

	fmt.Println("\nStatus code distribution:")
	for code, count := range statusCounts {
		label := ""
		switch code {
		case 201:
			label = "Created (success)"
		case 409:
			label = "Conflict (duplicate)"
		case 422:
			label = "Insufficient balance"
		case 429:
			label = "Rate limited"
		case 500:
			label = "Internal error"
		case 0:
			label = "Connection error"
		default:
			label = http.StatusText(code)
		}
		p := float64(count) / float64(total) * 100
		fmt.Printf("  [%d] %d responses (%.1f%%) - %s\n", code, count, p, label)
	}

	fmt.Println()
	success := statusCounts[201]
	errs := statusCounts[500] + statusCounts[0]
	if errs > 0 {
		fmt.Printf("WARNING: %d errors detected (%.1f%%)\n", errs, float64(errs)/float64(total)*100)
	}
	if success == total {
		fmt.Println("ALL REQUESTS SUCCEEDED!")
	} else if errs == 0 {
		fmt.Printf("OK: %d/%d transfers succeeded, 0 errors\n", success, total)
	}
}

func avgDur(d []time.Duration) time.Duration {
	var sum time.Duration
	for _, v := range d {
		sum += v
	}
	return sum / time.Duration(len(d))
}

func pct(d []time.Duration, p float64) time.Duration {
	idx := int(float64(len(d)) * p)
	if idx >= len(d) {
		idx = len(d) - 1
	}
	return d[idx]
}
