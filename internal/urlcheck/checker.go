package urlcheck

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type Result struct {
	URL      string `json:"url"`
	OK       bool   `json:"ok"`
	Status   int    `json:"status"`
	Error    string `json:"error,omitempty"`
	Attempts int    `json:"attempts"`
}

type Checker struct {
	client      *http.Client
	concurrency int
	timeout     time.Duration
	retries     int
}

func NewChecker(concurrency int, timeout time.Duration, retries int, client *http.Client) *Checker {
	if concurrency < 1 {
		concurrency = 1
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if retries < 0 {
		retries = 0
	}
	if client == nil {
		client = &http.Client{}
	}
	return &Checker{
		client:      client,
		concurrency: concurrency,
		timeout:     timeout,
		retries:     retries,
	}
}

func (c *Checker) Check(ctx context.Context, urls []string) ([]Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]Result, len(urls))
	type job struct {
		idx int
		url string
	}
	type workerResult struct {
		idx int
		res Result
	}
	jobs := make(chan job)
	out := make(chan workerResult, len(urls))
	var wg sync.WaitGroup
	for i := 0; i < c.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				out <- workerResult{idx: j.idx, res: c.checkOne(ctx, j.url)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for idx, url := range urls {
			select {
			case <-ctx.Done():
				return
			case jobs <- job{idx: idx, url: url}:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(out)
	}()
	for r := range out {
		results[r.idx] = r.res
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return results, err
	}
	return results, nil
}

func (c *Checker) checkOne(ctx context.Context, target string) Result {
	attempts := 0
	var lastErr error
	for attempts <= c.retries {
		attempts++
		reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
		if err != nil {
			cancel()
			lastErr = err
			break
		}
		resp, err := c.client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			if c.shouldRetry(err) && attempts <= c.retries {
				continue
			}
			break
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		cancel()
		ok := resp.StatusCode >= 200 && resp.StatusCode < 400
		return Result{
			URL:      target,
			OK:       ok,
			Status:   resp.StatusCode,
			Attempts: attempts,
		}
	}
	errText := ""
	if lastErr != nil {
		errText = lastErr.Error()
	}
	return Result{
		URL:      target,
		OK:       false,
		Status:   0,
		Error:    errText,
		Attempts: attempts,
	}
}

func (c *Checker) shouldRetry(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}
