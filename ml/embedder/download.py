"""Скачивает закреплённые ревизии моделей в кеш Hugging Face и сверяет sha256 весов.

Запуск: python -m embedder.download (из каталога ml/). Уже скачанное не перекачивается.
"""

import hashlib
import os
import sys

from huggingface_hub import snapshot_download, try_to_load_from_cache

from .models import ALL, ModelSpec


def _cached(spec: ModelSpec) -> bool:
    for name in spec.files:
        path = try_to_load_from_cache(spec.repo_id, name, revision=spec.revision)
        if not isinstance(path, str) or not os.path.isfile(path):
            return False
    return True


def _sha256(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 22), b""):
            h.update(chunk)
    return h.hexdigest()


def ensure(spec: ModelSpec) -> str:
    if _cached(spec):
        return snapshot_download(spec.repo_id, revision=spec.revision, allow_patterns=list(spec.files), local_files_only=True)
    print(f"downloading {spec.repo_id}@{spec.revision[:8]}", flush=True)
    path = snapshot_download(spec.repo_id, revision=spec.revision, allow_patterns=list(spec.files))
    digest = _sha256(os.path.join(path, spec.weights))
    if digest != spec.weights_sha256:
        raise SystemExit(f"{spec.repo_id}/{spec.weights}: sha256 {digest} != {spec.weights_sha256}")
    print(f"ok {spec.repo_id} sha256 verified", flush=True)
    return path


def main() -> int:
    for spec in ALL:
        print(f"{spec.repo_id}: {ensure(spec)}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
