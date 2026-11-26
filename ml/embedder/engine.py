"""Модели BGE-M3 (dense, CLS + L2) и bge-reranker-v2-m3 на MPS или CPU."""

import logging
import threading
import time
from dataclasses import dataclass, field

import torch
import torch.nn.functional as F
from huggingface_hub import snapshot_download
from transformers import AutoModel, AutoModelForSequenceClassification, AutoTokenizer

from .models import EMBEDDER, RERANKER, ModelSpec

log = logging.getLogger("embedder")

DTYPES = {"fp16": torch.float16, "fp32": torch.float32, "bf16": torch.bfloat16}
PAD_MULTIPLE = 8  # длины кратны 8: меньше разных форм и перекомпиляций графов MPS, лишних токенов ~5%


def pick_device(requested: str) -> str:
    if requested == "auto":
        return "mps" if torch.backends.mps.is_available() else "cpu"
    if requested == "mps" and not torch.backends.mps.is_available():
        log.warning("MPS недоступен, работаю на CPU")
        return "cpu"
    return requested


def pick_dtype(requested: str, device: str) -> str:
    if requested == "auto":
        return "fp16" if device == "mps" else "fp32"
    if requested not in DTYPES:
        raise ValueError(f"ML_DTYPE={requested}: ожидается auto|fp16|fp32|bf16")
    return requested


def _local_path(spec: ModelSpec) -> str:
    return snapshot_download(spec.repo_id, revision=spec.revision, allow_patterns=list(spec.files), local_files_only=True)


@dataclass
class Engine:
    device: str = "auto"
    dtype: str = "auto"
    max_length: int = 512
    micro_batch: int = 32
    load_seconds: dict = field(default_factory=dict)

    def __post_init__(self) -> None:
        self.device = pick_device(self.device)
        self.dtype = pick_dtype(self.dtype, self.device)
        self._lock = threading.Lock()
        torch_dtype = DTYPES[self.dtype]

        t0 = time.perf_counter()
        path = _local_path(EMBEDDER)
        self.emb_tok = AutoTokenizer.from_pretrained(path)
        self.emb_model = AutoModel.from_pretrained(path, dtype=torch_dtype).to(self.device).eval()
        self.load_seconds["embedder"] = round(time.perf_counter() - t0, 2)

        t0 = time.perf_counter()
        path = _local_path(RERANKER)
        self.rr_tok = AutoTokenizer.from_pretrained(path)
        self.rr_model = AutoModelForSequenceClassification.from_pretrained(path, dtype=torch_dtype).to(self.device).eval()
        self.load_seconds["reranker"] = round(time.perf_counter() - t0, 2)

        t0 = time.perf_counter()
        self._warmup()
        if self.device == "mps":
            torch.mps.empty_cache()
        self.load_seconds["warmup"] = round(time.perf_counter() - t0, 2)

    def _warmup(self) -> None:
        """Прогоняет типичные формы: запрос 16–128 токенов, пачка карточек, реранк карточек."""
        seen = set()
        for m in range(1, 200):
            text = "лицензия " * m
            bucket = -(-len(self.emb_tok(text)["input_ids"]) // PAD_MULTIPLE) * PAD_MULTIPLE
            if bucket > 128:
                break
            if bucket not in seen:
                seen.add(bucket)
                self.embed([text])
        for words in (12, 25):
            self.embed(["карточка товара " * words] * self.micro_batch)
            self.rerank("прогрев", ["карточка товара " * words] * 20)

    @property
    def dim(self) -> int:
        return int(self.emb_model.config.hidden_size)

    def _chunks(self, encoded: dict) -> list[list[int]]:
        order = sorted(range(len(encoded["input_ids"])), key=lambda i: len(encoded["input_ids"][i]))
        return [order[i : i + self.micro_batch] for i in range(0, len(order), self.micro_batch)]

    def _pad(self, tok, encoded: dict, idx: list[int]) -> dict:
        part = {k: [encoded[k][i] for i in idx] for k in ("input_ids", "attention_mask")}
        batch = tok.pad(part, padding=True, pad_to_multiple_of=PAD_MULTIPLE, return_tensors="pt")
        return {k: v.to(self.device) for k, v in batch.items()}

    @torch.inference_mode()
    def embed(self, texts: list[str], normalize: bool = True) -> torch.Tensor:
        out = torch.empty(len(texts), self.dim, dtype=torch.float32)
        with self._lock:  # быстрый токенизатор не потокобезопасен при truncation, MPS — одна очередь
            encoded = self.emb_tok(texts, truncation=True, max_length=self.max_length)
            for idx in self._chunks(encoded):
                hidden = self.emb_model(**self._pad(self.emb_tok, encoded, idx)).last_hidden_state
                vecs = hidden[:, 0].float()
                if normalize:
                    vecs = F.normalize(vecs, p=2, dim=-1)
                out[idx] = vecs.cpu()
        return out

    @torch.inference_mode()
    def rerank(self, query: str, texts: list[str], raw_scores: bool = False) -> list[float]:
        out = torch.empty(len(texts), dtype=torch.float32)
        with self._lock:
            encoded = self.rr_tok([query] * len(texts), texts, truncation=True, max_length=self.max_length)
            for idx in self._chunks(encoded):
                logits = self.rr_model(**self._pad(self.rr_tok, encoded, idx)).logits.view(-1).float()
                out[idx] = (logits if raw_scores else torch.sigmoid(logits)).cpu()
        return out.tolist()

    def memory(self) -> dict:
        if self.device != "mps":
            return {}
        return {
            "mps_allocated_mb": round(torch.mps.current_allocated_memory() / 2**20),
            "mps_driver_mb": round(torch.mps.driver_allocated_memory() / 2**20),
        }
