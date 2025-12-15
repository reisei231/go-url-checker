package urlcheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transientRoundTripper struct {
	calls int32
}

func (t *transientRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	count := atomic.AddInt32(&t.calls, 1)
	if count == 1 {
		return nil, &net.DNSError{Err: "temp", IsTemporary: true}
	}
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusOK)
	return rec.Result(), nil
}

func TestCheckerBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/bad":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	urls := []string{server.URL + "/ok", server.URL + "/bad", server.URL + "/missing"}
	checker := NewChecker(2, 2*time.Second, 1, server.Client())
	results, err := checker.Check(context.Background(), urls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != len(urls) {
		t.Fatalf("expected %d results, got %d", len(urls), len(results))
	}
	if !results[0].OK || results[0].Status != http.StatusOK {
		t.Fatalf("expected first to be ok, got %+v", results[0])
	}
	if results[1].OK || results[1].Status != http.StatusInternalServerError {
		t.Fatalf("expected second to fail, got %+v", results[1])
	}
	if results[2].OK || results[2].Status != http.StatusNotFound {
		t.Fatalf("expected third to be not found, got %+v", results[2])
	}
}

func TestRetriesOnlyOnNetworkErrors(t *testing.T) {
	client := &http.Client{Transport: &transientRoundTripper{}}
	checker := NewChecker(1, time.Second, 2, client)
	results, err := checker.Check(context.Background(), []string{"http://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", results[0].Attempts)
	}
	if !results[0].OK || results[0].Status != http.StatusOK {
		t.Fatalf("expected success after retry, got %+v", results[0])
	}
}

func TestNoRetryOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	checker := NewChecker(1, time.Second, 2, server.Client())
	results, err := checker.Check(context.Background(), []string{server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Attempts != 1 {
		t.Fatalf("expected no retries on http error, got %d", results[0].Attempts)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var current int32
	var maxSeen int32
	var mu sync.Mutex
	order := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active := atomic.AddInt32(&current, 1)
		for {
			prev := atomic.LoadInt32(&maxSeen)
			if active <= prev {
				break
			}
			if atomic.CompareAndSwapInt32(&maxSeen, prev, active) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		mu.Lock()
		order = append(order, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	var urls []string
	for i := 0; i < 8; i++ {
		urls = append(urls, server.URL+fmt.Sprintf("/%d", i))
	}
	checker := NewChecker(3, 2*time.Second, 0, server.Client())
	_, err := checker.Check(context.Background(), urls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxSeen > 3 {
		t.Fatalf("expected max concurrency <= 3, got %d", maxSeen)
	}
	if len(order) != len(urls) {
		t.Fatalf("expected %d requests, got %d", len(urls), len(order))
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	checker := NewChecker(2, 2*time.Second, 0, server.Client())
	_, err := checker.Check(ctx, []string{server.URL, server.URL})
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
