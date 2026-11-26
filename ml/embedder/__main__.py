"""Запуск: python -m embedder (из каталога ml/). Настройки — переменные окружения ML_*."""

import logging
import os
import time

import uvicorn

from .app import create_app
from .engine import Engine


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    log = logging.getLogger("embedder")
    host = os.environ.get("ML_HOST", "127.0.0.1")
    port = int(os.environ.get("ML_PORT", "8089"))
    max_batch = int(os.environ.get("ML_MAX_BATCH", "64"))

    t0 = time.perf_counter()
    engine = Engine(
        device=os.environ.get("ML_DEVICE", "auto"),
        dtype=os.environ.get("ML_DTYPE", "auto"),
        max_length=int(os.environ.get("ML_MAX_LENGTH", "512")),
        micro_batch=int(os.environ.get("ML_MICRO_BATCH", "32")),
    )
    engine.load_seconds["total"] = round(time.perf_counter() - t0, 2)
    log.info("models ready: device=%s dtype=%s load=%s %s", engine.device, engine.dtype, engine.load_seconds, engine.memory())

    uvicorn.run(create_app(engine, max_batch), host=host, port=port, workers=1, log_level="info", access_log=True)


if __name__ == "__main__":
    main()
