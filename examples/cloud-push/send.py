#!/usr/bin/env python3
"""Send an existing immutable request file. Reuse both file and key after failure."""
import json
import os
import random
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

url, key, path = sys.argv[1:]
if urllib.parse.urlsplit(url).scheme != "https":
    raise SystemExit("Use an HTTPS ingest URL")
with open(path, "rb") as source:
    body = source.read(1048577)
if len(body) > 1048576:
    raise SystemExit("Body exceeds 1 MiB")
headers = {"Authorization": "Bearer " + os.environ["DING_PRODUCER_TOKEN"],
           "Content-Type": "application/json", "Idempotency-Key": key}
opener = urllib.request.build_opener(NoRedirect())
for attempt in range(5):
    delay = 2 ** attempt
    try:
        with opener.open(urllib.request.Request(url, body, headers, method="POST"), timeout=15) as response:
            if response.status != 202:
                raise SystemExit("Unexpected response; retain file and key for reconciliation")
            print(json.dumps(json.load(response)))
            break
    except urllib.error.HTTPError as error:
        if error.code not in (429, 503):
            raise SystemExit(f"HTTP {error.code}; request {error.headers.get('X-Ding-Request-ID', 'unknown')}")
        retry = error.headers.get("Retry-After", "")
        if retry.isdigit():
            delay = max(delay, min(int(retry), 30))
    except (urllib.error.URLError, TimeoutError):
        pass
    if attempt == 4:
        raise SystemExit("Acceptance unconfirmed: retain this exact file and key")
    time.sleep(delay + random.random())
