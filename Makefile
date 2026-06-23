.PHONY: build run tidy lint clean

BINARY := gdrive-bot
BUILD_DIR := build

build:
	mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) ./cmd/bot

run:
	go run ./cmd/bot

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR) downloads/*

# Quick setup helper — copies the example env file
setup:
	@if [ ! -f config/.env ]; then \
		cp config/.env.example config/.env; \
		echo "Created config/.env — fill in your credentials before running."; \
	else \
		echo "config/.env already exists."; \
	fi
