"""Pair once locally; run packaged MCP without a Python installation or API key."""

import argparse
import asyncio
import json
import os
import sys
from pathlib import Path

import httpx

from . import __version__
from .client import DaemonClient, DingError
from .config import Connection, HTTPConfig, default_config, endpoint, private_json


async def admin_call(state: Path, method: str, path: str, body=None):
    connection = private_json(state / "connection.json")
    tokens = private_json(state / "tokens.json")
    url = endpoint(connection["url"])
    async with httpx.AsyncClient(timeout=15, follow_redirects=False, trust_env=False) as client:
        response = await client.request(
            method,
            url + "/v1/integrations/grants" + path,
            headers={"Authorization": "Bearer " + tokens["admin"]},
            json=body,
        )
        response.raise_for_status()
        envelope = response.json()
        if envelope.get("error"):
            raise ValueError("Ding could not complete the pairing operation")
        return url, envelope["data"]


async def pair(args):
    path = args.config
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    # Reserve the file before provisioning. Never silently replace an active grant.
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    grant = None
    try:
        scopes = ["inspect", "preview"]
        if args.manage:
            scopes.append("manage")
        if args.retry:
            scopes.append("retry")
        url, grant = await admin_call(
            args.state,
            "POST",
            "",
            {
                "name": args.name,
                "scopes": scopes,
                "days": args.days,
                "commandRevisions": args.allow_command_revision,
                "secretRefs": args.allow_secret_ref,
            },
        )
        connection = {"daemon_url": url, "token": grant["token"], "grant_id": grant["grant"]["id"]}
        with os.fdopen(fd, "w") as stream:
            fd = None
            json.dump(connection, stream, indent=2)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        print(
            f"Paired {args.name}. Permissions: {', '.join(scopes)}. Expires {grant['grant']['expiresAt']}."
        )
        print(f"Private connection saved to {path}. The credential was not printed.")
    except BaseException:
        if fd is not None:
            os.close(fd)
        path.unlink(missing_ok=True)
        if grant:
            try:
                await admin_call(args.state, "DELETE", "/" + grant["grant"]["id"])
            except Exception:
                print(
                    "Pairing failed after grant creation; inspect local grants and revoke the unused grant.",
                    file=sys.stderr,
                )
        raise


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(
        prog="ding-mcp", description="Ding's local and self-hosted MCP integration"
    )
    root.add_argument("--version", action="version", version=__version__)
    sub = root.add_subparsers(dest="command", required=True)
    from .setup import default_state

    p = sub.add_parser("setup", help="open the guided local pairing window")
    p.add_argument("--state", type=Path, default=default_state())
    p.add_argument("--config", type=Path, default=default_config())
    p = sub.add_parser("pair", help="create a scoped grant using local administrator access")
    p.add_argument("--state", type=Path, required=True, help="Ding daemon state directory")
    p.add_argument("--config", type=Path, default=default_config())
    p.add_argument("--name", default="Ding MCP")
    p.add_argument("--days", type=int, choices=range(1, 366), default=90, metavar="1..365")
    p.add_argument("--manage", action="store_true", help="allow applying and changing watches")
    p.add_argument("--retry", action="store_true", help="allow retrying failed notifications")
    p.add_argument(
        "--allow-secret-ref",
        action="append",
        default=[],
        help="allow manifests to reference this daemon environment variable; never pass its value",
    )
    p.add_argument(
        "--allow-command-revision",
        action="append",
        default=[],
        help="explicitly allow this exact compiled command watch revision",
    )
    p = sub.add_parser("grants", help="list local grants or revoke one")
    p.add_argument("--state", type=Path, required=True)
    p.add_argument("--revoke", help="grant ID to revoke; existing adapter sessions lose access")
    p = sub.add_parser("doctor", help="verify a paired connection without exposing its credential")
    p.add_argument("--config", type=Path, default=default_config())
    p = sub.add_parser("serve", help="run stdio by default, or explicitly authenticated HTTP")
    p.add_argument("--config", type=Path, default=default_config())
    p.add_argument("--transport", choices=["stdio", "http"], default="stdio")
    p.add_argument(
        "--http-config", type=Path, help="required private OAuth/subject configuration for HTTP"
    )
    p.add_argument("--host", default="127.0.0.1")
    p.add_argument("--port", type=int, default=7677)
    return root


def main():
    args = parser().parse_args()
    try:
        if args.command == "setup":
            from .setup import run_setup

            run_setup(args)
        elif args.command == "pair":
            asyncio.run(pair(args))
        elif args.command == "grants":
            if args.revoke:
                from .client import segment

                asyncio.run(admin_call(args.state, "DELETE", "/" + segment(args.revoke)))
                print("Grant revoked.")
            else:
                _, grants = asyncio.run(admin_call(args.state, "GET", ""))
                print(json.dumps(grants, indent=2))
        elif args.command == "doctor":
            data = asyncio.run(
                DaemonClient(Connection.load(args.config)).call("GET", "/capabilities")
            )
            print(json.dumps(data, indent=2))
        else:
            from .server import create_server

            if args.transport == "stdio":
                if args.http_config:
                    raise ValueError("--http-config requires --transport http")
                create_server(Connection.load(args.config)).run(
                    transport="stdio", show_banner=False
                )
            else:
                if not args.http_config:
                    raise ValueError(
                        "HTTP requires --http-config with OAuth and explicit subject bindings"
                    )
                http = HTTPConfig.model_validate(private_json(args.http_config))
                # Fail at startup for missing files; re-read per call for rotation.
                for path in http.subjects.values():
                    Connection.load(path)
                from urllib.parse import urlsplit

                create_server(http=http).run(
                    transport="http",
                    host=args.host,
                    port=args.port,
                    path="/mcp",
                    show_banner=False,
                    stateless_http=True,
                    host_origin_protection=True,
                    allowed_hosts=[urlsplit(http.public_url).hostname],
                    uvicorn_config={"access_log": False},
                )
    except (OSError, ValueError, KeyError, DingError, httpx.HTTPError) as exc:
        # Pydantic and HTTP exception reprs can include credentials. Keep CLI errors
        # actionable but never emit the exception, request headers, or input values.
        if isinstance(exc, FileExistsError):
            message = "configuration already exists; use it or explicitly revoke its grant before re-pairing"
        elif isinstance(exc, FileNotFoundError):
            message = "connection/configuration file missing; start Ding and run ding-mcp pair"
        elif isinstance(exc, DingError):
            message = str(exc)
        else:
            message = "could not complete the command; verify private configuration, daemon availability, and required options"
        print("ding-mcp: " + message, file=sys.stderr)
        raise SystemExit(1) from None


if __name__ == "__main__":
    main()
