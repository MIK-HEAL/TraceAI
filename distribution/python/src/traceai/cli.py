from __future__ import annotations

import os
import hashlib
import platform
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from urllib.request import urlopen

from . import __version__

DEFAULT_BASE_URL = "https://github.com/MIK-HEAL/TraceAI/releases/download"


def normalized_platform() -> str:
    system = platform.system().lower()
    if system == "windows":
        return "windows"
    if system == "darwin":
        return "darwin"
    return "linux"


def normalized_arch() -> str:
    machine = platform.machine().lower()
    if machine in {"x86_64", "amd64"}:
        return "amd64"
    if machine in {"aarch64", "arm64"}:
        return "arm64"
    return machine


def asset_name() -> str:
    suffix = ".exe" if os.name == "nt" else ""
    return f"traceai-{normalized_platform()}-{normalized_arch()}{suffix}"


def release_base_url() -> str:
    return os.environ.get("TRACEAI_RELEASE_BASE_URL", DEFAULT_BASE_URL)


def binary_url() -> str:
    override = os.environ.get("TRACEAI_BINARY_URL")
    if override:
        return override
    return f"{release_base_url()}/v{__version__}/{asset_name()}"


def checksum_url() -> str:
    return os.environ.get("TRACEAI_CHECKSUM_URL", f"{release_base_url()}/v{__version__}/SHA256SUMS")


def checksum_for_asset(manifest: str, name: str) -> str:
    for line in manifest.splitlines():
        fields = line.strip().split()
        if len(fields) >= 2 and len(fields[0]) == 64 and fields[1].lstrip("*") == name:
            try:
                int(fields[0], 16)
            except ValueError:
                continue
            return fields[0].lower()
    return ""


def cache_dir() -> Path:
    root = os.environ.get("TRACEAI_CACHE_DIR")
    if root:
        return Path(root)
    home = Path.home() if os.environ.get("HOME") or os.environ.get("USERPROFILE") else Path(tempfile.gettempdir())
    return home / ".traceai"


def binary_path() -> Path:
    suffix = ".exe" if os.name == "nt" else ""
    return cache_dir() / "bin" / __version__ / f"traceai{suffix}"


def ensure_binary() -> Path:
    target = binary_path()
    marker = target.with_suffix(target.suffix + ".sha256")
    if target.exists() and marker.exists():
        digest = hashlib.sha256(target.read_bytes()).hexdigest()
        if digest == marker.read_text(encoding="utf-8").strip().lower():
            return target
        target.unlink(missing_ok=True)

    target.parent.mkdir(parents=True, exist_ok=True)
    temp = target.with_suffix(target.suffix + ".download")
    checksum_temp = target.with_name(target.name + ".download.sha256")
    try:
        with urlopen(binary_url()) as response, temp.open("wb") as handle:
            shutil.copyfileobj(response, handle)
        with urlopen(checksum_url()) as response, checksum_temp.open("wb") as handle:
            shutil.copyfileobj(response, handle)
        expected = checksum_for_asset(checksum_temp.read_text(encoding="utf-8"), asset_name())
        if not expected:
            raise RuntimeError(f"checksum for {asset_name()} not found in SHA256SUMS")
        digest = hashlib.sha256(temp.read_bytes()).hexdigest()
        if digest != expected:
            raise RuntimeError(f"SHA256 mismatch for {asset_name()}")
        temp.replace(target)
        marker.write_text(expected, encoding="utf-8")
    finally:
        temp.unlink(missing_ok=True)
        checksum_temp.unlink(missing_ok=True)
    if os.name != "nt":
        target.parent.mkdir(parents=True, exist_ok=True)
    if os.name != "nt":
        target.chmod(0o755)
    return target


def main(argv: list[str] | None = None) -> int:
    if argv is None:
        argv = sys.argv[1:]
    try:
        binary = ensure_binary()
        completed = subprocess.run([str(binary), *argv], check=False)
        return completed.returncode
    except Exception as exc:  # pragma: no cover - surfaced to user
        print(f"[traceai] {exc}", file=sys.stderr)
        return 1
