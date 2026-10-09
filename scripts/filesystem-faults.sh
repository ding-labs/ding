#!/usr/bin/env bash
set -euo pipefail
# Isolated Docker filesystems: never fill or remount the host's working disk.
arch=${1:-$(go env GOARCH)}
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go test -c -o "$fixture/store.test" ./internal/store
mount_binary="type=bind,src=$fixture/store.test,dst=/store.test,readonly"
docker run --rm --platform "linux/$arch" --read-only --tmpfs /fault:rw,size=8m -e DING_FS_FULL_DIR=/fault --mount "$mount_binary" alpine:3.23 /store.test -test.v -test.run '^TestPhysicalDiskFullRollbackAndRecovery$'
mkdir "$fixture/state"
docker run --rm --platform "linux/$arch" -e DING_FS_PREPARE_DIR=/fixture --mount "$mount_binary" --mount "type=bind,src=$fixture/state,dst=/fixture" alpine:3.23 /store.test -test.v -test.run '^TestPrepareReadOnlyFixture$'
docker run --rm --platform "linux/$arch" --read-only -e DING_FS_READONLY_DIR=/fixture --mount "$mount_binary" --mount "type=bind,src=$fixture/state,dst=/fixture,readonly" alpine:3.23 /store.test -test.v -test.run '^TestPhysicalReadOnlyStartupPreservesCommittedData$'
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go test -c -o "$fixture/runtime.test" ./internal/watchrun
docker run --rm --platform "linux/$arch" --read-only --tmpfs /fault:rw,size=8m -e DING_FS_FULL_DIR=/fault --mount "type=bind,src=$fixture/runtime.test,dst=/runtime.test,readonly" alpine:3.23 /runtime.test -test.v -test.run '^TestPhysicalFullPushIsNotAcknowledged$'
