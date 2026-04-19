.PHONY: build test run clean docker-build docker-push docker-release

BINARY_NAME=zipserver
DOCKER_USER ?= kvsh
IMAGE_NAME = zipserver
VERSION ?= latest

build:
	go build -o $(BINARY_NAME) ./cmd/zipserver

test:
	go test ./...

run: build
	./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	go clean

docker-build:
	docker build -t $(DOCKER_USER)/$(IMAGE_NAME):$(VERSION) .

docker-push:
	docker push $(DOCKER_USER)/$(IMAGE_NAME):$(VERSION)

docker-release: docker-build docker-push
