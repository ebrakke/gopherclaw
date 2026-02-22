.PHONY: build test run clean

build:
	go build -o bin/gopherclaw ./cmd/gopherclaw/

test:
	go test -v ./...

run:
	go run ./cmd/gopherclaw/ serve

clean:
	rm -rf bin/
