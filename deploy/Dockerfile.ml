# syntax=docker/dockerfile:1
# Сервис моделей стенда: эмбеддер BGE-M3 и реранкер bge-reranker-v2-m3 на CPU (тот же код, что ml/ локально).
# Контекст сборки — корень репозитория. Веса не в образе: скачиваются при первом старте в том hf_cache
# (закреплённые ревизии, sha256 сверяется — ml/embedder/download.py), дальше сервис в сеть не ходит.
FROM python:3.13-slim
ENV PIP_NO_CACHE_DIR=1 PYTHONUNBUFFERED=1 TOKENIZERS_PARALLELISM=false \
    HF_HOME=/models ML_HOST=0.0.0.0 ML_PORT=8089 ML_DEVICE=cpu
WORKDIR /app
COPY ml/requirements.txt .
# torch — сборка только под CPU (без CUDA-библиотек на несколько ГБ); остальное — по замку requirements.txt.
RUN pip install --index-url https://download.pytorch.org/whl/cpu "torch==$(sed -n 's/^torch==//p' requirements.txt)" && \
    grep -v '^torch==' requirements.txt > req.txt && pip install -r req.txt && \
    adduser --disabled-password --uid 10001 app && mkdir -p /models && chown app /models
COPY ml/embedder ./embedder
USER app
EXPOSE 8089
# Первый старт качает веса (~4,5 ГБ), следующие — офлайн.
CMD ["sh", "-c", "python -m embedder.download && HF_HUB_OFFLINE=1 exec python -m embedder"]
