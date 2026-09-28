<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { Trash2, Copy, Check, Download } from 'lucide-vue-next'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { useAgentConsole } from '../../composables/useAgentConsole'
import type { ConsoleEntry } from '../../types'
import PhaseStatusBadge from '../common/PhaseStatusBadge.vue'
import ConsoleEntryView from './ConsoleEntryView.vue'
import PromptInput from './PromptInput.vue'
import GateSelector from './GateSelector.vue'
import SpinnerLine from './SpinnerLine.vue'
import TaskStatusBar from './TaskStatusBar.vue'
import { SHORTCUTS } from './consoleCommands'
import {
  downloadText, formatCost, formatTokens, shortId, transcriptToMarkdown,
} from './consoleFormat'
import type { ExpandSignal } from './expandSignal'

const props = withDefaults(defineProps<{
  taskId: string
  showHeader?: boolean
  autofocus?: boolean
}>(), {
  showHeader: false,
  autofocus: true,
})

const emit = defineEmits<{
  (e: 'task-updated'): void
}>()

const toast = useToastStore()
const c = useAgentConsole(() => props.taskId)

// Per-task input history, kept for the lifetime of the page.
const historyByTask = new Map<string, string[]>()
const history = computed(() => {
  let h = historyByTask.get(props.taskId)
  if (!h) {
    h = []
    historyByTask.set(props.taskId, h)
  }
  return h
})

const scroller = ref<HTMLDivElement | null>(null)
const prompt = ref<InstanceType<typeof PromptInput> | null>(null)
const gate = ref<InstanceType<typeof GateSelector> | null>(null)
const atBottom = ref(true)
const hasNewActivity = ref(false)
const showShortcuts = ref(false)
const expandSignal = ref<ExpandSignal>({ token: 0, value: false })
const gateFeedbackMode = ref(false)
const actionPending = ref(false)
const copied = ref(false)

const task = computed(() => c.task.value)
const stage = computed(() => task.value?.current_stage_id || '')
const isWaitingGate = computed(() => task.value?.state === 'WAITING_GATE_APPROVAL')
const showGate = computed(() => isWaitingGate.value && !gateFeedbackMode.value)

// ---------------------------------------------------------------------------
// Transcript rows: tool_result entries nest under their tool_use (by tool.id)
// ---------------------------------------------------------------------------

const toolResults = computed(() => {
  const map = new Map<string, ConsoleEntry[]>()
  for (const e of c.entries.value) {
    if (e.kind !== 'tool_result' || !e.tool?.id) continue
    const list = map.get(e.tool.id)
    if (list) list.push(e)
    else map.set(e.tool.id, [e])
  }
  return map
})

const toolUseIds = computed(() => {
  const set = new Set<string>()
  for (const e of c.entries.value) if (e.kind === 'tool_use' && e.tool?.id) set.add(e.tool.id)
  return set
})

const rows = computed(() =>
  c.entries.value.filter(e => !(e.kind === 'tool_result' && e.tool?.id && toolUseIds.value.has(e.tool.id))),
)

function resultsFor(e: ConsoleEntry): ConsoleEntry[] | undefined {
  return e.kind === 'tool_use' && e.tool?.id ? toolResults.value.get(e.tool.id) : undefined
}

// ---------------------------------------------------------------------------
// Auto-scroll
// ---------------------------------------------------------------------------

function isNearBottom(el: HTMLElement) {
  return el.scrollHeight - (el.scrollTop + el.clientHeight) < 48
}

function onScroll() {
  const el = scroller.value
  if (!el) return
  atBottom.value = isNearBottom(el)
  if (atBottom.value) hasNewActivity.value = false
}

function scrollToBottom(smooth = false) {
  const el = scroller.value
  if (!el) return
  el.scrollTo({ top: el.scrollHeight, behavior: smooth ? 'smooth' : 'auto' })
  atBottom.value = true
  hasNewActivity.value = false
}

const contentSignature = computed(() => {
  const list = c.entries.value
  let sig = `${list.length}|${c.busy.value}|${showGate.value}`
  for (let i = Math.max(0, list.length - 3); i < list.length; i++) {
    sig += `|${list[i].id}:${(list[i].content || '').length}:${list[i].status || ''}`
  }
  return sig
})

watch(contentSignature, () => {
  if (atBottom.value) nextTick(() => scrollToBottom())
  else hasNewActivity.value = true
})

watch(() => props.taskId, () => {
  gateFeedbackMode.value = false
  atBottom.value = true
  hasNewActivity.value = false
})

watch(showGate, v => {
  if (v) nextTick(() => gate.value?.focus())
})

watch(isWaitingGate, v => {
  if (!v) gateFeedbackMode.value = false
})

onMounted(() => {
  if (props.autofocus) nextTick(() => (showGate.value ? gate.value?.focus() : prompt.value?.focus()))
})

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

function fail(text: string) {
  c.addLocal('error', text)
}

// Task actions: the backend writes the real transcript entries (state change, stage start,
// pause note), so the console only reports failures locally.
async function taskAction(label: string, fn: () => Promise<any>) {
  if (actionPending.value) return
  actionPending.value = true
  try {
    await fn()
    await c.refreshTask()
    emit('task-updated')
  } catch (err: any) {
    fail(`${label} failed: ${err?.message || 'server error'}`)
    await c.refreshTask()
  } finally {
    actionPending.value = false
  }
}

const runTask = () => taskAction('Run', () => api.executeTask(props.taskId))
const pauseTask = () => taskAction('Pause', () => api.pauseTask(props.taskId))
const resumeTask = () => taskAction('Resume', () => api.resumeTask(props.taskId))
const resetTask = () => taskAction('Reset', () => api.resetWorkspace(props.taskId))

async function approveGate() {
  await taskAction('Approval', () => api.gateApproval(props.taskId, true))
}

async function rejectGate(feedback: string) {
  gateFeedbackMode.value = false
  await taskAction('Rejection', () => api.gateApproval(props.taskId, false, feedback))
  nextTick(() => prompt.value?.focus())
}

function startRejectFeedback() {
  gateFeedbackMode.value = true
  nextTick(() => prompt.value?.focus())
}

async function clearTranscript() {
  try {
    await c.clear()
  } catch (err: any) {
    fail(`Clear failed: ${err?.message || 'server error'}`)
  }
}

function markdown() {
  const title = `${props.taskId}${task.value?.title ? ` — ${task.value.title}` : ''}`
  return transcriptToMarkdown(c.entries.value, title)
}

function exportTranscript() {
  downloadText(`${props.taskId}-transcript-${new Date().toISOString().replace(/[:.]/g, '-')}.md`, markdown())
}

async function copyTranscript() {
  try {
    await navigator.clipboard.writeText(markdown())
    copied.value = true
    setTimeout(() => (copied.value = false), 1600)
    toast.info('Transcript copied', `${c.entries.value.length} entries copied as markdown`)
  } catch {
    toast.error('Copy failed', 'Clipboard is not available in this context')
  }
}

async function onSubmit(text: string) {
  atBottom.value = true
  if (gateFeedbackMode.value) {
    await rejectGate(text)
    return
  }
  await c.send(text)
}

function onPromptEscape() {
  if (gateFeedbackMode.value) {
    gateFeedbackMode.value = false
    nextTick(() => gate.value?.focus())
  }
}

function onRootKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && c.busy.value && !e.defaultPrevented) {
    e.preventDefault()
    e.stopPropagation()
    c.cancel()
  }
}

// ---------------------------------------------------------------------------
// Footer / header
// ---------------------------------------------------------------------------

const wsLabel = computed(() => {
  if (c.wsConnected.value) return 'connected'
  if (c.wsReconnecting.value) return 'reconnecting'
  return 'offline'
})

const promptPlaceholder = computed(() =>
  gateFeedbackMode.value
    ? 'Tell the agent what to do differently (enter to submit · esc to go back)'
    : 'Ask the agent about this task…',
)
</script>

<template>
  <div
    class="agent-console flex h-full min-h-0 flex-col bg-[#07090e] text-slate-200"
    @keydown="onRootKeydown"
  >
    <!-- Header -->
    <div
      v-if="showHeader"
      class="flex h-10 flex-shrink-0 items-center justify-between gap-2 border-b border-slate-800/80 bg-[#0b0f17] px-3 font-mono text-xs"
    >
      <div class="flex min-w-0 items-center gap-2">
        <span class="font-semibold text-slate-100">{{ taskId }}</span>
        <PhaseStatusBadge v-if="task" :state="task.state" />
        <span v-if="stage" class="truncate text-slate-500">{{ stage }}</span>
      </div>
      <div class="flex flex-shrink-0 items-center gap-1">
        <button type="button" class="console-btn" title="Clear transcript" aria-label="Clear transcript" @click="clearTranscript">
          <Trash2 class="h-3.5 w-3.5" />
        </button>
        <button type="button" class="console-btn" title="Copy transcript as markdown" aria-label="Copy transcript" @click="copyTranscript">
          <Check v-if="copied" class="h-3.5 w-3.5 text-emerald-400" />
          <Copy v-else class="h-3.5 w-3.5" />
        </button>
        <button type="button" class="console-btn" title="Download transcript" aria-label="Download transcript" @click="exportTranscript">
          <Download class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>

    <!-- Transcript -->
    <div class="relative min-h-0 flex-1">
      <div
        ref="scroller"
        class="h-full overflow-y-auto px-4 py-3"
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        :aria-busy="c.busy.value"
        aria-label="Agent activity transcript"
        @scroll="onScroll"
      >
        <div class="mx-auto max-w-5xl space-y-2.5">
          <!-- Welcome -->
          <div
            v-if="!c.loading.value && !rows.length"
            class="inline-block max-w-full rounded-lg border border-orange-400/40 px-4 py-3 font-mono text-sm"
          >
            <div><span class="text-orange-400">✻</span> Welcome to the <span class="font-semibold">agent console</span></div>
            <div class="mt-2 space-y-0.5 text-xs text-slate-500">
              <div>
                <span class="text-slate-400">{{ taskId }}</span>
                <span v-if="task?.title"> — {{ task.title }}</span>
              </div>
              <div class="pt-1">Run the current stage from the bar below, or ask the agent a question. Press ? for shortcuts.</div>
            </div>
          </div>

          <div v-if="c.loading.value" class="font-mono text-xs text-slate-500">Loading activity…</div>
          <div v-else-if="c.loadError.value" class="font-mono text-xs text-rose-400" role="alert">
            ⏺ {{ c.loadError.value }}
            <button type="button" class="ml-2 underline hover:text-rose-300" @click="c.load()">retry</button>
          </div>

          <ConsoleEntryView
            v-for="e in rows"
            :key="e.id"
            :entry="e"
            :results="resultsFor(e)"
            :expand-signal="expandSignal"
          />

          <SpinnerLine
            v-if="c.busy.value"
            :started-at="c.turnStartedAt.value"
            :tokens="c.streamingTokenEstimate.value"
          />
        </div>
      </div>

      <button
        v-if="hasNewActivity && !atBottom"
        type="button"
        class="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full border border-slate-700 bg-slate-900/95 px-3 py-1 font-mono text-[11px] text-slate-300 shadow-lg hover:bg-slate-800 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-400"
        @click="scrollToBottom(true)"
      >
        ↓ new activity
      </button>
    </div>

    <!-- Input region -->
    <div class="flex-shrink-0 border-t border-slate-900 bg-[#07090e] px-4 pb-2 pt-2">
      <div class="mx-auto max-w-5xl space-y-2">
        <TaskStatusBar
          :task="task"
          :busy="c.busy.value"
          :pending="actionPending"
          @run="runTask"
          @resume="resumeTask"
          @pause="pauseTask"
          @reset="resetTask"
        />

        <GateSelector
          v-if="showGate"
          ref="gate"
          :stage="stage"
          :pending="actionPending"
          @approve="approveGate()"
          @reject="startRejectFeedback"
        />

        <div
          v-if="showShortcuts"
          class="grid grid-cols-1 gap-x-6 gap-y-0.5 rounded-md border border-slate-800 bg-[#0b0f17] px-3 py-2 font-mono text-[11px] sm:grid-cols-2"
        >
          <div v-for="s in SHORTCUTS" :key="s.keys" class="flex gap-3">
            <span class="w-24 flex-shrink-0 text-slate-300">{{ s.keys }}</span>
            <span class="text-slate-500">{{ s.description }}</span>
          </div>
        </div>

        <PromptInput
          ref="prompt"
          :busy="c.busy.value || c.sending.value"
          :history="history"
          :mode="gateFeedbackMode ? 'feedback' : 'default'"
          :placeholder="promptPlaceholder"
          :hint="gateFeedbackMode ? `rejecting gate ${stage} — your feedback is sent to the agent` : ''"
          @submit="onSubmit"
          @cancel="c.cancel()"
          @escape="onPromptEscape"
          @shortcuts="showShortcuts = !showShortcuts"
        />

        <!-- Footer status line -->
        <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 px-1 font-mono text-[11px] text-slate-600">
          <span :class="c.selectedModel.value ? 'text-violet-400/80' : ''" title="model that answered last">
            {{ c.currentModel.value || 'auto' }}
          </span>
          <span aria-hidden="true">·</span>
          <span :title="c.sessionId.value || 'no session yet'">session {{ shortId(c.sessionId.value) }}</span>
          <span aria-hidden="true">·</span>
          <span title="tokens across response entries">Σ {{ formatTokens(c.totals.value.tokens) }} tokens</span>
          <span aria-hidden="true">·</span>
          <span>{{ formatCost(c.totals.value.cost) }}</span>
          <span aria-hidden="true">·</span>
          <span class="inline-flex items-center gap-1">
            <span
              class="inline-block h-1.5 w-1.5 rounded-full"
              :class="c.wsConnected.value ? 'bg-emerald-500' : c.wsReconnecting.value ? 'animate-pulse bg-amber-500' : 'bg-rose-500'"
            ></span>
            ws {{ wsLabel }}
          </span>
          <button
            type="button"
            class="ml-auto rounded hover:text-slate-400 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
            :aria-expanded="showShortcuts"
            @click="showShortcuts = !showShortcuts"
          >
            ? for shortcuts
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.console-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  height: 1.5rem;
  padding: 0 0.45rem;
  border-radius: 0.3rem;
  color: #94a3b8;
  border: 1px solid transparent;
  transition: background-color 0.12s, color 0.12s, border-color 0.12s;
}
.console-btn:hover:not(:disabled) {
  background: #111827;
  border-color: #1e293b;
  color: #e2e8f0;
}
.console-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.console-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 1px #64748b;
}
</style>
