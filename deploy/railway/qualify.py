#!/usr/bin/env python3
"""Exercise the built Railway image on disposable volumes; no cloud credentials.

Build: docker build -f Dockerfile.railway -t ding-cloud:railway .
Run: python3 deploy/railway/qualify.py [image]
"""
import base64
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

IMAGE = sys.argv[1] if len(sys.argv) > 1 else "ding-cloud:railway"
ORIGIN = "https://cloud.ding.ing"
name = "ding-railway-test-" + secrets.token_hex(5)
containers = []


def docker(*args, check=True, data=None):
    return subprocess.run(["docker", *args], check=check, input=data,
                          capture_output=True, text=True)


def operator(container, *args, check=True):
    return docker("exec", container, "/sbin/su-exec", "10001:10001",
                  "/ding-cloud", *args, check=check)


with tempfile.TemporaryDirectory(prefix=name) as tmp:
    key = base64.b64encode(secrets.token_bytes(32)).decode()
    password = secrets.token_urlsafe(32)
    docker("volume", "create", name)
    envfile = Path(tmp) / "env"
    envfile.touch(mode=0o600)

    def launch(mode="serve", state="ding", changed_key=False, mounted=True):
        env = {"PORT": "8080", "DING_PUBLIC_URL": ORIGIN, "DING_MODE": mode,
               "DING_STATE_NAME": state, "RAILWAY_VOLUME_MOUNT_PATH": "/data",
               "DING_DATA_KEY_BASE64": base64.b64encode(secrets.token_bytes(32)).decode() if changed_key else key}
        if mode == "init":
            env.update(DING_BOOTSTRAP_USERNAME="owner", DING_BOOTSTRAP_PASSWORD=password)
        envfile.write_text("\n".join(f"{k}={v}" for k, v in env.items()))
        cid = name + "-" + str(len(containers))
        args = ["run", "-d", "--name", cid, "--env-file", str(envfile), "-p", "127.0.0.1::8080"]
        if mounted:
            args += ["--mount", f"type=volume,source={name},target=/data"]
        docker(*args, IMAGE)
        containers.append(cid)
        port = docker("port", cid, "8080/tcp", check=False).stdout.strip().rsplit(":", 1)[-1]
        return cid, f"http://127.0.0.1:{port}"

    def request(url, path, payload=None, cookie=None, host="cloud.ding.ing"):
        headers = {"Host": host, "X-Forwarded-Proto": "https", "Origin": ORIGIN}
        if cookie:
            headers["Cookie"] = cookie
        data = None
        if payload is not None:
            data = json.dumps(payload).encode()
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url + path, data=data, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=2) as res:
                return res.status, res.read(), res.headers
        except urllib.error.HTTPError as err:
            return err.code, err.read(), err.headers

    def ready(cid, url):
        for _ in range(80):
            try:
                if request(url, "/readyz", host="healthcheck.railway.app")[0] == 200:
                    return
            except (OSError, urllib.error.URLError):
                pass
            time.sleep(0.25)
        raise AssertionError("container did not become ready: " + docker("logs", cid).stderr)

    def rejected(cid, message):
        for _ in range(80):
            if docker("inspect", "-f", "{{.State.Running}}", cid).stdout.strip() == "false":
                assert message in docker("logs", cid).stderr, "wrong startup rejection: " + docker("logs", cid).stderr
                return
            time.sleep(0.25)
        raise AssertionError("unsafe startup was accepted")

    try:
        cid, _ = launch(mounted=False)
        rejected(cid, "persistent volume")
        cid, url = launch("init")
        ready(cid, url)
        proc = docker("exec", cid, "cat", "/proc/1/status").stdout
        assert "Uid:\t10001\t10001\t10001\t10001" in proc
        assert "DING_DATA_KEY_BASE64" not in docker("exec", cid, "/sbin/su-exec", "10001:10001", "cat", "/proc/1/environ").stdout
        mode = docker("exec", cid, "stat", "-c", "%a:%u", "/run/ding/data.key").stdout.strip()
        assert mode == "600:10001", mode
        status, _, headers = request(url, "/v1/auth/password", {"username": "owner", "password": password})
        assert status == 200, status
        cookies = headers.get_all("Set-Cookie")
        assert all("Secure" in c for c in cookies)
        cookie = "; ".join(c.split(";", 1)[0] for c in cookies)
        assert request(url, "/v1/watches", cookie=cookie)[0] == 200
        assert request(url, "/ui/", host="healthcheck.railway.app")[0] == 403
        second, _ = launch()
        rejected(second, "another installation operation")
        recipient = operator(cid, "backup-keygen", "--output", "/run/ding/live-test.identity").stdout.split()[-1]
        live = operator(cid, "backup", "--data-dir", "/data/ding", "--output", "/data/backups/live.age", "--recipient", recipient, check=False)
        assert live.returncode != 0 and "another installation operation" in live.stderr, "live backup was not fenced"
        docker("stop", "-t", "60", cid)
        # Restart the SAME container: runtime files are safely rematerialized,
        # and repeat init must refuse to reset its already-created owner.
        docker("start", cid)
        rejected(cid, "already")
        cid, url = launch()
        ready(cid, url)
        assert request(url, "/v1/watches", cookie=cookie)[0] == 200, "session lost on restart"
        docker("kill", cid)
        cid, url = launch()
        ready(cid, url)
        assert request(url, "/v1/watches", cookie=cookie)[0] == 200, "session lost after crash"
        docker("stop", "-t", "60", cid)
        wrong, _ = launch(changed_key=True)
        rejected(wrong, "data key differs")
        cid, url = launch("maintenance")
        ready(cid, url)
        assert request(url, "/healthz")[0] == 503
        assert request(url, "/v1/watches", cookie=cookie)[0] == 503
        # Simulate a raw Railway volume snapshot. It must go through the portable
        # restore path, which revokes credentials and quarantines execution.
        docker("exec", cid, "/sbin/su-exec", "10001:10001", "cp", "-a", "/data/ding", "/data/raw-snapshot")
        recipient = operator(cid, "backup-keygen", "--output", "/run/ding/backup.identity").stdout.split()[-1]
        operator(cid, "backup", "--data-dir", "/data/raw-snapshot", "--output", "/data/backups/drill.age", "--recipient", recipient)
        archive = Path(tmp) / "drill.age"
        docker("cp", cid + ":/data/backups/drill.age", str(archive))
        assert b"SQLite format" not in archive.read_bytes()
        operator(cid, "restore", "--data-dir", "/data/restored", "--input", "/data/backups/drill.age", "--identity-file", "/run/ding/backup.identity")
        docker("stop", "-t", "60", cid)
        quarantined, _ = launch(state="restored")
        rejected(quarantined, "quarantined")
        cid, url = launch("maintenance", state="restored")
        ready(cid, url)
        operator(cid, "release-restore", "--data-dir", "/data/restored", "--confirm-other-runners-stopped")
        docker("stop", "-t", "60", cid)
        cid, url = launch(state="restored")
        ready(cid, url)
        assert request(url, "/v1/watches", cookie=cookie)[0] == 401, "restored session resurrected"
        assert request(url, "/v1/auth/password", {"username": "owner", "password": password})[0] == 200
        print("PASS: mounted storage, non-root runtime, secret files, owner/session persistence, exclusive writer, crash restart, wrong key refusal, maintenance, encrypted backup, raw snapshot quarantine and revoked restored sessions")
    finally:
        for cid in containers:
            docker("rm", "-f", cid, check=False)
        docker("volume", "rm", name, check=False)
