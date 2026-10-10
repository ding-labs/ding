#!/bin/sh
set -eu
# Root-operated systemd maintenance job. Requires a dedicated backup filesystem
# owned by ding-cloud; the age identity and master data key live elsewhere.
exec 9>/run/lock/ding-cloud-backup.lock
flock -n 9 || exit 1
ding_output=/var/backups/ding-cloud
ding_recipient=$(cat /etc/ding-cloud/backup-recipient)
test -d "$ding_output"
systemctl stop ding-cloud.service
trap 'systemctl start ding-cloud.service' EXIT HUP INT TERM
runuser -u ding-cloud -- /opt/ding-cloud/ding-cloud backup \
  --data-dir /var/lib/ding-cloud \
  --output "$ding_output/ding-cloud-$(date -u +%Y%m%dT%H%M%SZ).age" \
  --recipient "$ding_recipient"
# Only this job's regular encrypted snapshots expire, after seven days.
find "$ding_output" -maxdepth 1 -type f -name 'ding-cloud-????????T??????Z.age' -mmin +10080 -delete
