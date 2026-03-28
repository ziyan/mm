.PHONY: build clean test coverage lint format vendor tidy e2e e2e-up e2e-down

BINARY := mm
BUILD_DIR := .
GO := go
GOFLAGS := -mod=vendor
CGO_ENABLED := 0

VERSION ?= $(shell git describe --tags 2>/dev/null || echo 0.1.0)
COMMIT ?= $(shell git describe --match=NeVeRmAtCh --always --abbrev=40 --dirty)
LDFLAGS := -s -w \
	-X github.com/ziyan/mm/internal/version.version=$(VERSION) \
	-X github.com/ziyan/mm/internal/version.commit=$(COMMIT)

build:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY) ./command/

clean:
	rm -f $(BUILD_DIR)/$(BINARY) mm-e2e-test
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

e2e-up:
	docker compose -f test/docker-compose.yml up -d --wait

e2e-down:
	docker compose -f test/docker-compose.yml down -v

e2e: e2e-up
	$(GO) test $(GOFLAGS) -tags=e2e -v -count=1 -timeout=5m ./test/
	$(GO) tool cover -func=coverage/e2e-coverage.out
	$(MAKE) e2e-down
