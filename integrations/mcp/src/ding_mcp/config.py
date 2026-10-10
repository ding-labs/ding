"""Operator configuration. Credentials never enter MCP tool arguments or results."""

import ipaddress
import json
import os
import stat
from pathlib import Path
from urllib.parse import urlsplit

from pydantic import BaseModel, ConfigDict, Field, SecretStr, field_validator


def endpoint(value: str, *, https_only: bool = False) -> str:
    url = urlsplit(value)
    if url.username or url.password or url.query or url.fragment or not url.hostname:
        raise ValueError("endpoint must be an origin without credentials, query, or fragment")
    if url.path not in ("", "/"):
        raise ValueError("endpoint must not include a path")
    try:
        loopback = ipaddress.ip_address(url.hostname).is_loopback
    except ValueError:
        loopback = url.hostname == "localhost"
    if url.scheme != "https" and (https_only or url.scheme != "http" or not loopback):
        raise ValueError("HTTPS is required except for a loopback daemon")
    return value.rstrip("/")


def private_json(path: Path) -> dict:
    # Refuse symlinks and broad POSIX permissions. Windows uses the user's private
    # application-data directory; installers must retain its inherited user ACL.
    flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0)
    with os.fdopen(os.open(path, flags)) as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_size > 1 << 20:
            raise ValueError("configuration must be a regular file smaller than 1 MiB")
        if os.name != "nt" and (info.st_mode & 0o077 or info.st_uid != os.getuid()):
            raise ValueError("configuration must belong to this user with permissions 0600")
        return json.load(stream)


def default_config() -> Path:
    from platformdirs import user_config_path

    return user_config_path("Ding", appauthor=False) / "mcp.json"


class Connection(BaseModel):
    model_config = ConfigDict(extra="forbid")
    daemon_url: str
    token: SecretStr
    grant_id: str = Field(min_length=64, max_length=64)

    @field_validator("daemon_url")
    @classmethod
    def validate_endpoint(cls, value: str) -> str:
        return endpoint(value)

    @field_validator("token")
    @classmethod
    def integration_token(cls, value: SecretStr) -> SecretStr:
        raw = value.get_secret_value()
        if not raw.startswith("ding_mcp_") or len(raw) != 73:
            raise ValueError(
                "a paired integration token is required; admin tokens are not accepted"
            )
        return value

    @classmethod
    def load(cls, path: Path) -> "Connection":
        return cls.model_validate(private_json(path))


class HTTPConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")
    public_url: str
    issuer: str
    jwks_uri: str
    audience: str
    # Subjects are scoped to the single configured issuer. Each maps to a private
    # connection file, and that grant is checked by the daemon on every request.
    subjects: dict[str, Path] = Field(min_length=1)
    algorithm: str = "RS256"

    @field_validator("public_url")
    @classmethod
    def validate_origin(cls, value: str) -> str:
        return endpoint(value, https_only=True)

    @field_validator("issuer", "jwks_uri")
    @classmethod
    def validate_https(cls, value: str) -> str:
        url = urlsplit(value)
        if (
            url.scheme != "https"
            or not url.hostname
            or url.username
            or url.password
            or url.fragment
        ):
            raise ValueError("OAuth endpoints must use HTTPS without embedded credentials")
        return value

    @field_validator("algorithm")
    @classmethod
    def asymmetric_only(cls, value: str) -> str:
        if value not in {"RS256", "RS384", "RS512", "ES256", "ES384", "Ed25519"}:
            raise ValueError("an asymmetric JWT algorithm is required")
        return value

    @field_validator("subjects")
    @classmethod
    def absolute_files(cls, value: dict[str, Path]) -> dict[str, Path]:
        if any(not subject or not path.is_absolute() for subject, path in value.items()):
            raise ValueError(
                "subject bindings require nonempty subjects and absolute credential paths"
            )
        return value
