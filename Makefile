.PHONY: console headless test-console test-mcp mcp

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
	uv sync --project integrations/mcp --frozen
	uv run --project integrations/mcp python scripts/build-mcp.py

test-mcp:
	npm ci --prefix web/mcp-app
	npm run build --prefix web/mcp-app
	npm test --prefix web/mcp-app
	npm run test:e2e --prefix web/mcp-app
	uv sync --project integrations/mcp --frozen
	go test -race ./internal/store ./internal/watchrun ./internal/control ./internal/consolecontract
	go build -o dist/ding-integration-test ./cmd/ding
	DING_TEST_BINARY="$(CURDIR)/dist/ding-integration-test" uv run --project integrations/mcp pytest integrations/mcp/tests
