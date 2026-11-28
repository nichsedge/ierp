.PHONY: build test test-race vet lint-python check

build:
	go build -o bin/ierp ./cmd/ierp

test:
	go test -v ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint-python:
	python3 -m py_compile scripts/import_google_timeline.py scripts/gmail_expenses/*.py

check: vet test lint-python build
