APP   := analyzer
IMAGE := web-page-analyzer:latest

# load .env when it exists and pass the values on to the commands
-include .env
export

.PHONY: help run build test cover lint fmt docker-build docker-run up down clean

help: ## list the targets
	@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-14s %s\n", $$1, $$2}'

run: ## run the server with the settings from .env
	go run ./cmd/server

build: ## build the binary into bin/
	CGO_ENABLED=0 go build -o bin/$(APP) ./cmd/server

test: ## run the tests with the race detector
	go test -race ./...

cover: ## run the tests and print the total coverage
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

lint: ## run golangci-lint
	golangci-lint run ./...

fmt: ## format the code
	golangci-lint fmt

docker-build: ## build the docker image
	docker build -t $(IMAGE) .

docker-run: docker-build ## build and run the image on :8080
	docker run --rm -p 8080:8080 --stop-timeout 40 $(IMAGE)

up: ## start with docker compose
	docker compose up --build

down: ## stop docker compose
	docker compose down

clean: ## remove build and coverage output
	rm -rf bin coverage.out
