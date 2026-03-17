.PHONY: build clean test coverage lint format vendor tidy

BINARY := mm
BUILD_DIR := .
GO := go
GOFLAGS := -mod=vendor
CGO_ENABLED := 0

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./command/

clean:
	rm -f $(BUILD_DIR)/$(BINARY)
	rm -rf coverage/

test:
	$(GO) test $(GOFLAGS) ./... -v

coverage:
	mkdir -p coverage
	gotestsum --format testdox --junitfile coverage/junit.xml -- $(GOFLAGS) -coverprofile=coverage/coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage/coverage.out -o coverage/coverage.html
	$(GO) tool cover -func=coverage/coverage.out

lint:
	golangci-lint run ./...

format:
	gofmt -s -w .
	goimports -w .

vendor:
	$(GO) mod tidy
	$(GO) mod vendor

tidy:
	$(GO) mod tidy
