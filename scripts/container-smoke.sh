#!/usr/bin/env bash
set -euo pipefail
# Public HTTPS source, then a negative control with deliberately absent roots.
image=${1:?image required}
manifest="$(pwd)/testdata/packaging/https-watch.yaml"
container=
trap 'if [[ -n "$container" ]]; then docker rm -f "$container" >/dev/null; fi' EXIT
for mode in trusted untrusted; do
  if [[ "$mode" == untrusted ]]; then
    container=$(docker run -d -e SSL_CERT_FILE=/missing -e SSL_CERT_DIR=/missing --mount "type=bind,src=$manifest,dst=/smoke.yaml,readonly" "$image")
  else
    container=$(docker run -d --mount "type=bind,src=$manifest,dst=/smoke.yaml,readonly" "$image")
  fi
  ready=false
  for ((n=0;n<30;n++)); do
    if docker exec "$container" /ding watch list --state-dir /var/lib/ding --json >/dev/null 2>&1; then ready=true; break; fi
    sleep 1
  done
  if [[ "$ready" != true ]]; then docker logs "$container"; exit 1; fi
  docker exec "$container" /ding apply /smoke.yaml --state-dir /var/lib/ding --json >/dev/null
  passed=false
  for ((n=0;n<30;n++)); do
    events=$(docker exec "$container" /ding events --watch verified-https --state-dir /var/lib/ding --json)
    if [[ "$mode" == trusted && "$events" == *'"type":"firing"'* ]]; then passed=true; break; fi
    if [[ "$mode" == untrusted && "$events" == *'"type":"source_error"'* && "$events" != *'"type":"firing"'* ]]; then passed=true; break; fi
    sleep 1
  done
  if [[ "$passed" != true ]]; then printf '%s\n' "$events" >&2; docker logs "$container"; exit 1; fi
  docker stop -t 10 "$container" >/dev/null
  docker rm "$container" >/dev/null
  container=
done
docker run --rm "$image" version
