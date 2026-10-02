// loadtest fires concurrent BET requests against a running instance of the
// app over a fixed duration and reports throughput and latency
// percentiles. It spreads load across several wallets so the pessimistic
// per-wallet lock (SELECT ... FOR UPDATE) isn't the sole bottleneck being
// measured — concurrency within one wallet is already covered by the
// race-detected unit tests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "app base URL")
	internalToken := flag.String("internal-token", "", "bearer token with wallets.manage scope, to open the test wallets")
	providerToken := flag.String("provider-token", "", "bearer token with wagering.write scope, to submit bets")
	providerID := flag.String("provider-id", "provider-a", "providerId claimed by -provider-token")
	duration := flag.Duration("duration", 30*time.Second, "how long to generate load")
	concurrency := flag.Int("concurrency", 20, "concurrent workers")
	numWallets := flag.Int("wallets", 10, "distinct wallets to spread load across")
	flag.Parse()

	if *internalToken == "" || *providerToken == "" {
		fmt.Fprintln(os.Stderr, "both -internal-token and -provider-token are required")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	walletIDs := make([]string, *numWallets)
	for i := range walletIDs {
		id, err := openWallet(client, *baseURL, *internalToken, fmt.Sprintf("loadtest-player-%s", uuid.NewString()))
		if err != nil {
			fmt.Fprintln(os.Stderr, "open wallet:", err)
			os.Exit(1)
		}
		walletIDs[i] = id
	}
	fmt.Printf("opened %d wallets with 1,000,000.00 BRL each\n", len(walletIDs))
	fmt.Printf("running %d workers for %s against %s ...\n", *concurrency, *duration, *baseURL)

	var total, success, failed atomic.Int64
	var mu sync.Mutex
	var latencies []time.Duration

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var wg sync.WaitGroup
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			i := 0
			for ctx.Err() == nil {
				walletID := walletIDs[(worker+i)%len(walletIDs)]
				i++

				start := time.Now()
				ok := placeBet(client, *baseURL, *providerToken, *providerID, walletID)
				elapsed := time.Since(start)

				total.Add(1)
				if ok {
					success.Add(1)
				} else {
					failed.Add(1)
				}
				mu.Lock()
				latencies = append(latencies, elapsed)
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()

	report(*duration, total.Load(), success.Load(), failed.Load(), latencies)
}

func report(wallClock time.Duration, total, success, failed int64, latencies []time.Duration) {
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}

	fmt.Printf("\nrequests: %d (success=%d, failed=%d)\n", total, success, failed)
	fmt.Printf("throughput: %.1f req/s\n", float64(total)/wallClock.Seconds())
	fmt.Printf("latency: p50=%v p95=%v p99=%v max=%v\n",
		percentile(0.50), percentile(0.95), percentile(0.99), percentile(1.0))
}

func openWallet(client *http.Client, baseURL, token, playerID string) (string, error) {
	body := map[string]string{
		"playerId":       playerID,
		"currency":       "BRL",
		"openingBalance": "1000000.00",
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := doJSON(client, http.MethodPost, baseURL+"/wallets", token, body, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func placeBet(client *http.Client, baseURL, token, providerID, walletID string) bool {
	body := map[string]string{
		"providerId":            providerID,
		"playerId":              "loadtest-player",
		"walletId":              walletID,
		"roundId":               uuid.NewString(),
		"gameId":                "loadtest-game",
		"kind":                  "BET",
		"currency":              "BRL",
		"amount":                "1.00",
		"externalTransactionId": uuid.NewString(),
		"idempotencyKey":        uuid.NewString(),
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := doJSON(client, http.MethodPost, baseURL+"/wagering/transactions", token, body, &resp); err != nil {
		return false
	}
	return resp.Status == "PROCESSED"
}

func doJSON(client *http.Client, method, url, token string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, respBody)
	}
	return json.Unmarshal(respBody, out)
}
