"""Short-lived, loopback-only pairing UI launched by the native setup shortcut."""

import argparse
import asyncio
import hmac
import json
import os
import secrets
import sys
import threading
import time
import webbrowser
from http.server import BaseHTTPRequestHandler, HTTPServer
from importlib.resources import files
from pathlib import Path


def default_state() -> Path:
    if sys.platform == "darwin":
        return Path.home() / "Library/Application Support/ding/watch"
    if os.name == "nt":
        return Path(os.environ["APPDATA"]) / "ding/watch"
    return Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config")) / "ding/watch"


def setup_server(args):
    """Return a server and launch URL; separated so the security boundary is testable."""
    from .cli import pair

    nonce = secrets.token_urlsafe(32)
    deadline = time.monotonic() + 600
    completed = threading.Event()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def send(self, status, body, content_type="application/json"):
            data = body.encode() if isinstance(body, str) else json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Cache-Control", "no-store")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("X-Frame-Options", "DENY")
            self.send_header("Referrer-Policy", "no-referrer")
            self.send_header(
                "Content-Security-Policy",
                "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
            )
            self.end_headers()
            self.wfile.write(data)

        def host_ok(self):
            return self.headers.get("Host") == f"127.0.0.1:{self.server.server_port}"

        def authorized(self):
            return (
                self.host_ok()
                and time.monotonic() < deadline
                and hmac.compare_digest(self.headers.get("X-Ding-Setup", ""), nonce)
            )

        def do_GET(self):
            if not self.host_ok():
                return self.send(403, {"error": "Unrecognized setup host"})
            if self.path == "/":
                return self.send(
                    200,
                    files("ding_mcp").joinpath("assets/setup.html").read_text(),
                    "text/html; charset=utf-8",
                )
            if self.path == "/status" and self.authorized():
                return self.send(200, {"state": str(args.state), "paired": args.config.exists()})
            self.send(403, {"error": "Open the setup window from Ding again."})

        def do_POST(self):
            origin = f"http://127.0.0.1:{self.server.server_port}"
            if (
                not self.authorized()
                or self.headers.get("Origin") != origin
                or self.path != "/pair"
                or completed.is_set()
            ):
                return self.send(
                    403, {"error": "This setup session is unavailable. Open Ding Setup again."}
                )
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= 8192 or self.headers.get_content_type() != "application/json":
                    raise ValueError("invalid request")
                payload = json.loads(self.rfile.read(length))
                if not isinstance(payload, dict) or set(payload) - {"state", "manage", "retry"}:
                    raise ValueError("invalid request")
                if not isinstance(payload.get("manage", False), bool) or not isinstance(
                    payload.get("retry", False), bool
                ):
                    raise ValueError("invalid permissions")
                state = Path(payload.get("state", str(args.state))).expanduser()
                if not state.is_absolute():
                    raise ValueError("absolute state directory required")
                request = argparse.Namespace(
                    state=state,
                    config=args.config,
                    name="Ding desktop plugin",
                    days=90,
                    manage=payload.get("manage", False),
                    retry=payload.get("retry", False),
                    allow_command_revision=[],
                    allow_secret_ref=[],
                )
                asyncio.run(pair(request))
                completed.set()
                self.send(200, {"paired": True})
            except Exception:
                self.send(
                    400,
                    {
                        "error": "Could not connect. Start the Ding daemon, check its state directory, and use a new connection file if already paired."
                    },
                )

    server = HTTPServer(("127.0.0.1", 0), Handler)
    server.timeout = 0.5
    return server, f"http://127.0.0.1:{server.server_port}/#{nonce}", completed, deadline


def run_setup(args):
    server, url, completed, deadline = setup_server(args)
    print(
        "Opening Ding Setup in your browser. This setup window expires in 10 minutes.",
        file=sys.stderr,
    )
    if not webbrowser.open(url):
        print("Open this private setup link locally: " + url, file=sys.stderr)
    try:
        while not completed.is_set() and time.monotonic() < deadline:
            server.handle_request()
    finally:
        server.server_close()
