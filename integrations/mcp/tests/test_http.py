import json
import time

import httpx
import jwt
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives import serialization
from fastmcp.server.auth.providers.jwt import JWTVerifier

import ding_mcp.server as module
from ding_mcp.client import DaemonClient
from ding_mcp.config import HTTPConfig


async def test_http_oauth_boundary_and_subject_isolation(tmp_path, monkeypatch):
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    public = key.public_key().public_bytes(
        serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo
    )
    issuer = "https://auth.example"
    audience = "https://ding.example/mcp"
    monkeypatch.setattr(
        module,
        "JWTVerifier",
        lambda **_: JWTVerifier(
            public_key=public, issuer=issuer, audience=audience, required_scopes=["ding:inspect"]
        ),
    )
    paths = {}
    for subject, suffix in (("alice", "a"), ("bob", "b")):
        path = tmp_path / f"{subject}.json"
        path.write_text(
            json.dumps(
                {
                    "daemon_url": "http://127.0.0.1:7676",
                    "token": "ding_mcp_" + suffix * 64,
                    "grant_id": suffix * 64,
                }
            )
        )
        path.chmod(0o600)
        paths[subject] = path
    seen = []

    def daemon(request):
        seen.append(request.headers["authorization"])
        return httpx.Response(
            200,
            json={
                "apiVersion": "ding.ing/v1alpha1",
                "data": {"watches": [], "total": 0, "cursor": "", "more": False},
            },
        )

    monkeypatch.setattr(
        module, "DaemonClient", lambda config: DaemonClient(config, httpx.MockTransport(daemon))
    )
    config = HTTPConfig(
        public_url="https://ding.example",
        issuer=issuer,
        jwks_uri=issuer + "/jwks",
        audience=audience,
        subjects=paths,
    )
    server = module.create_server(http=config)
    app = server.http_app(path="/mcp", stateless_http=True, json_response=True)

    def token(subject="alice", aud=audience, scope="ding:inspect", expired=False):
        return jwt.encode(
            {
                "iss": issuer,
                "sub": subject,
                "aud": aud,
                "iat": int(time.time()) - 10,
                "exp": int(time.time()) + (-60 if expired else 300),
                "scope": scope,
                "client_id": "qualification-client",
            },
            key,
            algorithm="RS256",
        )

    headers = {
        "Accept": "application/json, text/event-stream",
        "MCP-Protocol-Version": "2026-07-28",
        "MCP-Method": "tools/call",
        "MCP-Name": "ding_list_watches",
    }
    meta = {
        "io.modelcontextprotocol/protocolVersion": "2026-07-28",
        "io.modelcontextprotocol/clientCapabilities": {},
    }
    request = {
        "jsonrpc": "2.0",
        "id": "test",
        "method": "tools/call",
        "params": {"name": "ding_list_watches", "arguments": {}, "_meta": meta},
    }
    async with app.router.lifespan_context(app):
        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app), base_url="https://ding.example"
        ) as client:
            assert (await client.post("/mcp", json=request, headers=headers)).status_code == 401
            for invalid in (token(aud="wrong"), token(expired=True), token(scope="ding:manage")):
                assert (
                    await client.post(
                        "/mcp",
                        json=request,
                        headers={**headers, "Authorization": "Bearer " + invalid},
                    )
                ).status_code == 401
            for subject in ("alice", "bob"):
                response = await client.post(
                    "/mcp",
                    json=request,
                    headers={**headers, "Authorization": "Bearer " + token(subject)},
                )
                assert response.status_code == 200, response.text
                assert not response.json()["result"].get("isError"), response.text
            assert seen == ["Bearer ding_mcp_" + "a" * 64, "Bearer ding_mcp_" + "b" * 64]
            response = await client.post(
                "/mcp",
                json=request,
                headers={**headers, "Authorization": "Bearer " + token("unpaired")},
            )
            assert response.json()["result"]["isError"]
            assert len(seen) == 2
            write = {
                **request,
                "params": {
                    "_meta": meta,
                    "name": "ding_apply_changes",
                    "arguments": {"handle": "a" * 64, "operation_key": "oauth_write_000001"},
                },
            }
            response = await client.post(
                "/mcp",
                json=write,
                headers={
                    **headers,
                    "MCP-Name": "ding_apply_changes",
                    "Authorization": "Bearer " + token(),
                },
            )
            assert response.json()["result"]["isError"]
            assert len(seen) == 2
            metadata = await client.get("/.well-known/oauth-protected-resource/mcp")
            assert metadata.status_code == 200
            assert metadata.json()["resource"] == audience
            legacy = {**request, "params": {"name": "ding_list_watches", "arguments": {}}}
            response = await client.post(
                "/mcp",
                json=legacy,
                headers={
                    "Accept": "application/json, text/event-stream",
                    "MCP-Protocol-Version": "2025-11-25",
                    "Authorization": "Bearer " + token(),
                },
            )
            assert response.status_code == 200, response.text
            assert not response.json()["result"].get("isError"), response.text
