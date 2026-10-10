import argparse
import threading
from urllib.parse import urlsplit

import httpx

from ding_mcp.setup import setup_server


def test_pairing_page_requires_host_nonce_and_same_origin(tmp_path):
    args = argparse.Namespace(state=tmp_path / "state", config=tmp_path / "mcp.json")
    server, url, _, _ = setup_server(args)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    parts = urlsplit(url)
    base = f"http://{parts.netloc}"
    try:
        with httpx.Client(base_url=base, trust_env=False) as client:
            page = client.get("/")
            assert page.status_code == 200
            assert parts.fragment not in page.text
            assert page.headers["x-frame-options"] == "DENY"
            assert client.get("/status").status_code == 403
            headers = {"X-Ding-Setup": parts.fragment}
            assert client.get("/status", headers=headers).status_code == 200
            assert (
                client.get("/status", headers={**headers, "Host": "attacker.example"}).status_code
                == 403
            )
            assert (
                client.post(
                    "/pair", headers={**headers, "Origin": "https://attacker.example"}, json={}
                ).status_code
                == 403
            )
            assert client.post("/pair", headers={"Origin": base}, json={}).status_code == 403
            assert not args.config.exists()
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)
