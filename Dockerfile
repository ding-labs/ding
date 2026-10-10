# Build matching static assets before compiling the embedded console.
FROM node:24-alpine AS console
WORKDIR /src
COPY web/console/package*.json web/console/
RUN npm ci --prefix web/console
COPY web/console web/console
COPY design design
COPY testdata/console testdata/console
RUN npm run build --prefix web/console
COPY web/mcp-app/package*.json web/mcp-app/
RUN npm ci --prefix web/mcp-app
COPY web/mcp-app web/mcp-app
RUN npm run build --prefix web/mcp-app

# Build stage
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=console /src/internal/webui/dist internal/webui/dist
COPY --from=console /src/internal/mcpui/dist internal/mcpui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -tags console,mcpui -ldflags="-s -w" -o /ding ./cmd/ding/

# Final stage — scratch for minimal image
FROM scratch
LABEL org.opencontainers.image.title="ding"
LABEL org.opencontainers.image.description="Persistent watches and durable alerts"
LABEL org.opencontainers.image.source="https://github.com/ding-labs/ding"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.url="https://ding.ing"
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /ding /ding
EXPOSE 7676
ENV HOME=/home/ding
ENTRYPOINT ["/ding"]
CMD ["daemon", "--state-dir", "/var/lib/ding"]
