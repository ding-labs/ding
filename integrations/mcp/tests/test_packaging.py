import json
import subprocess
import sys
import zipfile
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[3]


@pytest.mark.parametrize(
    "mode,manifest,config",
    [
        ("remote-chatgpt", "plugin.json", "mcp.json"),
        ("remote-claude", ".claude-plugin/plugin.json", ".mcp.json"),
    ],
)
def test_remote_packages_include_mcp_configuration_and_workflows(tmp_path, mode, manifest, config):
    output = tmp_path / "package"
    result = subprocess.run(
        [
            sys.executable,
            str(ROOT / "scripts/package-integrations.py"),
            "--mode",
            mode,
            "--endpoint",
            "https://qualification.example/mcp",
            "--output",
            str(output),
        ],
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stderr
    with zipfile.ZipFile(output / "ding.zip") as archive:
        assert json.loads(archive.read(manifest))["name"] == "ding"
        assert (
            json.loads(archive.read(config))["mcpServers"]["ding"]["url"]
            == "https://qualification.example/mcp"
        )
        assert "skills/ding-watch/SKILL.md" in archive.namelist()
        assert "skills/ding-watch/references/manifest.md" in archive.namelist()
        assert "skills/ding-setup/SKILL.md" in archive.namelist()
        assert not any(name.startswith(("bin/", "hooks/")) for name in archive.namelist())
    assert not json.loads((output / "qualification.json").read_text())["marketplaceApproved"]


@pytest.mark.parametrize(
    "endpoint",
    [
        "http://localhost:7677/mcp",
        "https://user:secret@example.com/mcp",
        "https://example.com/mcp?token=secret",
    ],
)
def test_packager_refuses_unsafe_endpoint_before_creating_files(tmp_path, endpoint):
    output = tmp_path / "package"
    result = subprocess.run(
        [
            sys.executable,
            str(ROOT / "scripts/package-integrations.py"),
            "--mode",
            "remote-chatgpt",
            "--endpoint",
            endpoint,
            "--output",
            str(output),
        ],
        capture_output=True,
        text=True,
    )
    assert result.returncode != 0
    assert not output.exists()
