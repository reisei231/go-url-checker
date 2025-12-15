package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/reisei231/go-url-checker/internal/urlcheck"
)

type config struct {
	file        string
	concurrency int
	timeout     time.Duration
	retries     int
	asJSON      bool
}

func main() {
	cfg := parseFlags()
	urls, err := loadURLs(cfg.file, os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "input error: %v\n", err)
		os.Exit(1)
	}
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "no urls provided")
		os.Exit(1)
	}
	checker := urlcheck.NewChecker(cfg.concurrency, cfg.timeout, cfg.retries, nil)
	results, err := checker.Check(context.Background(), urls)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check error: %v\n", err)
		os.Exit(1)
	}
	if err := writeOutputs(results, cfg.asJSON); err != nil {
		fmt.Fprintf(os.Stderr, "output error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.file, "file", "", "path to file with urls, one per line (defaults to stdin)")
	flag.IntVar(&cfg.concurrency, "concurrency", 5, "maximum concurrent checks")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Second, "per-request timeout")
	flag.IntVar(&cfg.retries, "retries", 1, "retries on network errors")
	flag.BoolVar(&cfg.asJSON, "json", false, "output as json instead of table")
	flag.Parse()
	if cfg.concurrency < 1 {
		cfg.concurrency = 1
	}
	if cfg.timeout <= 0 {
		cfg.timeout = 5 * time.Second
	}
	if cfg.retries < 0 {
		cfg.retries = 0
	}
	return cfg
}

func loadURLs(path string, stdin io.Reader) ([]string, error) {
	var reader io.Reader
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		reader = f
	} else {
		reader = stdin
	}
	scanner := bufio.NewScanner(reader)
	var urls []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}

func writeOutputs(results []urlcheck.Result, asJSON bool) error {
	if err := os.MkdirAll(".out", 0o755); err != nil {
		return err
	}
	validPath := ".out/valid.txt"
	invalidPath := ".out/invalid.txt"
	if err := writeSplit(results, validPath, invalidPath); err != nil {
		return err
	}
	if asJSON {
		return writeJSON(results)
	}
	return writeTable(results)
}

func writeSplit(results []urlcheck.Result, validPath, invalidPath string) error {
	valid, err := os.Create(validPath)
	if err != nil {
		return err
	}
	defer valid.Close()
	invalid, err := os.Create(invalidPath)
	if err != nil {
		return err
	}
	defer invalid.Close()
	for _, r := range results {
		if r.OK {
			if _, err := fmt.Fprintln(valid, r.URL); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintln(invalid, r.URL); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(results []urlcheck.Result) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func writeTable(results []urlcheck.Result) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "URL\tSTATUS\tOK\tATTEMPTS\tERROR")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%d\t%t\t%d\t%s\n", r.URL, r.Status, r.OK, r.Attempts, r.Error)
	}
	return w.Flush()
}
