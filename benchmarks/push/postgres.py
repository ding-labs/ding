#!/usr/bin/env python3
"""Own one private PostgreSQL lab; never print its generated credentials."""
import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess

COMPOSE = Path(__file__).with_name("postgres.compose.yml").resolve()


def private(path, content):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        stream.write(content)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["start", "status", "stop"])
    parser.add_argument("--root", type=Path, required=True)
    args = parser.parse_args()
    root = args.root.expanduser().resolve()
    manifest = root / "lab.json"
    if args.action == "start":
        root.mkdir(mode=0o700, parents=True, exist_ok=True)
        if manifest.exists():
            parser.error("lab already exists; use status, or choose a new private root")
        admin, migrator, app = (secrets.token_hex(24) for _ in range(3))
        project = "ding-pg-" + secrets.token_hex(5)
        private(root / "admin-password", admin)
        private(root / "roles.sql", f"""CREATE ROLE ding_migrator LOGIN PASSWORD '{migrator}';
CREATE ROLE ding_app LOGIN PASSWORD '{app}';
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE ding TO ding_app;
GRANT CONNECT, CREATE, TEMPORARY ON DATABASE ding TO ding_migrator;
""")
        private(manifest, json.dumps({"project": project, "state": "starting"}, indent=2))
    else:
        if not manifest.exists():
            parser.error("no owned lab at this root")
        project = json.loads(manifest.read_text())["project"]
    env = dict(os.environ, DING_PG_PASSWORD_FILE=str(root / "admin-password"))
    command = ["docker", "compose", "-p", project, "-f", str(COMPOSE)]
    if args.action == "start":
        subprocess.run(command + ["up", "-d", "--wait"], env=env, check=True)
        cid = subprocess.check_output(command + ["ps", "-q", "postgres"], env=env, text=True).strip()
        # Keep the host role file private. Linux bind mounts preserve its host
        # UID/mode, which the image's postgres UID cannot read during initdb.
        # Bootstrap the owned ready database through stdin, never argv or logs.
        subprocess.run(["docker", "exec", "-i", cid, "psql", "-U", "postgres",
                        "-d", "ding", "-v", "ON_ERROR_STOP=1"],
                       input=(root / "roles.sql").read_text(), text=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
        info = json.loads(subprocess.check_output(["docker", "inspect", cid], text=True))[0]
        port = info["NetworkSettings"]["Ports"]["5432/tcp"][0]["HostPort"]
        urls = {"app": f"postgres://ding_app:{app}@127.0.0.1:{port}/ding?sslmode=disable",
                "migration": f"postgres://ding_migrator:{migrator}@127.0.0.1:{port}/ding?sslmode=disable",
                "admin": f"postgres://postgres:{admin}@127.0.0.1:{port}/ding?sslmode=disable"}
        private(root / "connections.json", json.dumps(urls, indent=2))
        manifest.write_text(json.dumps({"project": project, "state": "running", "container": cid,
                                       "image": info["Image"], "port": port}, indent=2))
        print(f"Local PostgreSQL lab ready: {project}; private connection file saved.")
    elif args.action == "status":
        subprocess.run(command + ["ps"], env=env, check=True)
    else:
        # Keep the volume and private evidence. Destruction requires an explicit
        # operator action after failed-state evidence has been preserved.
        subprocess.run(command + ["stop"], env=env, check=True)


if __name__ == "__main__":
    main()
