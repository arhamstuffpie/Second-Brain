from __future__ import annotations

import hashlib
import os
import shutil
from pathlib import Path

from huggingface_hub import snapshot_download


ROOT = Path(__file__).parent / "models"
MODELS = (
    ("pyannote/speaker-diarization-community-1", os.environ.get("PYANNOTE_REVISION", ""), "pyannote-community-1"),
    ("speechbrain/sepformer-whamr16k", "cfd5df650d52eb60ba93e009acdb9894c5900038", "sepformer-whamr16k"),
)


def main() -> None:
    token = os.environ.get("HF_TOKEN")
    if not token:
        raise SystemExit("HF_TOKEN is required; accept the Community-1 model terms first")
    if not MODELS[0][1]:
        raise SystemExit("PYANNOTE_REVISION is required and must be a reviewed commit SHA")
    ROOT.mkdir(exist_ok=True)
    for repository, revision, directory in MODELS:
        snapshot_download(
            repo_id=repository,
            revision=revision,
            local_dir=ROOT / directory,
            token=token,
        )
        shutil.rmtree(ROOT / directory / ".cache", ignore_errors=True)
    lines: list[str] = []
    for path in sorted(item for item in ROOT.rglob("*") if item.is_file() and item.name != "SHA256SUMS" and ".cache" not in item.parts):
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        lines.append(f"{digest}  {path.relative_to(ROOT)}")
    (ROOT / "SHA256SUMS").write_text("\n".join(lines) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
