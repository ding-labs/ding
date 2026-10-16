FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go test -c -o /cloud.test ./internal/cloud
RUN CGO_ENABLED=0 go build -o /ding-local-load ./cmd/ding-local-load

# Both executables use the same compiler/image and run in the same deployment.
# Pin the previous measured source by revision AND content digest.
FROM build AS baseline
WORKDIR /baseline
ADD --checksum=sha256:b8db1c1394cec70208af338ed27bcfa27392ee2df9eda58b37322de1dd78ad8c https://codeload.github.com/ding-labs/ding/tar.gz/26bb27dcb96471f1f1d2ecf4b8dfb2b0ce866cb4 /baseline-source.tar.gz
RUN tar -xzf /baseline-source.tar.gz --strip-components=1 -C /baseline \
    && CGO_ENABLED=0 go test -c -o /baseline.test ./internal/cloud \
    && printf '%s' 26bb27dcb96471f1f1d2ecf4b8dfb2b0ce866cb4 > /baseline-revision

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends python3 python3-psycopg postgresql-client openssl ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /cloud.test /ding-local-load /opt/
COPY --from=baseline /baseline.test /baseline-revision /opt/
COPY benchmarks/push/*.py benchmarks/push/cases.json /opt/lab/
RUN mkdir -p /data/ding-lab && chown -R 10001:10001 /data
USER 10001
ENV PYTHONUNBUFFERED=1
CMD ["python3", "/opt/lab/railway_app.py"]
