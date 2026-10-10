#!/usr/bin/env python3
"""Build a native, dependency-complete adapter for the current OS and architecture."""

import os
import platform
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
arch = {"aarch64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}.get(
    platform.machine(), platform.machine()
)
target = (
    {"Darwin": "darwin", "Linux": "linux", "Windows": "windows"}[platform.system()]
    + "-"
    + arch
)
output = ROOT / "dist/integrations" / target
subprocess.run(
    [
        "npm.cmd" if os.name == "nt" else "npm",
        "run",
        "build",
        "--prefix",
        str(ROOT / "web/mcp-app"),
    ],
    check=True,
)
env = {
    **os.environ,
    "PYINSTALLER_CONFIG_DIR": str(ROOT / "integrations/mcp/build/pyinstaller-cache"),
}
subprocess.run(
    [
        sys.executable,
        "-m",
        "PyInstaller",
        "--noconfirm",
        "--onedir",
        "--name",
        "ding-mcp",
        "--distpath",
        str(output),
        "--workpath",
        str(ROOT / "integrations/mcp/build/work"),
        "--specpath",
        str(ROOT / "integrations/mcp/build"),
        "--collect-all",
        "fastmcp",
        "--collect-submodules",
        "mcp.server",
        "--collect-submodules",
        "mcp.client",
        "--collect-submodules",
        "mcp.shared",
        "--collect-all",
        "mcp_types",
        "--collect-data",
        "ding_mcp",
        "--recursive-copy-metadata",
        "fastmcp",
        "--copy-metadata",
        "fastmcp-slim",
        "--copy-metadata",
        "ding-mcp",
        str(ROOT / "integrations/mcp/entrypoint.py"),
    ],
    cwd=ROOT,
    env=env,
    check=True,
)
executable = output / "ding-mcp" / ("ding-mcp.exe" if os.name == "nt" else "ding-mcp")
subprocess.run([str(executable), "--version"], check=True)
print(
    f"Native runtime: {executable}\nUnsigned qualification build; signing and host review are separate release gates."
)
