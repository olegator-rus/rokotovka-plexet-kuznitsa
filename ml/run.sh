#!/usr/bin/env bash
# Эмбеддер BGE-M3 + реранкер bge-reranker-v2-m3 на 127.0.0.1:8089 (TEI-совместимый API).
#   ml/run.sh          — подготовить окружение и модели, запустить в текущем терминале
#   ml/run.sh start    — то же в фоне: PID в ml/.pid, журнал в ml/.run.log, ждёт /health
#   ml/run.sh stop     — остановить фоновый процесс по PID из ml/.pid
#   ml/run.sh status   — запущен ли и отвечает ли /health
# Процесс гасится только по PID, никогда по порту.
set -euo pipefail
cd "$(dirname "$0")"

export ML_HOST="${ML_HOST:-127.0.0.1}"
export ML_PORT="${ML_PORT:-8089}"
PID_FILE=.pid
LOG_FILE=.run.log
URL="http://${ML_HOST}:${ML_PORT}"

running_pid() {
  [[ -f $PID_FILE ]] || return 1
  local pid
  pid=$(cat "$PID_FILE")
  if [[ -n $pid ]] && ps -p "$pid" -o command= 2>/dev/null | grep -q "embedder"; then
    echo "$pid"
    return 0
  fi
  return 1
}

healthy() { curl -fsS -m 2 -o /dev/null "$URL/health" 2>/dev/null; }

setup() {
  local py="${PYTHON:-}"
  if [[ -z $py ]]; then
    py=$(command -v python3.13 || true)
    [[ -n $py ]] || py=/opt/homebrew/bin/python3.13
  fi
  if [[ ! -x .venv/bin/python ]]; then
    echo "creating ml/.venv with $py"
    "$py" -m venv .venv
  fi
  local want have
  want=$(shasum -a 256 requirements.txt | cut -d' ' -f1)
  have=$(cat .venv/.requirements.sha256 2>/dev/null || true)
  if [[ $want != "$have" ]]; then
    echo "installing ml/requirements.txt"
    .venv/bin/python -m pip install -q --upgrade pip
    .venv/bin/python -m pip install -q -r requirements.txt
    echo "$want" > .venv/.requirements.sha256
  fi
  .venv/bin/python -m embedder.download
}

serve_env() {
  export HF_HUB_OFFLINE=1
  export TOKENIZERS_PARALLELISM=false
  export PYTORCH_ENABLE_MPS_FALLBACK=1
}

case "${1:-run}" in
  run)
    setup
    serve_env
    exec .venv/bin/python -m embedder
    ;;
  start)
    if pid=$(running_pid); then
      echo "already running, pid $pid"
      exit 0
    fi
    setup
    serve_env
    nohup .venv/bin/python -m embedder >"$LOG_FILE" 2>&1 &
    pid=$!
    echo "$pid" > "$PID_FILE"
    echo "started pid $pid, waiting for $URL/health"
    for _ in $(seq 1 300); do
      if healthy; then
        echo "healthy"
        exit 0
      fi
      if ! ps -p "$pid" >/dev/null 2>&1; then
        echo "process exited, see ml/$LOG_FILE:" >&2
        tail -n 20 "$LOG_FILE" >&2
        rm -f "$PID_FILE"
        exit 1
      fi
      sleep 1
    done
    echo "no /health after 300s, see ml/$LOG_FILE" >&2
    exit 1
    ;;
  stop)
    if pid=$(running_pid); then
      kill "$pid"
      for _ in $(seq 1 20); do
        ps -p "$pid" >/dev/null 2>&1 || break
        sleep 0.5
      done
      echo "stopped pid $pid"
    else
      echo "not running"
    fi
    rm -f "$PID_FILE"
    ;;
  status)
    if pid=$(running_pid); then
      if healthy; then echo "running, pid $pid, healthy"; else echo "running, pid $pid, not healthy yet"; fi
    else
      echo "not running"
      exit 1
    fi
    ;;
  *)
    echo "usage: ml/run.sh [run|start|stop|status]" >&2
    exit 2
    ;;
esac
