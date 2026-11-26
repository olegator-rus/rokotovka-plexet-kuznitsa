"""Пачки одновременных запросов: один поток владеет моделью и забирает из очереди всё, что накопилось.

Под нагрузкой (20 поисков параллельно) запросы приходили к модели по одному, и вектор короткого запроса
ждал в очереди за реранком чужих карточек. Теперь:
- все ждущие /embed с одинаковым normalize идут в модель одной пачкой (до max_batch текстов) — 20 коротких
  запросов считаются почти за время одного;
- /embed и /rerank чередуются пачками, когда ждут оба (вектор запроса нужен всем маршрутам, реранк — только
  C и D; строгий приоритет векторов морил реранк под нагрузкой);
- ждущие /rerank склеиваются в одну пачку пар (до rerank_batch пар) — меньше проходов модели.
Память MPS не растёт сверх одного запроса максимального размера: пачка не больше max_batch, в модель она
идёт кусками по micro_batch, как и раньше.
"""

import threading
import time
from collections import deque
from concurrent.futures import Future

import torch

from .engine import Engine


class _Job:
    __slots__ = ("kind", "texts", "query", "flag", "future")

    def __init__(self, kind: str, texts: list[str], query: str, flag: bool):
        self.kind, self.texts, self.query, self.flag = kind, texts, query, flag
        self.future: Future = Future()


class Batcher:
    def __init__(self, engine: Engine, max_batch: int = 64, rerank_batch: int = 32, wait_ms: float = 0.0):
        self.engine = engine
        self.max_batch = max_batch
        # Реранк растёт линейно от числа пар: большая пачка держит модель дольше и задерживает векторы запросов.
        self.rerank_batch = rerank_batch
        self.wait = wait_ms / 1000.0
        self._embed: deque[_Job] = deque()
        self._rerank: deque[_Job] = deque()
        self._cond = threading.Condition()
        self._last = ""
        self.stats = {"embed_batches": 0, "embed_requests": 0, "rerank_batches": 0, "rerank_requests": 0}
        threading.Thread(target=self._loop, name="model-worker", daemon=True).start()

    def embed(self, texts: list[str], normalize: bool = True) -> torch.Tensor:
        return self._submit(_Job("embed", texts, "", normalize), self._embed).result()

    def rerank(self, query: str, texts: list[str], raw_scores: bool = False) -> list[float]:
        return self._submit(_Job("rerank", texts, query, raw_scores), self._rerank).result()

    def _submit(self, job: _Job, queue: deque) -> Future:
        with self._cond:
            queue.append(job)
            self._cond.notify()
        return job.future

    def _take(self, queue: deque, limit: int) -> list[_Job]:
        """Первый в очереди и следующие с тем же флагом, пока пачка не больше limit."""
        first = queue.popleft()
        batch, n = [first], len(first.texts)
        rest = deque()
        while queue:
            job = queue.popleft()
            if job.flag == first.flag and n + len(job.texts) <= limit:
                batch.append(job)
                n += len(job.texts)
            else:
                rest.append(job)
        queue.extendleft(reversed(rest))
        return batch

    def _loop(self) -> None:
        while True:
            with self._cond:
                while not self._embed and not self._rerank:
                    self._cond.wait()
            if self.wait:
                time.sleep(self.wait)  # дать подойти соседям; по умолчанию 0 — пачку собирает сама очередь
            with self._cond:
                # Векторы — первыми, но не дольше одной пачки подряд, если ждёт реранк: иначе под нагрузкой
                # маршруты C и D стоят в очереди бесконечно.
                if self._embed and (not self._rerank or self._last == "rerank"):
                    kind, batch = "embed", self._take(self._embed, self.max_batch)
                else:
                    kind, batch = "rerank", self._take(self._rerank, self.rerank_batch)
                self._last = kind
            for job in batch:
                job.future.set_running_or_notify_cancel()
            try:
                if kind == "embed":
                    self._run_embed(batch)
                else:
                    self._run_rerank(batch)
            except Exception as exc:  # сбой модели — ошибка каждому из пачки, поток живёт дальше
                for job in batch:
                    if not job.future.done():
                        job.future.set_exception(exc)

    def _run_embed(self, batch: list[_Job]) -> None:
        texts = [t for job in batch for t in job.texts]
        vecs = self.engine.embed(texts, normalize=batch[0].flag)
        self.stats["embed_batches"] += 1
        self.stats["embed_requests"] += len(batch)
        at = 0
        for job in batch:
            job.future.set_result(vecs[at : at + len(job.texts)])
            at += len(job.texts)

    def _run_rerank(self, batch: list[_Job]) -> None:
        queries = [job.query for job in batch for _ in job.texts]
        texts = [t for job in batch for t in job.texts]
        scores = self.engine.rerank_pairs(queries, texts, raw_scores=batch[0].flag)
        self.stats["rerank_batches"] += 1
        self.stats["rerank_requests"] += len(batch)
        at = 0
        for job in batch:
            job.future.set_result(scores[at : at + len(job.texts)])
            at += len(job.texts)
