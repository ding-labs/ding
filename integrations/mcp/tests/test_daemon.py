"""Contract tests against an actual daemon and MCP subprocess, not a mock API."""

import json
import os
import subprocess
import sys
import time

import httpx
import pytest
from fastmcp import Client
from fastmcp.client.transports import StdioTransport

from ding_mcp.client import DaemonClient
from ding_mcp.config import Connection
from ding_mcp.server import create_server

MANIFEST = """apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: heartbeat, name: Heartbeat}
spec:
  source: {type: push}
  condition: {missingFor: 5m}
"""


@pytest.fixture
def paired(tmp_path):
    binary = os.environ.get("DING_TEST_BINARY")
    if not binary:
        pytest.skip("set DING_TEST_BINARY to exercise the real daemon (required in CI)")
    state = tmp_path / "state"
    config = tmp_path / "mcp.json"
    with (tmp_path / "daemon.log").open("w") as log:
        process = subprocess.Popen(
            [binary, "daemon", "--state-dir", str(state), "--listen", "127.0.0.1:0"],
            stdout=log,
            stderr=log,
        )
        try:
            deadline = time.monotonic() + 10
            while not (state / "connection.json").exists():
                if process.poll() is not None or time.monotonic() > deadline:
                    pytest.fail(
                        "test daemon did not start: " + (tmp_path / "daemon.log").read_text()
                    )
                time.sleep(0.02)
            result = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "ding_mcp.cli",
                    "pair",
                    "--state",
                    str(state),
                    "--config",
                    str(config),
                    "--manage",
                    "--retry",
                ],
                capture_output=True,
                text=True,
                timeout=15,
            )
            assert result.returncode == 0, result.stderr
            assert "ding_mcp_" not in result.stdout + result.stderr
            yield Connection.load(config), config, state
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()


async def test_real_daemon_contracts_preview_apply_lifecycle_evidence(paired):
    connection, _, state = paired
    async with Client(create_server(connection)) as client:
        cap = await client.call_tool("ding_get_capabilities")
        assert cap.structured_content["data"]["version"] == "ding.integration/v1"
        for name in (
            "ding_list_watches",
            "ding_list_events",
            "ding_list_deliveries",
            "ding_list_destinations",
        ):
            assert not (await client.call_tool(name)).is_error
        preview = await client.call_tool("ding_preview_changes", {"manifest": MANIFEST})
        assert preview.structured_content["data"]["valid"]
        handle = preview.structured_content["data"]["preview"]["handle"]
        args = {"handle": handle, "operation_key": "real_apply_key_000001"}
        applied = await client.call_tool("ding_apply_changes", args)
        assert not applied.structured_content["data"]["dryRun"]
        again = await client.call_tool("ding_apply_changes", args)
        assert applied.structured_content == again.structured_content
        watch = (
            await client.call_tool("ding_get_watch", {"watch_id": "heartbeat"})
        ).structured_content["data"]["watch"]
        events = (
            await client.call_tool("ding_list_events", {"watch": "heartbeat"})
        ).structured_content["data"]["events"]
        assert events
        evidence = await client.call_tool("ding_get_event", {"event_id": events[0]["id"]})
        assert evidence.structured_content["data"]["event"]["id"] == events[0]["id"]
        paused = await client.call_tool(
            "ding_pause_watch",
            {
                "watch_id": "heartbeat",
                "expected_revision": watch["plan"]["revision"],
                "expected_generation": watch["generation"],
                "operation_key": "real_pause_key_000001",
            },
        )
        assert paused.structured_content["data"]["status"] == "paused"
        operation = await client.call_tool(
            "ding_get_operation", {"operation_key": "real_pause_key_000001"}
        )
        assert operation.structured_content["data"]["action"] == "pause"
        bad = await client.call_tool(
            "ding_apply_changes",
            {"handle": handle, "operation_key": "real_pause_key_000001"},
            raise_on_error=False,
        )
        assert bad.is_error and "operation_conflict" in bad.content[0].text
        # Revoke through the actual admin API. The still-connected MCP client must
        # immediately stop reading and must not replay a saved write receipt.
        admin = json.loads((state / "tokens.json").read_text())["admin"]
        async with httpx.AsyncClient(trust_env=False) as http:
            response = await http.delete(
                connection.daemon_url + "/v1/integrations/grants/" + connection.grant_id,
                headers={"Authorization": "Bearer " + admin},
            )
            assert response.status_code == 200
        revoked = await client.call_tool("ding_list_watches", raise_on_error=False)
        assert revoked.is_error


async def test_native_stdio_process_and_resource(paired):
    connection, config, _ = paired
    packaged = os.environ.get("DING_MCP_BINARY")
    command = packaged or sys.executable
    args = ([] if packaged else ["-m", "ding_mcp.cli"]) + ["serve", "--config", str(config)]
    transport = StdioTransport(command=command, args=args, keep_alive=False)
    async with Client(transport) as client:
        assert len(await client.list_tools()) == 15
        result = await client.call_tool("ding_list_watches")
        assert result.structured_content["data"]["watches"] == []
        html = (await client.read_resource("ui://ding/workspace.html"))[0].text
        assert "<title>Ding</title>" in html
        from importlib.resources import files

        assert html == files("ding_mcp").joinpath("assets/workspace.html").read_text()
    # Closing the LLM adapter did not stop the independent watch daemon.
    assert (await DaemonClient(connection).call("GET", "/capabilities"))["instance"]
