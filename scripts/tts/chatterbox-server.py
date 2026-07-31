"""Launch the pinned Chatterbox server without opening its standalone UI."""

from __future__ import annotations

import runpy
import sys
import webbrowser
from pathlib import Path


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: chatterbox-server.py <upstream-server.py>")

    server = Path(sys.argv[1]).resolve()
    if not server.is_file():
        raise SystemExit(f"Chatterbox server is unavailable: {server}")

    # MagicHandy owns the surrounding UI. Upstream opens a browser after model
    # load; suppress that one side effect while leaving the server unchanged.
    webbrowser.open = lambda *_args, **_kwargs: False
    sys.path.insert(0, str(server.parent))
    sys.argv = [str(server)]
    runpy.run_path(str(server), run_name="__main__")


if __name__ == "__main__":
    main()
