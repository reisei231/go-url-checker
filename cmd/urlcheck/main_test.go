package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reisei231/go-url-checker/internal/urlcheck"
)

func TestLoadURLsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "urls.txt")
	content := "https://a.example\n\nhttps://b.example\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	urls, err := loadURLs(path, nil)
	if err != nil {
		t.Fatalf("loadURLs: %v", err)
	}
	if len(urls) != 2 || urls[0] != "https://a.example" || urls[1] != "https://b.example" {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestLoadURLsFromStdin(t *testing.T) {
	data := strings.NewReader("https://c.example\nhttps://d.example\n")
	urls, err := loadURLs("", data)
	if err != nil {
		t.Fatalf("loadURLs: %v", err)
	}
	if len(urls) != 2 || urls[0] != "https://c.example" || urls[1] != "https://d.example" {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestWriteOutputsCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	results := []urlcheck.Result{
		{URL: "https://ok.example", OK: true, Status: 200, Attempts: 1},
		{URL: "https://bad.example", OK: false, Status: 500, Attempts: 1, Error: "boom"},
	}
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	if err := writeOutputs(results, false); err != nil {
		w.Close()
		os.Stdout = stdout
		t.Fatalf("writeOutputs: %v", err)
	}
	w.Close()
	os.Stdout = stdout
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	validData, err := os.ReadFile(".out/valid.txt")
	if err != nil {
		t.Fatalf("read valid: %v", err)
	}
	invalidData, err := os.ReadFile(".out/invalid.txt")
	if err != nil {
		t.Fatalf("read invalid: %v", err)
	}
	if !strings.Contains(string(validData), "https://ok.example") {
		t.Fatalf("missing ok url in valid.txt: %s", string(validData))
	}
	if !strings.Contains(string(invalidData), "https://bad.example") {
		t.Fatalf("missing bad url in invalid.txt: %s", string(invalidData))
	}
	output := buf.String()
	if !strings.Contains(output, "URL") || !strings.Contains(output, "STATUS") {
		t.Fatalf("table output missing headers: %s", output)
	}
}
