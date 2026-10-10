.PHONY: console headless test-console test-mcp mcp

MCP_VERSION ?= 0.1.0
MCP_OS := $(shell go env GOOS)
MCP_ARCH := $(shell go env GOARCH)
MCP_EXT := $(if $(filter windows,$(MCP_OS)),.exe,)
MCP_RUNTIME := dist/go-mcp/$(MCP_OS)-$(MCP_ARCH)/ding-mcp

console:
	cd web/console && npm ci && npm run build
	go build -tags console -o ding ./cmd/ding

headless:
	go build -o ding ./cmd/ding

test-console:
	go test ./internal/consolecontract
	cd web/console && npm ci && npm run build
	go test -race -tags console ./...
	cd web/console && npm test && npm run test:e2e

mcp:
	npm ci --prefix web/mcp-app
	npm run build --prefix web/mcp-app
	CGO_ENABLED=0 go build -tags mcpui -trimpath -ldflags='-s -w -X main.version=$(MCP_VERSION)' -o $(MCP_RUNTIME)/ding-mcp$(MCP_EXT) ./cmd/ding-mcp

test-mcp:
	npm ci --prefix web/mcp-app
	npm run build --prefix web/mcp-app
	npm test --prefix web/mcp-app
	go test -race -tags mcpui ./internal/mcp... ./internal/pluginpackage ./internal/store ./internal/watchrun ./internal/control ./internal/consolecontract ./internal/watchcli
	go build -tags mcpui -o dist/ding-integration-test$(MCP_EXT) ./cmd/ding
	go build -tags mcpui -o dist/ding-mcp-test$(MCP_EXT) ./cmd/ding-mcp
	DING_TEST_BINARY="$(CURDIR)/dist/ding-integration-test$(MCP_EXT)" DING_MCP_BINARY="$(CURDIR)/dist/ding-mcp-test$(MCP_EXT)" npm run test:interop --prefix web/mcp-app
	DING_TEST_BINARY="$(CURDIR)/dist/ding-integration-test$(MCP_EXT)" DING_MCP_BINARY="$(CURDIR)/dist/ding-mcp-test$(MCP_EXT)" npm run test:e2e --prefix web/mcp-app
