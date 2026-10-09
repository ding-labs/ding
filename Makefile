.PHONY: console headless test-console

console:
	cd web/console && npm ci && npm run build
	go build -tags console -o ding ./cmd/ding

headless:
	go build -o ding ./cmd/ding

test-console:
	go run ./cmd/console-contract
	cd web/console && npm ci && npm run build
	go test -race -tags console ./...
	cd web/console && npm run test:e2e
