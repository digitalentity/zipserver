.PHONY: build test run clean

BINARY_NAME=zipserver

build:
	go build -o $(BINARY_NAME) ./cmd/zipserver

test:
	go test ./...

run: build
	./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	go clean
