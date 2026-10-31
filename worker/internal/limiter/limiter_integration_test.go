//go:build integration

package limiter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
)

func newClient(t *testing.T) valkey.Client {
	t.Helper()
	url := os.Getenv("ROLERANK_TEST_VALKEY_URL")
	if url == "" {
		t.Fatal("ROLERANK_TEST_VALKEY_URL is required")
	}
	options, err := valkey.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client, err := valkey.NewClient(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func prefix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "test:" + hex.EncodeToString(b) + ":"
}

// Four workers with separate connections hammer one fake host. The host's own clock
// is the judge, so a bug can't hide inside the limiter's arithmetic.
func TestWorkersNeverExceedTheRate(t *testing.T) {
	const (
		workers  = 4
		each     = 15
		interval = 50 * time.Millisecond
	)
	var mu sync.Mutex
	var arrivals []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		mu.Unlock()
	}))
	defer server.Close()

	p := prefix(t)
	var wg sync.WaitGroup
	for range workers {
		limiter := New(newClient(t), p)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				if err := limiter.Wait(context.Background(), "fake.example", interval); err != nil {
					t.Error(err)
					return
				}
				resp, err := http.Get(server.URL)
				if err != nil {
					t.Error(err)
					return
				}
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	n := len(arrivals)
	if n != workers*each {
		t.Fatalf("got %d arrivals, want %d", n, workers*each)
	}
	sort.Slice(arrivals, func(i, j int) bool { return arrivals[i].Before(arrivals[j]) })
	elapsed := arrivals[n-1].Sub(arrivals[0])
	floor := time.Duration(n-1) * interval * 9 / 10
	if elapsed < floor {
		t.Fatalf("%d requests in %v, faster than the limit allows (floor %v)", n, elapsed, floor)
	}
	// No one-second window may hold more than the rate plus scheduling jitter.
	for i := range arrivals {
		count := 0
		for j := i; j < n && arrivals[j].Sub(arrivals[i]) < time.Second; j++ {
			count++
		}
		if count > 23 {
			t.Fatalf("%d requests inside one second; the limit is 20", count)
		}
	}
}

func TestBlockStopsEveryone(t *testing.T) {
	ctx := context.Background()
	p := prefix(t)
	first, second := New(newClient(t), p), New(newClient(t), p)
	if err := first.Block(ctx, "h.example", 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := second.Wait(ctx, "h.example", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := time.Since(start); got < 250*time.Millisecond {
		t.Fatalf("another worker went after %v, inside the block", got)
	}
}

func TestHostsAreIndependent(t *testing.T) {
	ctx := context.Background()
	l := New(newClient(t), prefix(t))
	if err := l.Block(ctx, "busy.example", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := l.Wait(ctx, "free.example", time.Second); err != nil {
		t.Fatal(err)
	}
	if got := time.Since(start); got > time.Second {
		t.Fatalf("waited %v on an unrelated host", got)
	}
}

func TestWaitHonoursCancellation(t *testing.T) {
	l := New(newClient(t), prefix(t))
	if err := l.Block(context.Background(), "h.example", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx, "h.example", time.Second); err == nil {
		t.Fatal("expected a cancellation error")
	}
}
