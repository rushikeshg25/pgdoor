.PHONY: build run test vet fuzz clean

build:
	go build -o bin/pgdoor ./cmd/pgdoor

run: build
	./bin/pgdoor -listen :6432 -upstream localhost:5432

trace: build
	./bin/pgdoor -listen :6432 -upstream localhost:5432 -trace

test:
	go test ./...

vet:
	go vet ./...

# Phase 1: fuzz the framer against truncated headers and absurd lengths.
fuzz:
	go test ./internal/pgwire -run=Fuzz -fuzz=FuzzReadMessage -fuzztime=60s

clean:
	rm -rf bin
