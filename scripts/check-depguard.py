"""Verify the repository's depguard globs using the installed CI linter."""
import os
from pathlib import Path
import subprocess
import tempfile


def main():
    config = Path(__file__).resolve().parents[1] / ".golangci.yml"
    module = "github.com/mapledaemon/MagicHandy"
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        (root / "go.mod").write_text(f"module {module}\n\ngo 1.26.11\n", encoding="utf-8")
        owner = root / "internal" / "transport"
        owner.mkdir(parents=True)
        (owner / "transport.go").write_text("package transport\n", encoding="utf-8")
        fixtures = ["internal/modes/direct.go", "internal/modes/nested/nested.go"]
        for fixture in fixtures:
            file = root / fixture
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_text(f'package {file.parent.name}\n\nimport _ "{module}/internal/transport"\n', encoding="utf-8")
        result = subprocess.run(
            ["golangci-lint", "run", "--config", str(config), "--enable-only", "depguard", "./..."],
            cwd=root, capture_output=True, text=True, timeout=180, check=False,
            env={**os.environ, "GOWORK": "off"},
        )
        report = (result.stdout + result.stderr).replace("\\", "/")
        for fixture in fixtures:
            if result.returncode != 1 or not any(fixture in line and "depguard" in line for line in report.splitlines()):
                raise SystemExit(f"Expected a depguard rejection for {fixture}\n{report}")
        print("Depguard rejected direct and nested semantic transport imports.")


if __name__ == "__main__":
    main()
