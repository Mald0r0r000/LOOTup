APP_NAME := lootup
BUILD_DIR := bin
SRC := ./cmd/lootup
GO := go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: all build run test clean lint deps

all: build

build:
	@echo "Building $(APP_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME) $(SRC)

run: build
	@echo "Running $(APP_NAME)..."
	@$(BUILD_DIR)/$(APP_NAME)

test:
	@echo "Running tests..."
	@$(GO) test ./... -v

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

lint:
	@echo "Running go vet..."
	@$(GO) vet ./...

deps:
	@echo "Downloading dependencies..."
	@$(GO) mod tidy
	@$(GO) mod download

install: build
	@echo "Installing $(APP_NAME)..."
	@cp $(BUILD_DIR)/$(APP_NAME) $(GOPATH)/bin/ 2>/dev/null || cp $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/
