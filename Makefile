VERSION ?= 2026.9.3
BIN_DIR = dist
BINARY_NAME = token-analyzer-go

.PHONY: all build test clean release sync-webui

all: test build


# `src/webui.html` is canonical; `sync-webui` refreshes the copy used by Go embed.
sync-webui:
	@cp -f src/webui.html internal/server/webui.html

build: sync-webui
	@mkdir -p $(BIN_DIR)
	go build -ldflags "-s -w" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/token-analyzer
	@echo "Build successful: $(BIN_DIR)/$(BINARY_NAME)"

test: sync-webui
	go test -v ./...

clean:
	rm -rf $(BIN_DIR)/token-analyzer*

release: sync-webui
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-linux-amd64 ./cmd/token-analyzer
	GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-linux-arm64 ./cmd/token-analyzer
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-darwin-amd64 ./cmd/token-analyzer
	GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-darwin-arm64 ./cmd/token-analyzer
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/token-analyzer-windows-amd64.exe ./cmd/token-analyzer
	@echo "Cross-compilation release builds completed in $(BIN_DIR)/"
