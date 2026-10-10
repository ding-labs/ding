#!/usr/bin/env python3
"""Assemble native Claude or fixed-endpoint remote plugin qualification packages."""

import argparse
import hashlib
import json
import os
import plistlib
import shutil
import stat
import zipfile
from pathlib import Path
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n")


def package(args):
    platform = "chatgpt" if args.mode == "remote-chatgpt" else "claude"
    if args.mode == "native-claude":
        if not args.runtime or not args.target:
            raise ValueError("native bundles require --runtime and --target")
        executable = (
            "ding-mcp.exe" if args.target.startswith("windows-") else "ding-mcp"
        )
        if (
            not (args.runtime / executable).is_file()
            or not (args.runtime / "_internal").is_dir()
        ):
            raise ValueError("runtime must be the complete PyInstaller onedir output")
    else:
        url = urlsplit(args.endpoint or "")
        if (
            url.scheme != "https"
            or not url.hostname
            or url.username
            or url.password
            or url.query
            or url.fragment
            or url.path != "/mcp"
        ):
            raise ValueError("supply a credential-free HTTPS endpoint ending in /mcp")
    # Refuse to overwrite artifacts; this also makes interrupted builds visible.
    args.output.mkdir(parents=True, exist_ok=False)
    dest = args.output / "ding"
    shutil.copytree(ROOT / "plugins" / platform / "ding", dest)
    shutil.copytree(ROOT / "plugins/shared/skills", dest / "skills")
    shutil.copytree(ROOT / "plugins/shared/assets", dest / "assets")
    shutil.copyfile(ROOT / "LICENSE", dest / "LICENSE")
    if args.mode == "native-claude":
        shutil.copytree(args.runtime, dest / "runtime/ding-mcp", symlinks=True)
        config = {
            "mcpServers": {
                "ding": {
                    "command": "${CLAUDE_PLUGIN_ROOT}/runtime/ding-mcp/" + executable,
                    "args": ["serve"],
                }
            }
        }
        write_json(dest / ".mcp.json", config)
        if args.target.startswith("darwin-"):
            contents = dest / "Ding Setup.app/Contents"
            (contents / "MacOS").mkdir(parents=True)
            (contents / "Info.plist").write_bytes(
                plistlib.dumps(
                    {
                        "CFBundleName": "Ding Setup",
                        "CFBundleIdentifier": "ing.ding.mcp.setup",
                        "CFBundleVersion": "0.1.0",
                        "CFBundleExecutable": "setup",
                        "CFBundlePackageType": "APPL",
                    }
                )
            )
            launcher = contents / "MacOS/setup"
            launcher.write_text(
                '#!/bin/sh\nHERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"\nexec "$HERE/../../../runtime/ding-mcp/ding-mcp" setup\n'
            )
            launcher.chmod(0o755)
        elif args.target.startswith("windows-"):
            (dest / "Ding Setup.cmd").write_text(
                '@echo off\r\n"%~dp0runtime\\ding-mcp\\ding-mcp.exe" setup\r\n'
            )
        else:
            launcher = dest / "Ding Setup.sh"
            launcher.write_text(
                '#!/bin/sh\nHERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"\nexec "$HERE/runtime/ding-mcp/ding-mcp" setup\n'
            )
            launcher.chmod(0o755)
        readme = "Open Ding Setup to pair with an existing running Ding daemon, then enable/restart this plugin in local Claude Cowork or Claude Code. This native package does not run inside ordinary Claude chat. No Python or Node installation is needed."
    else:
        server = {
            "type": "streamable-http" if platform == "chatgpt" else "http",
            "url": args.endpoint,
        }
        if platform == "chatgpt":
            server["extensions"] = {
                "com.openai": {
                    "auth": {
                        "type": "oauth",
                        "baseScopes": [
                            "ding:inspect",
                            "ding:preview",
                            "ding:manage",
                            "ding:retry",
                        ],
                    }
                }
            }
        config = {"mcpServers": {"ding": server}}
        if platform == "chatgpt":
            config["$schema"] = (
                "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
            )
        write_json(
            dest / ("mcp.json" if platform == "chatgpt" else ".mcp.json"), config
        )
        readme = "Connect through your host's OAuth flow. This package targets one operator-supplied, self-hosted endpoint. An operator must pair your identity to a Ding grant. It does not route arbitrary customer endpoints or expose localhost."
    (dest / "README.md").write_text(
        "# Ding\n\n"
        + readme
        + "\n\nThis is a qualification build, not evidence of official marketplace approval.\n\nAuthoritative data remains on your Ding instance; data you request is sent to your selected LLM provider. See the included privacy and review documents.\n"
    )
    for name in ("privacy.md", "release.md", "verification.md"):
        shutil.copyfile(ROOT / "docs/integrations" / name, dest / name)
    archive = args.output / "ding.zip"
    # Preserve native permissions and PyInstaller framework symlinks in the ZIP.
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as z:
        for path in sorted(dest.rglob("*")):
            if path.is_symlink():
                info = zipfile.ZipInfo(str(path.relative_to(dest)).replace(os.sep, "/"))
                info.create_system = 3
                info.external_attr = (stat.S_IFLNK | 0o777) << 16
                z.writestr(info, os.readlink(path))
            elif path.is_file():
                z.write(path, path.relative_to(dest))
    digest = hashlib.file_digest(archive.open("rb"), "sha256").hexdigest()
    (args.output / "SHA256SUMS").write_text(digest + "  ding.zip\n")
    write_json(
        args.output / "qualification.json",
        {
            "mode": args.mode,
            "target": args.target,
            "sha256": digest,
            "signed": False,
            "marketplaceApproved": False,
            "containsRuntime": args.mode == "native-claude",
        },
    )
    print(
        f"Built {archive}\nSHA-256 {digest}\nQualification only; review release.md before submission."
    )


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument(
        "--mode",
        required=True,
        choices=["native-claude", "remote-claude", "remote-chatgpt"],
    )
    p.add_argument("--runtime", type=Path)
    p.add_argument(
        "--target",
        choices=[
            "darwin-arm64",
            "darwin-amd64",
            "linux-amd64",
            "linux-arm64",
            "windows-amd64",
            "windows-arm64",
        ],
    )
    p.add_argument("--endpoint")
    p.add_argument("--output", type=Path, required=True)
    package(p.parse_args())
