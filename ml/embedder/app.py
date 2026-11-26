"""HTTP API, совместимый с Text Embeddings Inference: /embed, /rerank, /health, /info.

Ошибки — как у TEI: {"error": "...", "error_type": "Validation|Empty|Backend"}.
Тексты запросов в журнал не пишутся: в них поисковые фразы покупателей.
"""

import json
import logging

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse, Response
from pydantic import BaseModel

from .engine import Engine
from .models import EMBEDDER, RERANKER

log = logging.getLogger("embedder")


class EmbedRequest(BaseModel):
    inputs: str | list[str]
    normalize: bool = True
    truncate: bool | None = None


class RerankRequest(BaseModel):
    query: str
    texts: list[str]
    raw_scores: bool = False
    return_text: bool = False
    truncate: bool | None = None


class ApiError(Exception):
    STATUS = {"Validation": 413, "Empty": 400, "Backend": 424}

    def __init__(self, error_type: str, message: str):
        self.error_type = error_type
        self.message = message


def _json(payload) -> Response:
    return Response(json.dumps(payload, separators=(",", ":")), media_type="application/json")


def create_app(engine: Engine, max_batch: int) -> FastAPI:
    app = FastAPI(title="kuznica-embedder", docs_url=None, redoc_url=None, openapi_url=None)

    @app.exception_handler(ApiError)
    async def _api_error(_: Request, exc: ApiError):
        return JSONResponse({"error": exc.message, "error_type": exc.error_type}, status_code=ApiError.STATUS[exc.error_type])

    @app.exception_handler(RequestValidationError)
    async def _bad_body(_: Request, exc: RequestValidationError):
        fields = sorted({".".join(str(p) for p in e.get("loc", ()) if p != "body") for e in exc.errors()})
        return JSONResponse({"error": f"invalid request body: {', '.join(fields) or 'json'}", "error_type": "Validation"}, status_code=422)

    def check_batch(n: int, name: str) -> None:
        if n == 0:
            raise ApiError("Empty", f"`{name}` cannot be empty")
        if n > max_batch:
            raise ApiError("Validation", f"batch size {n} > maximum allowed batch size {max_batch}")

    @app.get("/health")
    def health() -> Response:
        return Response(status_code=200)

    @app.get("/info")
    def info() -> Response:
        return _json(
            {
                "model_id": EMBEDDER.repo_id,
                "model_sha": EMBEDDER.revision,
                "reranker_id": RERANKER.repo_id,
                "reranker_sha": RERANKER.revision,
                "model_dtype": engine.dtype,
                "device": engine.device,
                "dim": engine.dim,
                "pooling": "cls",
                "max_input_length": engine.max_length,
                "max_client_batch_size": max_batch,
                "micro_batch": engine.micro_batch,
                "load_seconds": engine.load_seconds,
                **engine.memory(),
            }
        )

    @app.post("/embed")
    def embed(req: EmbedRequest) -> Response:
        texts = [req.inputs] if isinstance(req.inputs, str) else req.inputs
        check_batch(len(texts), "inputs")
        try:
            vectors = engine.embed(texts, normalize=req.normalize)
        except Exception:
            log.exception("embed failed")
            raise ApiError("Backend", "embedding failed")
        return _json(vectors.tolist())

    @app.post("/rerank")
    def rerank(req: RerankRequest) -> Response:
        check_batch(len(req.texts), "texts")
        try:
            scores = engine.rerank(req.query, req.texts, raw_scores=req.raw_scores)
        except Exception:
            log.exception("rerank failed")
            raise ApiError("Backend", "rerank failed")
        ranked = sorted(range(len(scores)), key=lambda i: scores[i], reverse=True)
        out = []
        for i in ranked:
            item = {"index": i, "score": scores[i]}
            if req.return_text:
                item["text"] = req.texts[i]
            out.append(item)
        return _json(out)

    return app
