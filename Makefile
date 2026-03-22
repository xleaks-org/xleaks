BINARY_NAME=xleaks
MODULE=github.com/xleaks-org/xleaks
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X $(MODULE)/pkg/version.Version=$(VERSION) -X $(MODULE)/pkg/version.BuildTime=$(BUILD_TIME)"

.PHONY: build build-all test test-unit test-integration lint proto frontend dev clean release

## Build for current platform
build: frontend
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/xleaks/

## Build for all platforms
build-all: frontend
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/xleaks/
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/xleaks/
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/xleaks/
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/xleaks/
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/xleaks/

## Run all tests
test:
	go test -race -cover ./...

## Run unit tests only
test-unit:
	go test -race -cover -short ./pkg/...

## Run integration tests
test-integration:
	go test -race -v -count=1 ./tests/integration/...

## Run protocol conformance tests
test-protocol:
	go test -race -v -count=1 ./tests/protocol/...

## Run linter
lint:
	golangci-lint run ./...

## Regenerate protobuf code
proto:
	./scripts/gen-proto.sh

## Build Next.js frontend
frontend:
	cd web && npm run build

## Development mode
dev:
	./scripts/dev.sh

## Clean build artifacts
clean:
	rm -rf bin/ web/.next/ web/out/

## Build release artifacts with checksums
release: build-all
	@mkdir -p release
	@for f in bin/$(BINARY_NAME)-*; do \
		name=$$(basename $$f); \
		shasum -a 256 $$f > release/$$name.sha256; \
		if echo $$name | grep -q windows; then \
			zip release/$$name.zip -j $$f; \
		else \
			tar czf release/$$name.tar.gz -C bin $$name; \
		fi; \
		cp $$f release/; \
	done
	@echo "Release artifacts in release/"
