VERSION ?= 2026.9.3
BIN_DIR = dist
BINARY_NAME = token-analyzer

.PHONY: all build test clean release

all: test build


# WebUI 唯一源码是 internal/server/webui.html（Go embed 直引），无 copy/sync。
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "-s -w" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/token-analyzer
	@echo "Build successful: $(BIN_DIR)/$(BINARY_NAME)"

test:
	go test -v ./...

clean:
	rm -rf $(BIN_DIR)/token-analyzer*

release:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-linux-amd64 ./cmd/token-analyzer
	GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-linux-arm64 ./cmd/token-analyzer
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-darwin-amd64 ./cmd/token-analyzer
	GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-darwin-arm64 ./cmd/token-analyzer
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-windows-amd64.exe ./cmd/token-analyzer
	@echo "Cross-compilation release builds completed in $(BIN_DIR)/"
