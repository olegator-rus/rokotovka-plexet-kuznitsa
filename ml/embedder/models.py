"""Закреплённые модели: репозиторий, ревизия, нужные файлы и sha256 весов."""

from dataclasses import dataclass


@dataclass(frozen=True)
class ModelSpec:
    repo_id: str
    revision: str
    files: tuple[str, ...]
    weights: str
    weights_sha256: str


EMBEDDER = ModelSpec(
    repo_id="BAAI/bge-m3",
    revision="5617a9f61b028005a4858fdac845db406aefb181",
    files=(
        "config.json",
        "tokenizer.json",
        "tokenizer_config.json",
        "special_tokens_map.json",
        "sentencepiece.bpe.model",
        "pytorch_model.bin",
    ),
    weights="pytorch_model.bin",
    weights_sha256="b5e0ce3470abf5ef3831aa1bd5553b486803e83251590ab7ff35a117cf6aad38",
)

RERANKER = ModelSpec(
    repo_id="BAAI/bge-reranker-v2-m3",
    revision="953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e",
    files=(
        "config.json",
        "tokenizer.json",
        "tokenizer_config.json",
        "special_tokens_map.json",
        "sentencepiece.bpe.model",
        "model.safetensors",
    ),
    weights="model.safetensors",
    weights_sha256="d9e3e081faff1eefb84019509b2f5558fd74c1a05a2c7db22f74174fcedb5286",
)

ALL = (EMBEDDER, RERANKER)
