

1) go mod tidy -- install
2) go test ./...
2) go run ./cmd/urlcheck -file urls.txt -concurrency 5 -timeout 3s -retries 2


Результат в out/valid.txt и out/invalid.txt