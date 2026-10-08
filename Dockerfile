# Build stage
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ding ./cmd/ding/

# Final stage — scratch for minimal image
FROM scratch
LABEL org.opencontainers.image.title="ding"
LABEL org.opencontainers.image.description="Stream-based alerting daemon"
LABEL org.opencontainers.image.source="https://github.com/ding-labs/ding"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.url="https://ding.ing"
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /ding /ding
EXPOSE 8080
ENTRYPOINT ["/ding", "serve", "--config", "/etc/ding/ding.yaml"]
