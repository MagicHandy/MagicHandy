"""Select the download backend for an app-owned Hugging Face voice environment."""

import os
import subprocess
import sys
import tempfile
from importlib import metadata
from pathlib import Path


def probe_current_python():
    from hf_xet import PyXetDownloadInfo, download_files
    from huggingface_hub.utils._runtime import is_xet_available
    from packaging.requirements import Requirement
    from packaging.utils import canonicalize_name

    if not (callable(PyXetDownloadInfo) and callable(download_files) and is_xet_available()):
        return False
    version = metadata.version("hf-xet")
    for item in metadata.requires("huggingface-hub") or ():
        requirement = Requirement(item)
        if canonicalize_name(requirement.name) == "hf-xet" and (
            requirement.marker is None or requirement.marker.evaluate({"extra": "hf-xet"})
        ):
            if not requirement.specifier.contains(version, prereleases=True):
                return False
    return True


def xet_usable():
    # Hub caches package availability at import time. A fresh interpreter also
    # isolates a broken native wheel and verifies the entry points Hub uses.
    # Some wheels create a log on import. A read-only module check must not
    # write into either the installed runtime or the user's global Hub cache.
    with tempfile.TemporaryDirectory(prefix="magichandy-xet-probe-") as cache:
        return subprocess.run(
            [sys.executable, "-I", "-B", __file__, "--probe"],
            env={**os.environ, "HF_XET_CACHE": cache, "HF_XET_LOG_DEST": os.devnull},
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        ).returncode == 0


def select_backend(uv=None):
    if os.environ.get("HF_HUB_DISABLE_XET", "").upper() in {"1", "ON", "YES", "TRUE"}:
        return "HTTP (Xet disabled by HF_HUB_DISABLE_XET)"
    if xet_usable():
        return "Xet"
    if uv is None:
        return "HTTP (Xet unavailable)"

    # Never install into a system interpreter. The module's resolved Hub
    # version supplies the compatible range for the optional native wheel.
    if sys.prefix == sys.base_prefix:
        return "HTTP (outside the managed virtual environment)"
    hub = metadata.distribution("huggingface-hub")
    extras = hub.metadata.get_all("Provides-Extra") or []
    if not any(extra.replace("_", "-").lower() == "hf-xet" for extra in extras):
        return "HTTP (the installed Hub version has no Xet extra)"

    root = Path(sys.prefix).parent
    print("Installing Xet support in the managed voice environment.", flush=True)
    installed = subprocess.run(
        [
            uv, "pip", "install", "--python", os.path.relpath(sys.executable, root),
            "--only-binary", "hf-xet", "--reinstall-package", "hf-xet",
            f"huggingface-hub[hf_xet]=={hub.version}",
        ],
        cwd=root,
    )
    if installed.returncode == 0 and xet_usable():
        return "Xet"
    return "HTTP (a usable Xet wheel could not be installed)"


def main(uv=None):
    try:
        backend = select_backend(uv)
    except (OSError, metadata.PackageNotFoundError) as error:
        backend = f"HTTP (Xet check failed with {type(error).__name__})"
    print(f"Hugging Face model downloads use {backend}.", flush=True)
    # Check-only is informational. During setup, tell the downloader to opt
    # out explicitly if Hub might otherwise select a broken native package.
    return 0 if uv is None or backend == "Xet" else 1


if __name__ == "__main__":
    if sys.argv[1:] == ["--probe"]:
        sys.exit(0 if probe_current_python() else 1)
    sys.exit(main(*sys.argv[1:]))
