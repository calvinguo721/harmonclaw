package tests

import (
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func TestStress_ConcurrentHealth(t *testing.T) {
	baseURL := "http://127.0.0.1:8080"
	if u := os.Getenv("HC_TEST_BASE_URL"); u != "" {
		baseURL = u
	}
	const concurrency = 20
	const total = 100
	var wg sync.WaitGroup
	var errCount int
	var mu sync.Mutex
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(baseURL + "/v1/health")
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
				return
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}()
		if i%concurrency == 0 && i > 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	wg.Wait()
	if errCount == total {
		t.Skip("server not running (all requests failed)")
	}
	if errCount > total/2 {
		t.Errorf("too many errors: %d/%d", errCount, total)
	}
}
