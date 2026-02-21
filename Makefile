.PHONY: build test run clean

build:
	go build -o bin/gopherclaw cmd/gopherclaw/main.go

test:
	go test -v ./...

run:
	go run cmd/gopherclaw/main.go

clean:
	rm -rf bin/
