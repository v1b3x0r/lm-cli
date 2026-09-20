#!/usr/bin/env python3
"""Build explicit release bundles; never uploads or includes runtime credentials."""
import hashlib
import io
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent.parent
version = sys.argv[1] if len(sys.argv) == 2 else ""
if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.]+)?", version):
    raise SystemExit("usage: python3 scripts/package.py 0.1.0-rc.1")
dest = ROOT / "dist" / version
dest.mkdir(parents=True, exist_ok=False)
files = {
    "README.md": ROOT / "README.md",
    "QUICKSTART.md": ROOT / "docs/agent-quickstart.md",
    "INSTALL.md": ROOT / "docs/install.md",
}
checksums = []
for target, arch in [("darwin", "arm64"), ("darwin", "amd64"),
                     ("linux", "amd64"), ("linux", "arm64")]:
    stem = f"lm-cli_{version}_{target}_{arch}"
    executable = "lm.exe" if target == "windows" else "lm"
    with tempfile.TemporaryDirectory(prefix="lm-cli-build-") as work:
        binary = Path(work) / executable
        env = dict(os.environ, CGO_ENABLED="0", GOOS=target, GOARCH=arch)
        subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags",
                        f"-s -w -X lm-cli/internal/cli.version={version}",
                        "-o", str(binary), "./cmd/lm"], cwd=ROOT, env=env, check=True)
        payload = {executable: binary.read_bytes(), **{name: path.read_bytes() for name, path in files.items()}}
        if target == "windows":
            artifact = dest / (stem + ".zip")
            with zipfile.ZipFile(artifact, "w", zipfile.ZIP_DEFLATED) as archive:
                for name, data in payload.items():
                    archive.writestr(name, data)
        else:
            artifact = dest / (stem + ".tar.gz")
            with tarfile.open(artifact, "w:gz") as archive:
                for name, data in payload.items():
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    info.mode = 0o755 if name == executable else 0o644
                    archive.addfile(info, io.BytesIO(data))
        checksums.append(f"{hashlib.sha256(artifact.read_bytes()).hexdigest()}  {artifact.name}")
        print(artifact.name, flush=True)
(dest / "SHA256SUMS").write_text("\n".join(checksums) + "\n")
print(f"Local candidate only: {dest}")
