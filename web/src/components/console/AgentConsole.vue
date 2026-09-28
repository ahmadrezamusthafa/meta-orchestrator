<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { Play, Pause, RotateCw, Trash2, Copy, Check, Download } from 'lucide-vue-next'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { useAgentConsole } from '../../composables/useAgentConsole'
import type { ConsoleEntry } from '../../types'
import PhaseStatusBadge from '../common/PhaseStatusBadge.vue'
import ConsoleEntryView from './ConsoleEntryView.vue'
import PromptInput from './PromptInput.vue'
import GateSelector from './GateSelector.vue'
import SpinnerLine from './SpinnerLine.vue'
import { SHORTCUTS, SLASH_COMMANDS, helpText } from './consoleCommands'
import {
  downloadText, formatCost, formatDuration, formatTokens, shortId, transcriptToMarkdown,
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

function system(text: string) {
  c.addLocal('system', text)
}

function fail(text: string) {
  c.addLocal('error', text)
}

async function taskAction(label: string, fn: () => Promise<any>, done: string) {
  if (actionPending.value) return
  actionPending.value = true
  try {
    await fn()
    system(done)
    await c.refreshTask()
    emit('task-updated')
  } catch (err: any) {
    fail(`${label} failed: ${err?.message || 'server error'}`)
  } finally {
    actionPending.value = false
  }
}

const runTask = () => taskAction('Run', () => api.executeTask(props.taskId), `⎿  Execution dispatched for ${props.taskId}`)
const pauseTask = () => taskAction('Pause', () => api.pauseTask(props.taskId), '⎿  Session paused — context preserved')
const resumeTask = () => taskAction('Resume', () => api.resumeTask(props.taskId), '⎿  Session resumed')

async function approveGate(feedback?: string) {
  const s = stage.value
  await taskAction(
    'Gate approval',
    () => api.gateApproval(props.taskId, true, feedback || undefined),
    `⎿  Gate approved${s ? ` — advancing past ${s}` : ''}${feedback ? `\n   feedback: ${feedback}` : ''}`,
  )
}

async function rejectGate(feedback: string) {
  gateFeedbackMode.value = false
  await taskAction(
    'Gate rejection',
    () => api.gateApproval(props.taskId, false, feedback || undefined),
    `⎿  Gate rejected${feedback ? `\n   feedback: ${feedback}` : ''}`,
  )
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

function setExpandAll(value: boolean) {
  expandSignal.value = { token: expandSignal.value.token + 1, value }
}

function statusText(): string {
  const t = task.value
  const tot = c.totals.value
  const pad = (k: string) => k.padEnd(10)
  if (!t) return `${pad('Task')}${props.taskId}\n${pad('State')}unknown (task not loaded)`
  const u = t.token_usage
  return [
    `${pad('Task')}${t.id}${t.title ? ` — ${t.title}` : ''}`,
    `${pad('State')}${t.state}`,
    `${pad('Stage')}${t.current_stage_id || '—'}`,
    `${pad('Method')}${t.selected_method || '—'}`,
    `${pad('Model')}${c.currentModel.value || 'auto'}${c.selectedModel.value ? ' (override)' : ''}`,
    `${pad('Session')}${c.sessionId.value || '—'}`,
    `${pad('Task')}${formatTokens(u?.prompt_tokens)} in · ${formatTokens(u?.completion_tokens)} out · ${formatTokens(u?.total_tokens)} total · ${formatCost(u?.estimated_cost_usd)}`,
    `${pad('Console')}${formatTokens(tot.tokens)} tokens · ${formatCost(tot.cost)} across ${tot.responses} response${tot.responses === 1 ? '' : 's'}`,
    `${pad('Turn')}${c.busy.value ? `running${c.activeTurnId.value ? ` (${shortId(c.activeTurnId.value, 14)})` : ''}` : 'idle'}`,
  ].join('\n')
}

function costText(): string {
  const t = c.totals.value
  return [
    `Total cost:            ${formatCost(t.cost)}`,
    `Total duration (API):  ${formatDuration(t.durationMs)}`,
    `Responses:             ${t.responses}`,
    `Usage:                 ${t.prompt.toLocaleString()} input, ${t.completion.toLocaleString()} output, ${t.cached.toLocaleString()} cached`,
  ].join('\n')
}

async function runSlash(input: string) {
  const rawName = input.slice(1).split(/\s+/)[0] || ''
  const name = rawName.toLowerCase()
  const arg = input.slice(1 + rawName.length).trim()
  c.addLocal('user', input)
  switch (name) {
    case 'help':
      system(helpText())
      break
    case 'clear':
      await clearTranscript()
      break
    case 'run':
      await runTask()
      break
    case 'pause':
      await pauseTask()
      break
    case 'resume':
      await resumeTask()
      break
    case 'approve':
      if (!isWaitingGate.value) fail('No gate is waiting for approval.')
      else await approveGate(arg)
      break
    case 'reject':
      if (!isWaitingGate.value) fail('No gate is waiting for approval.')
      else if (arg) await rejectGate(arg)
      else startRejectFeedback()
      break
    case 'status':
      await c.refreshTask()
      system(statusText())
      break
    case 'cost':
      system(costText())
      break
    case 'model':
      if (!arg) {
        system(`Current model: ${c.currentModel.value || 'auto (router default)'}${c.selectedModel.value ? ' (override)' : ''}\nUsage: /model <provider/model> · /model default`)
      } else if (arg === 'default' || arg === 'auto' || arg === 'reset') {
        c.selectedModel.value = ''
        system('⎿  Model override cleared — using router default')
      } else if (!/^[\w.-]+\/[\w.:@/-]+$/.test(arg)) {
        fail(`Invalid model "${arg}". Expected <provider/model>, e.g. claude/claude-3-5-sonnet-20241022`)
      } else {
        c.selectedModel.value = arg
        system(`⎿  Model set to ${arg} for subsequent messages`)
      }
      break
    case 'export':
      exportTranscript()
      system('⎿  Transcript downloaded')
      break
    case 'expand':
      setExpandAll(true)
      break
    case 'collapse':
      setExpandAll(false)
      break
    default: {
      const near = SLASH_COMMANDS.filter(cmd => cmd.name.startsWith(name.slice(0, 2))).map(cmd => `/${cmd.name}`)
      fail(`Unknown command /${name}.${near.length ? ` Did you mean ${near.join(', ')}?` : ''} Type /help for the list.`)
    }
  }
}

async function onSubmit(text: string) {
  atBottom.value = true
  if (gateFeedbackMode.value) {
    await rejectGate(text)
    return
  }
  if (text.startsWith('/')) {
    await runSlash(text)
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

const canPause = computed(() => task.value?.state === 'RUNNING')
const canResume = computed(() => ['SUSPENDED', 'BLOCKED_FRUSTRATION'].includes(task.value?.state || ''))
const canRun = computed(() => !canPause.value && !canResume.value && !['COMPLETED', 'WAITING_DEPENDENCY'].includes(task.value?.state || ''))

const promptPlaceholder = computed(() =>
  gateFeedbackMode.value
    ? 'Tell the agent what to do differently (enter to submit · esc to go back)'
    : 'Try "summarize the failing test" or /help',
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
        <button
          v-if="canRun"
          type="button"
          class="console-btn text-emerald-300"
          :disabled="actionPending"
          title="Execute current stage (/run)"
          @click="runTask"
        >
          <Play class="h-3 w-3" /><span>Run</span>
        </button>
        <button
          v-if="canPause"
          type="button"
          class="console-btn text-amber-300"
          :disabled="actionPending"
          title="Pause session (/pause)"
          @click="pauseTask"
        >
          <Pause class="h-3 w-3" /><span>Pause</span>
        </button>
        <button
          v-if="canResume"
          type="button"
          class="console-btn text-emerald-300"
          :disabled="actionPending"
          title="Resume session (/resume)"
          @click="resumeTask"
        >
          <RotateCw class="h-3 w-3" /><span>Resume</span>
        </button>
        <span class="mx-1 h-4 w-px bg-slate-800" aria-hidden="true"></span>
        <button type="button" class="console-btn" title="Clear transcript (/clear)" aria-label="Clear transcript" @click="clearTranscript">
          <Trash2 class="h-3.5 w-3.5" />
        </button>
        <button type="button" class="console-btn" title="Copy transcript as markdown" aria-label="Copy transcript" @click="copyTranscript">
          <Check v-if="copied" class="h-3.5 w-3.5 text-emerald-400" />
          <Copy v-else class="h-3.5 w-3.5" />
        </button>
        <button type="button" class="console-btn" title="Download transcript (/export)" aria-label="Download transcript" @click="exportTranscript">
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
              <div>cwd: /workspaces/{{ taskId }}</div>
              <div class="pt-1">/help for commands · /run to execute the current stage · ? for shortcuts</div>
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
          <span :class="c.selectedModel.value ? 'text-violet-400/80' : ''" :title="c.selectedModel.value ? 'model override (/model default to reset)' : 'model'">
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
