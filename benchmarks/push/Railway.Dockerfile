FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go test -c -o /cloud.test ./internal/cloud
RUN CGO_ENABLED=0 go build -o /ding-local-load ./cmd/ding-local-load

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends python3 python3-psycopg postgresql-client openssl ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /cloud.test /ding-local-load /opt/
COPY benchmarks/push/*.py benchmarks/push/cases.json /opt/lab/
RUN mkdir -p /data/ding-lab && chown -R 10001:10001 /data
USER 10001
ENV PYTHONUNBUFFERED=1
CMD ["python3", "/opt/lab/railway_app.py"]
