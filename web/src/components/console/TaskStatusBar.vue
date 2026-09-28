<script setup lang="ts">
import { computed } from 'vue'
import { Play, Pause, RotateCw, RefreshCcw } from 'lucide-vue-next'
import type { Task } from '../../types'

/** Default SDLC stage order (mirrors the backend stage controller). */
const STAGES = [
  'prd_discovery', 'atdd_creation', 'techdoc_rfc', 'task_breakdown',
  'task_implementation', 'e2e_validation', 'uat_verification', 'signoff_merge',
]

const props = defineProps<{
  task: Task | null
  /** An agent turn is streaming right now. */
  busy: boolean
  pending: boolean
}>()

const emit = defineEmits<{
  (e: 'run'): void
  (e: 'resume'): void
  (e: 'pause'): void
  (e: 'reset'): void
}>()

type Action = { kind: 'run' | 'resume' | 'pause' | 'reset'; label: string }

interface View {
  tone: 'active' | 'idle' | 'warn' | 'error' | 'done' | 'review'
  icon: string
  title: string
  detail: string
  action?: Action
}

const stage = computed(() => props.task?.current_stage_id || '')
const stageIndex = computed(() => STAGES.indexOf(stage.value))
const meta = computed(() => props.task?.metadata || {})

const view = computed<View | null>(() => {
  const t = props.task
  if (!t) return null
  const s = stage.value
  switch (t.state) {
    case 'RUNNING':
      return props.busy
        ? { tone: 'active', icon: '●', title: `Running ${s}`, detail: 'The agent is working — its activity streams above.', action: { kind: 'pause', label: 'Pause' } }
        : { tone: 'active', icon: '◌', title: `Starting ${s}…`, detail: 'Connecting to the model.', action: { kind: 'pause', label: 'Pause' } }
    case 'PENDING':
      return { tone: 'idle', icon: '○', title: `Ready to run ${s}`, detail: 'Nothing is running. Start the stage, or ask the agent a question first.', action: { kind: 'run', label: 'Run stage' } }
    case 'SUSPENDED':
      return { tone: 'warn', icon: '⏸', title: `Paused at ${s}`, detail: 'Nothing is running. Resume to run this stage again.', action: { kind: 'resume', label: 'Resume' } }
    case 'FAILED':
      return { tone: 'error', icon: '✗', title: `${s} failed`, detail: meta.value.last_error || 'The last run ended with an error.', action: { kind: 'run', label: 'Retry' } }
    case 'BLOCKED_FRUSTRATION':
      return { tone: 'error', icon: '⚠', title: `Blocked at ${s}`, detail: 'Stopped after repeated failures. Give the agent guidance below, or reset and retry.', action: { kind: 'reset', label: 'Reset & retry' } }
    case 'WAITING_DEPENDENCY': {
      const deps = meta.value.unmet_dependencies || (t.dependencies || []).join(', ')
      return { tone: 'idle', icon: '⧗', title: `Waiting for ${deps || 'prerequisite tasks'}`, detail: 'This task starts automatically once they are completed.' }
    }
    case 'WAITING_GATE_APPROVAL':
      return { tone: 'review', icon: '◆', title: `${s} is ready for your review`, detail: 'Read the output above, then approve or reject with feedback.' }
    case 'COMPLETED':
      return { tone: 'done', icon: '✓', title: 'Completed', detail: 'All stages were approved.' }
    default:
      return { tone: 'idle', icon: '○', title: t.state, detail: '' }
  }
})

const toneClass: Record<View['tone'], string> = {
  active: 'border-sky-800/60 bg-sky-950/20 text-sky-300',
  idle: 'border-slate-800 bg-[#0b0f17] text-slate-300',
  warn: 'border-amber-800/60 bg-amber-950/20 text-amber-300',
  error: 'border-rose-800/60 bg-rose-950/20 text-rose-300',
  done: 'border-emerald-800/60 bg-emerald-950/20 text-emerald-300',
  review: 'border-violet-800/60 bg-violet-950/20 text-violet-300',
}

function dotClass(i: number) {
  if (props.task?.state === 'COMPLETED' || i < stageIndex.value) return 'bg-emerald-500'
  if (i === stageIndex.value) {
    if (props.task?.state === 'RUNNING') return 'bg-sky-400 animate-pulse'
    if (props.task?.state === 'FAILED' || props.task?.state === 'BLOCKED_FRUSTRATION') return 'bg-rose-500'
    return 'bg-slate-300'
  }
  return 'bg-slate-700'
}

function act(a: Action) {
  emit(a.kind as any)
}
</script>

<template>
  <div
    v-if="view"
    class="flex items-center gap-3 rounded-md border px-3 py-2 font-mono text-xs"
    :class="toneClass[view.tone]"
    role="status"
    aria-live="polite"
  >
    <span class="w-4 flex-shrink-0 text-center" :class="view.tone === 'active' && busy ? 'animate-pulse' : ''" aria-hidden="true">{{ view.icon }}</span>
    <div class="min-w-0 flex-1">
      <div class="truncate font-semibold">{{ view.title }}</div>
      <div v-if="view.detail" class="truncate text-slate-400" :title="view.detail">{{ view.detail }}</div>
    </div>
    <div
      v-if="stageIndex >= 0"
      class="hidden flex-shrink-0 items-center gap-1 sm:flex"
      :title="`Stage ${stageIndex + 1} of ${STAGES.length}: ${stage}`"
      :aria-label="`Stage ${stageIndex + 1} of ${STAGES.length}`"
    >
      <span v-for="(s, i) in STAGES" :key="s" class="h-1.5 w-1.5 rounded-full" :class="dotClass(i)"></span>
      <span class="ml-1 text-[10px] text-slate-500">{{ stageIndex + 1 }}/{{ STAGES.length }}</span>
    </div>
    <button
      v-if="view.action"
      type="button"
      class="inline-flex h-7 flex-shrink-0 items-center gap-1.5 rounded border border-current/30 px-2.5 font-semibold hover:bg-white/5 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-400 disabled:cursor-not-allowed disabled:opacity-50"
      :disabled="pending"
      @click="act(view.action)"
    >
      <Pause v-if="view.action.kind === 'pause'" class="h-3 w-3" />
      <RotateCw v-else-if="view.action.kind === 'resume'" class="h-3 w-3" />
      <RefreshCcw v-else-if="view.action.kind === 'reset'" class="h-3 w-3" />
      <Play v-else class="h-3 w-3" />
      {{ view.action.label }}
    </button>
  </div>
</template>
