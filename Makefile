SHELL := /bin/bash

.PHONY: help fmt lint test run-smtpd run-worker tidy

help:
	@echo "make fmt|lint|test|run-smtpd|run-worker|tidy"

fmt:
	gofmt -w .

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 2; }
	golangci-lint run ./...

test:
	go test ./...

run-smtpd:
	go run ./cmd/smtpd

run-worker:
	go run ./cmd/worker

run-api:
	go run ./cmd/api

run-indexer:
	go run ./cmd/indexer

tidy:
	go mod tidy
