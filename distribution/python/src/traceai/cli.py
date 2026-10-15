from __future__ import annotations

import os
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
    if target.exists():
        return target

    target.parent.mkdir(parents=True, exist_ok=True)
    temp = target.with_suffix(target.suffix + ".download")
    with urlopen(binary_url()) as response, temp.open("wb") as handle:
        shutil.copyfileobj(response, handle)
    if os.name != "nt":
        temp.chmod(0o755)
    temp.replace(target)
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
