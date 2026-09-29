import { computed, onUnmounted, ref, toValue, watch } from 'vue'
import type { MaybeRefOrGetter } from 'vue'
import { api } from '../services/api'
import { wsService } from '../services/websocket'
import { useTaskStore } from '../stores/tasks'
import type {
  ConsoleActivityDelta,
  ConsoleEntry,
  ConsoleEntryKind,
  OrchestratorEvent,
  Task,
} from '../types'

const LOCAL_PREFIX = 'local_'
let localSeq = 0

export function isLocalEntry(e: ConsoleEntry): boolean {
  return e.id.startsWith(LOCAL_PREFIX)
}

/**
 * Structured agent activity transcript for one task.
 * Contract: .superpowers/console/contract.md
 */
export function useAgentConsole(taskIdSource: MaybeRefOrGetter<string>) {
  const taskStore = useTaskStore()

  const entries = ref<ConsoleEntry[]>([])
  const loading = ref(false)
  const loadError = ref<string | null>(null)
  const busy = ref(false)
  const activeTurnId = ref('')
  const turnStartedAt = ref<number | null>(null)
  const serverSessionId = ref('')
  const serverModel = ref('')
  /** Optional model override sent with subsequent chat messages (empty = router default). */
  const selectedModel = ref('')
  const localTask = ref<Task | null>(null)
  const sending = ref(false)

  // Turns known to be finished; late authoritative upserts must not revive them.
  const completedTurns = new Set<string>()
  let loadSeq = 0

  const taskId = computed(() => toValue(taskIdSource))

  const task = computed<Task | null>(() => {
    return localTask.value || taskStore.tasks.find(t => t.id === taskId.value) || null
  })

  // ---------------------------------------------------------------------------
  // Turn / busy tracking
  // ---------------------------------------------------------------------------

  function setBusy(turnId: string) {
    if (!busy.value) turnStartedAt.value = Date.now()
    busy.value = true
    if (turnId) activeTurnId.value = turnId
  }

  function setIdle() {
    busy.value = false
    activeTurnId.value = ''
    turnStartedAt.value = null
  }

  function isTerminal(e: ConsoleEntry): boolean {
    return e.kind === 'response' || e.status === 'cancelled'
  }

  /** Only applied for live events (WS / gap-fill), never for the initial load. */
  function trackTurn(e: ConsoleEntry) {
    const turn = e.turn_id || ''
    if (!turn) return
    if (isTerminal(e)) {
      completedTurns.add(turn)
      if (!activeTurnId.value || activeTurnId.value === turn) setIdle()
      return
    }
    if (e.kind === 'error') {
      // ASSUMPTION: an error ends the turn unless further request/stream entries follow.
      if (!activeTurnId.value || activeTurnId.value === turn) setIdle()
      return
    }
    if (completedTurns.has(turn)) return
    if (e.kind === 'request' || e.kind === 'user' || e.status === 'streaming' || e.kind === 'tool_use') {
      setBusy(turn)
    }
  }

  // ---------------------------------------------------------------------------
  // Entry mutation
  // ---------------------------------------------------------------------------

  function findIndex(id: string): number {
    const list = entries.value
    for (let i = list.length - 1; i >= 0; i--) {
      if (list[i].id === id) return i
    }
    return -1
  }

  function upsert(entry: ConsoleEntry, live = true) {
    const idx = findIndex(entry.id)
    if (idx === -1) entries.value.push(entry)
    else entries.value[idx] = entry
    if (entry.request?.session_id) serverSessionId.value = entry.request.session_id
    if (entry.usage?.session_id) serverSessionId.value = entry.usage.session_id
    if (entry.kind === 'request' && entry.request?.model) serverModel.value = entry.request.model
    if (live) trackTurn(entry)
  }

  function applyDelta(d: ConsoleActivityDelta, eventTaskId: string) {
    if (!d?.id || typeof d.delta !== 'string') return
    const idx = findIndex(d.id)
    if (idx === -1) {
      // Unknown id and no task attribution → cannot safely place it.
      if (!eventTaskId) return
      entries.value.push({
        id: d.id,
        task_id: eventTaskId,
        turn_id: activeTurnId.value,
        kind: d.kind === 'thinking' ? 'thinking' : 'assistant',
        content: d.delta,
        status: 'streaming',
        created_at: new Date().toISOString(),
      })
      if (!busy.value) setBusy(activeTurnId.value)
      return
    }
    const e = entries.value[idx]
    e.content = (e.content || '') + d.delta
    if (!e.status || e.status === 'done') e.status = 'streaming'
  }

  function addLocal(kind: Extract<ConsoleEntryKind, 'system' | 'error' | 'user'>, content: string): ConsoleEntry {
    const now = new Date().toISOString()
    const entry: ConsoleEntry = {
      id: `${LOCAL_PREFIX}${Date.now().toString(36)}_${++localSeq}`,
      task_id: taskId.value,
      kind,
      content,
      status: 'done',
      created_at: now,
      updated_at: now,
    }
    entries.value.push(entry)
    return entry
  }

  const lastServerId = computed(() => {
    const list = entries.value
    for (let i = list.length - 1; i >= 0; i--) {
      if (!isLocalEntry(list[i])) return list[i].id
    }
    return ''
  })

  // ---------------------------------------------------------------------------
  // Loading
  // ---------------------------------------------------------------------------

  async function refreshTask() {
    const id = taskId.value
    if (!id) return
    try {
      const t = await api.getTask(id)
      if (id !== taskId.value) return
      localTask.value = t
      const idx = taskStore.tasks.findIndex(x => x.id === id)
      if (idx !== -1) taskStore.tasks[idx] = t
    } catch {
      // keep whatever the store already has
    }
  }

  async function load() {
    const id = taskId.value
    const seq = ++loadSeq
    entries.value = []
    completedTurns.clear()
    setIdle()
    localTask.value = null
    loadError.value = null
    if (!id) return
    loading.value = true
    refreshTask()
    try {
      const res = await api.getTaskActivity(id)
      if (seq !== loadSeq) return
      const incoming = res.entries || []
      for (const e of incoming) {
        upsert(e, false)
        if (e.turn_id && isTerminal(e)) completedTurns.add(e.turn_id)
      }
      if (res.session_id) serverSessionId.value = res.session_id
      if (res.model) serverModel.value = res.model
      if (res.busy) setBusy(res.active_turn_id || '')
    } catch (err: any) {
      if (seq === loadSeq) loadError.value = err?.message || 'Failed to load activity'
    } finally {
      if (seq === loadSeq) loading.value = false
    }
  }

  /** Fetch entries newer than the last one we hold (used after a WS reconnect). */
  async function fillGap() {
    const id = taskId.value
    if (!id) return
    const seq = loadSeq
    const after = lastServerId.value
    try {
      const res = await api.getTaskActivity(id, after || undefined)
      if (seq !== loadSeq) return
      for (const e of res.entries || []) upsert(e, true)
      if (after && (res.entries || []).length) {
        // Keep chronological order after merging a gap.
        entries.value.sort((a, b) => (a.created_at || '').localeCompare(b.created_at || ''))
      }
      if (res.session_id) serverSessionId.value = res.session_id
      if (res.model) serverModel.value = res.model
      if (res.busy) setBusy(res.active_turn_id || '')
      else setIdle()
    } catch {
      // next reconnect will retry
    }
  }

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  async function send(message: string): Promise<boolean> {
    const text = message.trim()
    if (!text || !taskId.value) return false
    if (busy.value || sending.value) {
      addLocal('error', 'A turn is already in progress — press esc to interrupt it first.')
      return false
    }
    sending.value = true
    try {
      const res = await api.sendTaskChat(taskId.value, text, selectedModel.value || undefined)
      if (res.turn_id && !completedTurns.has(res.turn_id)) setBusy(res.turn_id)
      if (res.entry_id && findIndex(res.entry_id) === -1) {
        const now = new Date().toISOString()
        entries.value.push({
          id: res.entry_id,
          task_id: taskId.value,
          turn_id: res.turn_id,
          kind: 'user',
          content: text,
          status: 'done',
          created_at: now,
          updated_at: now,
        })
      }
      return true
    } catch (err: any) {
      if (err?.status === 409) {
        addLocal('error', err?.message || 'A turn is already running for this task.')
        setBusy(activeTurnId.value)
      } else {
        addLocal('error', err?.message || 'Failed to send message')
      }
      return false
    } finally {
      sending.value = false
    }
  }

  async function cancel(): Promise<void> {
    if (!taskId.value) return
    try {
      const res = await api.cancelTaskChat(taskId.value)
      if (res.status === 'idle') setIdle()
      // "cancelled": the backend emits the cancelled entries; they end the turn.
    } catch (err: any) {
      addLocal('error', err?.message || 'Failed to interrupt the running turn')
    }
  }

  async function clear(): Promise<void> {
    if (!taskId.value) return
    await api.clearTaskActivity(taskId.value)
    entries.value = []
    completedTurns.clear()
  }

  function clearLocalOnly() {
    entries.value = entries.value.filter(e => !isLocalEntry(e))
  }

  // ---------------------------------------------------------------------------
  // Derived values
  // ---------------------------------------------------------------------------

  const totals = computed(() => {
    let prompt = 0
    let completion = 0
    let cached = 0
    let cost = 0
    let durationMs = 0
    let responses = 0
    for (const e of entries.value) {
      if (e.kind !== 'response' || !e.usage) continue
      responses++
      prompt += e.usage.prompt_tokens || 0
      completion += e.usage.completion_tokens || 0
      cached += e.usage.cached_tokens || 0
      cost += e.usage.cost_usd || 0
      durationMs += e.usage.duration_ms || 0
    }
    return { prompt, completion, cached, tokens: prompt + completion, cost, durationMs, responses }
  })

  /** chars/4 estimate of text streamed so far in the active turn. */
  const streamingTokenEstimate = computed(() => {
    if (!busy.value) return 0
    let chars = 0
    const turn = activeTurnId.value
    for (const e of entries.value) {
      if (e.kind !== 'assistant' && e.kind !== 'thinking') continue
      if (turn ? e.turn_id === turn : e.status === 'streaming') chars += (e.content || '').length
    }
    return Math.round(chars / 4)
  })

  const currentModel = computed(() => selectedModel.value || serverModel.value || task.value?.metadata?.active_model || '')

  // ---------------------------------------------------------------------------
  // WebSocket wiring
  // ---------------------------------------------------------------------------

  function belongs(event: OrchestratorEvent, payloadTaskId?: string): boolean {
    const id = taskId.value
    if (event.task_id) return event.task_id === id
    return payloadTaskId ? payloadTaskId === id : false
  }

  function onEvent(event: OrchestratorEvent) {
    switch (event.type) {
      case 'agent.activity': {
        const e = event.payload as ConsoleEntry
        if (!e?.id || !belongs(event, e.task_id)) return
        upsert(e, true)
        break
      }
      case 'agent.activity.delta': {
        const d = event.payload as ConsoleActivityDelta
        if (!d?.id) return
        if (event.task_id && event.task_id !== taskId.value) return
        if (!event.task_id && findIndex(d.id) === -1) return
        applyDelta(d, event.task_id)
        break
      }
      case 'agent.activity.clear': {
        if (!belongs(event, event.payload?.task_id)) return
        entries.value = []
        completedTurns.clear()
        break
      }
      case 'task.status': {
        const t = event.payload as Task
        if (!t?.id || t.id !== taskId.value) return
        localTask.value = t
        break
      }
    }
  }

  const unsubscribe = wsService.subscribe(onEvent)

  const stopReconnectWatch = watch(wsService.isConnected, (now, prev) => {
    if (now && !prev && !loading.value) fillGap()
  })

  watch(taskId, () => { load() }, { immediate: true })

  onUnmounted(() => {
    unsubscribe()
    stopReconnectWatch()
  })

  return {
    taskId,
    task,
    entries,
    loading,
    loadError,
    busy,
    sending,
    activeTurnId,
    turnStartedAt,
    sessionId: serverSessionId,
    serverModel,
    selectedModel,
    currentModel,
    totals,
    streamingTokenEstimate,
    wsConnected: wsService.isConnected,
    wsReconnecting: wsService.isReconnecting,
    load,
    fillGap,
    refreshTask,
    send,
    cancel,
    clear,
    clearLocalOnly,
    addLocal,
  }
}

export type AgentConsoleState = ReturnType<typeof useAgentConsole>
