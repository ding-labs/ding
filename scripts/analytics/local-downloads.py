"""Collect only public package counts and official bottle-builder fetch receipts."""
import datetime as dt
import hashlib
import hmac
import json
import os
import re
import subprocess
import sys
import time
import urllib.request

REPO = "repos/ding-labs/ding"
TAP = "repos/ding-labs/homebrew-tap"
BREW = "https://formulae.brew.sh/api/analytics/install-on-request/30d.json"
DESTINATION = "https://ding.ing/_funnel/downloads"


def pages(path):
    output = subprocess.check_output(["gh", "api", "--paginate", "--slurp", path])
    return json.loads(output)


def epoch(timestamp):
    return int(dt.datetime.fromisoformat(timestamp.replace("Z", "+00:00")).timestamp())


def homebrew():
    with urllib.request.urlopen(BREW, timeout=40) as response:
        report = json.load(response)
    items = report["items"]
    if report["category"] != "formula_install_on_request" or not items:
        raise ValueError("unexpected Homebrew report")
    start = dt.date.fromisoformat(report["start_date"])
    end = dt.date.fromisoformat(report["end_date"])
    if not 28 <= (end - start).days <= 31 or not any("/" in row["formula"] for row in items):
        raise ValueError("expected the published 30-day report including third-party taps")
    count = sum(int(row["count"].replace(",", "")) for row in items
                if row["formula"].split()[0] == "ding-labs/tap/ding")
    return {"installs": count, "start_date": start.isoformat(), "end_date": end.isoformat()}


def release_assets():
    result = []
    for page in pages(f"{REPO}/releases?per_page=100"):
        for release in page:
            if release["draft"]:
                continue
            assets = release["assets"]
            if len(assets) >= 30:
                assets = [a for p in pages(f"{REPO}/releases/{release['id']}/assets?per_page=100") for a in p]
            for asset in assets:
                name = asset["name"]
                if asset["state"] != "uploaded":
                    continue
                if re.fullmatch(r"ding_(darwin|linux|windows)_(amd64|arm64)\.(tar\.gz|zip)", name):
                    kind = "archive"
                elif re.fullmatch(r"ding-[\w.-]+\.bottle\.tar\.gz", name):
                    kind = "bottle"
                else:
                    continue
                result.append({"id": asset["id"], "tag": release["tag_name"], "name": name,
                               "kind": kind, "created": epoch(asset["created_at"]), "count": asset["download_count"]})
    if not result:
        raise ValueError("no Ding release assets found")
    return result


def confirmed_fetches():
    since = dt.datetime.now(dt.timezone.utc).date() - dt.timedelta(days=35)
    receipts = []
    runs_path = f"{TAP}/actions/workflows/bottles.yml/runs?created=%3E%3D{since}&per_page=100"
    for page in pages(runs_path):
        if page["total_count"] >= 1000:
            raise ValueError("bottle runs exceed the GitHub search window")
        for run in page["workflow_runs"]:
            for jobs in pages(f"{TAP}/actions/runs/{run['id']}/jobs?filter=all&per_page=100"):
                for job in jobs["jobs"]:
                    for step in job["steps"]:
                        match = re.fullmatch(r"Fetch Ding release archive (v\d+\.\d+\.\d+) \((arm64|x86_64)\)", step["name"])
                        if match and step["conclusion"] == "success":
                            arch = {"arm64": "arm64", "x86_64": "amd64"}[match[2]]
                            receipts.append({"id": job["id"], "tag": match[1], "name": f"ding_darwin_{arch}.tar.gz",
                                             "fetched": epoch(step["completed_at"])})
    return receipts


def publish(reading):
    body = json.dumps(reading, separators=(",", ":")).encode()
    timestamp = str(int(time.time()*1000))
    digest = hmac.new(os.environ["DOWNLOAD_REPORT_SECRET"].encode(), timestamp.encode()+b"."+body, hashlib.sha256)
    request = urllib.request.Request(DESTINATION, data=body, headers={"Content-Type": "application/json",
        "User-Agent": "Ding download analytics/1.0", "Ding-Signature": f"t={timestamp}, v1={digest.hexdigest()}"})
    with urllib.request.urlopen(request, timeout=30) as response:
        if response.status != 204:
            raise RuntimeError(f"unexpected report response {response.status}")


if __name__ == "__main__":
    source = sys.argv[1]
    observed = int(time.time())
    if source == "github":
        reading = {"assets": release_assets(), "fetches": confirmed_fetches()}
    elif source == "homebrew":
        reading = homebrew()
    else:
        raise ValueError("unknown source")
    reading.update(source=source, observed=observed)
    if "--check" in sys.argv[2:]:
        print({k: len(v) if isinstance(v,list) else v for k,v in reading.items()})
    else:
        publish(reading)
        print(f"Published {source} reading.")
