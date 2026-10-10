FROM node:24-alpine AS ui
WORKDIR /src
COPY web/mcp-app/package*.json web/mcp-app/
RUN npm ci --prefix web/mcp-app
COPY web/mcp-app web/mcp-app
COPY web/console web/console
COPY design design
RUN npm run build --prefix web/mcp-app

FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/internal/mcpui/dist internal/mcpui/dist
ARG VERSION=0.1.0
RUN CGO_ENABLED=0 go build -tags mcpui -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /ding-mcp ./cmd/ding-mcp

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /ding-mcp /ding-mcp
USER 10001:10001
ENV HOME=/home/ding
EXPOSE 7677
ENTRYPOINT ["/ding-mcp"]
CMD ["serve", "--transport", "http", "--http-config", "/run/ding/http.json", "--host", "0.0.0.0"]
