// Загрузка и разбор ТЗ (контур ТЗ, TZ-UI.md §2–3, §10): одна операция на вкладку, общая для строки поиска и зоны
// сброса на любой странице. Отмена и обычный поиск во время разбора отменяют операцию: поздний completed не
// переводит на страницу ТЗ. После completed — переход на /search?tz=<хеш>, полоса остаётся на шаге «Ищу товары»,
// пока страница ищет товары по позициям.
import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { deleteTzOperation, getTzOperation, postTz } from '@/api/tz'
import { isApiError } from '@/lib/apiError'
import {
  type TzStepKey,
  type TzUploadError,
  abortableWait,
  pollOperation,
  reasonToError,
  stageStep,
  validateTzFile,
  validateTzFiles,
} from '@/lib/tz'

export type TzPhase = 'idle' | 'processing' | 'searching'
/** line — под строкой поиска (проверка файла), bar — в полосе разбора (ответ сервера). */
export type TzErrorPlace = 'line' | 'bar'

/** Отказ POST /api/tz → причина по коду или статусу сервера; иначе общая. */
export function uploadErrorOf(e: unknown): TzUploadError {
  if (!isApiError(e)) return 'general'
  const code = (e.code || '').toLowerCase()
  if (e.status === 413 || code.includes('size') || code.includes('too_large')) return 'size'
  if (e.status === 415 || code.includes('format') || code.includes('unsupported')) return 'format'
  if (code.includes('password')) return 'password'
  if (code.includes('damaged') || code.includes('corrupt')) return 'damaged'
  return 'general'
}

export const useTzStore = defineStore('tz', () => {
  const router = useRouter()
  const phase = ref<TzPhase>('idle')
  const fileName = ref('')
  const step = ref<TzStepKey>('read')
  const sawOcr = ref(false)
  const error = ref<TzUploadError | null>(null)
  const errorPlace = ref<TzErrorPlace>('line')
  /** Хеш, для которого полоса стоит на «Ищу товары». */
  const searchingHash = ref<string | null>(null)
  const lastFile = shallowRef<File | null>(null)
  let ctrl: AbortController | null = null
  let operationId: string | null = null
  /** Скрытое поле выбора файла (GlobalTzDropzone): одно на приложение. */
  let picker: (() => void) | null = null

  function registerPicker(fn: (() => void) | null) {
    picker = fn
  }

  function openPicker() {
    picker?.()
  }

  function clearError() {
    error.value = null
  }

  function fail(e: TzUploadError, place: TzErrorPlace) {
    error.value = e
    errorPlace.value = place
  }

  /** Отменить текущий разбор: опрос прекращается, ответ игнорируется, серверу — DELETE. */
  function cancel() {
    const id = operationId
    ctrl?.abort()
    ctrl = null
    operationId = null
    if (phase.value === 'processing') phase.value = 'idle'
    if (id) deleteTzOperation(id).catch(() => undefined)
  }

  /** Обычный поиск во время разбора: разбор отменяется, на страницу ТЗ не переводим. */
  function onTextSearch() {
    if (phase.value === 'processing') cancel()
    if (phase.value === 'searching') finishSearching()
  }

  function finishSearching(hash?: string) {
    if (hash && searchingHash.value && hash !== searchingHash.value) return
    if (phase.value === 'searching') phase.value = 'idle'
    searchingHash.value = null
  }

  /** Файлы из выбора или перетаскивания: ровно один подходящий, иначе ошибка под строкой поиска. */
  function startFiles(files: readonly File[]) {
    const v = validateTzFiles(files)
    if (v.error) {
      fail(v.error, 'line')
      return
    }
    if (v.file !== null) void start(files[v.file] as File)
  }

  async function start(file: File) {
    const bad = validateTzFile(file)
    if (bad) {
      fail(bad, 'line')
      return
    }
    cancel()
    finishSearching()
    error.value = null
    lastFile.value = file
    fileName.value = file.name
    step.value = 'read'
    sawOcr.value = false
    phase.value = 'processing'
    const c = new AbortController()
    ctrl = c
    let id: string
    try {
      const up = await postTz(file, { signal: c.signal })
      if (ctrl === c && up.status === 'error') {
        ctrl = null
        phase.value = 'idle'
        const e = reasonToError(up.reason)
        fail(e, e === 'format' || e === 'size' ? 'line' : 'bar')
        return
      }
      id = up.operation_id
    } catch (e) {
      if (ctrl !== c) return
      ctrl = null
      phase.value = 'idle'
      fail(uploadErrorOf(e), uploadErrorOf(e) === 'general' ? 'bar' : 'line')
      return
    }
    if (ctrl !== c) {
      deleteTzOperation(id).catch(() => undefined)
      return
    }
    operationId = id
    const op = await pollOperation(
      id,
      {
        get: (opId, signal) => getTzOperation(opId, { signal }),
        wait: abortableWait,
        onStage: (stage) => {
          if (ctrl !== c) return
          step.value = stageStep(stage)
          if (step.value === 'ocr') sawOcr.value = true
        },
      },
      c.signal,
    )
    if (ctrl !== c || !op) return
    ctrl = null
    operationId = null
    if (op.status === 'completed' && op.hash) {
      step.value = 'search'
      phase.value = 'searching'
      searchingHash.value = op.hash
      await router.push({ path: '/search', query: { tz: op.hash } })
      return
    }
    phase.value = 'idle'
    fail(op.status === 'error' ? reasonToError(op.reason) : 'general', 'bar')
  }

  function retry() {
    if (lastFile.value) void start(lastFile.value)
    else openPicker()
  }

  return {
    phase,
    fileName,
    step,
    sawOcr,
    error,
    errorPlace,
    searchingHash,
    registerPicker,
    openPicker,
    clearError,
    cancel,
    onTextSearch,
    finishSearching,
    startFiles,
    start,
    retry,
  }
})
