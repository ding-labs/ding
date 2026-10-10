import httpx
import pytest

from ding_mcp.client import DaemonClient, DingError, segment
from ding_mcp.config import Connection, HTTPConfig, private_json


def connection():
    return Connection(
        daemon_url="http://127.0.0.1:7676", token="ding_mcp_" + "a" * 64, grant_id="b" * 64
    )


async def test_redirect_never_forwards_credentials():
    calls = []

    def handler(request):
        calls.append(request)
        return httpx.Response(307, headers={"location": "https://attacker.example/"})

    client = DaemonClient(connection(), httpx.MockTransport(handler))
    with pytest.raises(DingError):
        await client.call("GET", "/watches")
    assert len(calls) == 1
    assert calls[0].url.host == "127.0.0.1"


async def test_ambiguous_write_never_retries_or_discloses_credentials():
    calls = 0

    def handler(request):
        nonlocal calls
        calls += 1
        raise httpx.ReadTimeout("sensitive upstream error", request=request)

    client = DaemonClient(connection(), httpx.MockTransport(handler))
    with pytest.raises(DingError, match="outcome_unknown") as error:
        await client.call("POST", "/apply", body={"operationKey": "same_key_000000001"})
    assert calls == 1
    assert "sensitive" not in str(error.value)
    assert "ding_mcp_" not in str(error.value)


@pytest.mark.parametrize("value", ["..", "/v1/backup", "a/b", "a\\b", "\x00", ""])
def test_path_segments_cannot_escape_route(value):
    with pytest.raises(DingError):
        segment(value)


@pytest.mark.parametrize(
    "url",
    [
        "http://remote.example",
        "https://user:secret@example.com",
        "file:///tmp/ding",
        "https://example.com/other",
        "https://example.com?token=secret",
    ],
)
def test_connection_rejects_unsafe_endpoints(url):
    with pytest.raises(ValueError):
        connection().model_copy(update={})  # baseline remains a valid model
        Connection(daemon_url=url, token="ding_mcp_" + "a" * 64, grant_id="b" * 64)


def test_http_requires_explicit_identity_bindings():
    with pytest.raises(ValueError):
        HTTPConfig(
            public_url="https://ding.example",
            issuer="https://auth.example",
            jwks_uri="https://auth.example/jwks",
            audience="ding",
            subjects={},
        )


def test_private_configuration_permissions(tmp_path):
    import os

    path = tmp_path / "config.json"
    path.write_text("{}")
    if os.name != "nt":
        path.chmod(0o644)
        with pytest.raises(ValueError):
            private_json(path)
    path.chmod(0o600)
    assert private_json(path) == {}
