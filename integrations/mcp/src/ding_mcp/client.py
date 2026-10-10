"""Bounded asynchronous client for the daemon's closed integration route set."""

from typing import Any
from urllib.parse import quote

import httpx

from .config import Connection

API_VERSION = "ding.ing/v1alpha1"
MAX_RESPONSE = 16 << 20


class DingError(Exception):
    def __init__(self, code: str, message: str):
        self.code = code
        self.message = message
        super().__init__(f"{code}: {message}")


def segment(value: str) -> str:
    if not value or len(value) > 256 or value in {".", ".."} or any(c in value for c in "/\\\x00"):
        raise DingError("invalid_id", "use an ID returned by Ding")
    return quote(value, safe="")


class DaemonClient:
    def __init__(self, connection: Connection, transport: httpx.AsyncBaseTransport | None = None):
        self.connection = connection
        self.transport = transport

    async def call(self, method: str, path: str, *, params=None, body=None) -> Any:
        if not path.startswith("/") or ".." in path or "?" in path:
            raise ValueError("invalid integration route")
        # No ambient proxy or auth configuration. Redirects must never carry the
        # scoped credential to another origin. A write is never blindly retried.
        async with httpx.AsyncClient(
            timeout=httpx.Timeout(35, connect=5),
            follow_redirects=False,
            trust_env=False,
            transport=self.transport,
            limits=httpx.Limits(max_connections=4),
        ) as client:
            try:
                async with client.stream(
                    method,
                    self.connection.daemon_url + "/v1/integrations" + path,
                    headers={"Authorization": "Bearer " + self.connection.token.get_secret_value()},
                    params=params,
                    json=body,
                ) as response:
                    raw = bytearray()
                    async for chunk in response.aiter_bytes():
                        raw.extend(chunk)
                        if len(raw) > MAX_RESPONSE:
                            raise DingError(
                                "response_too_large", "narrow the query or inspect in Ding Console"
                            )
                    import json

                    try:
                        envelope = json.loads(raw)
                    except (ValueError, UnicodeError):
                        raise DingError(
                            "invalid_response", "Ding returned an invalid API response"
                        ) from None
                    if not isinstance(envelope, dict) or envelope.get("apiVersion") != API_VERSION:
                        raise DingError(
                            "incompatible_daemon", "upgrade Ding to an integration-enabled build"
                        )
                    error = envelope.get("error")
                    if isinstance(error, dict):
                        # Only bounded structured diagnostics; never include URLs,
                        # headers, raw HTTP bodies, or exception representations.
                        raise DingError(
                            str(error.get("code", "daemon_error"))[:100],
                            str(error.get("message", "request failed"))[:2000],
                        )
                    if response.status_code not in range(200, 300) or "data" not in envelope:
                        raise DingError("daemon_error", "Ding could not complete the request")
                    return envelope["data"]
            except httpx.HTTPError:
                if method == "POST" and path != "/preview":
                    raise DingError(
                        "outcome_unknown",
                        "connection interrupted; look up the SAME operation key before retrying; never generate a new key for this attempt",
                    ) from None
                raise DingError(
                    "daemon_unavailable", "start Ding or check the configured connection"
                ) from None
