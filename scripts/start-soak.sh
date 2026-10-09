#!/usr/bin/env bash
# Start the real 24-hour fixture independently of the calling terminal.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ -n $(git status --porcelain) ]]; then
  echo 'Commit the tested source before starting qualification.' >&2
  exit 1
fi
revision=$(git rev-parse HEAD)
name="ding-soak-$(git rev-parse --short HEAD)-$(date -u +%Y%m%d%H%M%S)"
case $(docker info --format '{{.Architecture}}') in
  aarch64|arm64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  *) echo 'Unsupported Docker architecture.' >&2; exit 1 ;;
esac
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$scratch/ding-soak" ./cmd/ding-soak
docker create --name "$name" --cpus=4 --memory=1g \
  --mount "type=volume,src=$name,dst=/results" \
  --entrypoint /ding-soak alpine:3.23 \
  --out /results/run --duration 24h --warmup 15m --history 1h \
  --burst-every 1h --sample-every 1m --revision "$revision" >/dev/null
docker cp "$scratch/ding-soak" "$name:/ding-soak"
docker start "$name" >/dev/null
echo "Started $name at $(date -u +%FT%TZ) on Linux/$arch, capped at four CPUs and 1 GiB container memory."
echo "Progress: docker exec $name cat /results/run/progress.json"
echo "Finish: docker inspect $name --format '{{.State.Status}} {{.State.ExitCode}}'"
echo "Collect: docker cp $name:/results/run ./qualification-result"
echo 'Keep Docker and the host running. The 250 MiB RSS target includes the fixture driver.'
