#!/usr/bin/env bash
set -euo pipefail
# Run from the repository root. Uses a public CA-validated HTTPS guard and a
# negative control with no trust roots. No secrets or real delivery destinations.
image=${1:?image required}
config="$(pwd)/testdata/packaging/https-smoke.yaml"
result=$(docker run --rm --entrypoint /ding --mount "type=bind,src=$config,dst=/smoke.yaml,readonly" "$image" run --config /smoke.yaml -- /ding version)
if [[ "$result" != *'"rule":"verified_https"'* ]]; then
  echo 'Container failed verified HTTPS guard' >&2
  exit 1
fi
negative=$(docker run --rm --entrypoint /ding -e SSL_CERT_FILE=/missing -e SSL_CERT_DIR=/missing --mount "type=bind,src=$config,dst=/smoke.yaml,readonly" "$image" run --config /smoke.yaml -- /ding version)
if [[ "$negative" == *'"rule":"verified_https"'* ]]; then
  echo 'HTTPS guard unexpectedly succeeded without roots' >&2
  exit 1
fi
docker run --rm --entrypoint /ding "$image" version
