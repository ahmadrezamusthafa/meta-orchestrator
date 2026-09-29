<script setup lang="ts">
import { computed } from 'vue'
import { Play, Pause, RotateCw, RefreshCcw } from 'lucide-vue-next'
import type { Task } from '../../types'
import { taskLifecycle, stageName, STAGES, type Tone } from '../../composables/taskLifecycle'

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

const stage = computed(() => props.task?.current_stage_id || '')
const stageIndex = computed(() => STAGES.indexOf(stage.value as (typeof STAGES)[number]))
const life = computed(() => taskLifecycle(props.task, { busy: props.busy }))
// Review happens in the gate selector right below the bar, so the bar offers no button for it.
const action = computed(() => (life.value?.primary?.kind === 'review' || life.value?.primary?.kind === 'assign' || life.value?.primary?.kind === 'approve' ? undefined : life.value?.primary))

const ICON: Record<Tone, string> = { active: '●', idle: '○', warn: '⏸', error: '✗', done: '✓', review: '◆', waiting: '⧗' }

const toneClass: Record<Tone, string> = {
  active: 'border-sky-800/60 bg-sky-950/20 text-sky-300',
  idle: 'border-slate-800 bg-[#0b0f17] text-slate-300',
  warn: 'border-amber-800/60 bg-amber-950/20 text-amber-300',
  error: 'border-rose-800/60 bg-rose-950/20 text-rose-300',
  done: 'border-emerald-800/60 bg-emerald-950/20 text-emerald-300',
  review: 'border-violet-800/60 bg-violet-950/20 text-violet-300',
  waiting: 'border-orange-800/60 bg-orange-950/20 text-orange-300',
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

function act() {
  const k = action.value?.kind
  if (k === 'run') emit('run')
  else if (k === 'resume') emit('resume')
  else if (k === 'pause') emit('pause')
  else if (k === 'reset') emit('reset')
}
</script>

<template>
  <div
    v-if="life"
    class="flex items-center gap-3 rounded-md border px-3 py-2 font-mono text-xs"
    :class="toneClass[life.tone]"
    role="status"
    aria-live="polite"
  >
    <span class="w-4 flex-shrink-0 text-center" :class="life.tone === 'active' && busy ? 'animate-pulse' : ''" aria-hidden="true">{{ ICON[life.tone] }}</span>
    <div class="min-w-0 flex-1">
      <div class="truncate font-semibold">{{ life.title }}</div>
      <div v-if="life.detail" class="truncate text-slate-400" :title="life.detail">{{ life.detail }}</div>
    </div>
    <div
      v-if="stageIndex >= 0"
      class="hidden flex-shrink-0 items-center gap-1 sm:flex"
      :title="`Stage ${stageIndex + 1} of ${STAGES.length}: ${stageName(stage)}`"
      :aria-label="`Stage ${stageIndex + 1} of ${STAGES.length}`"
    >
      <span v-for="(s, i) in STAGES" :key="s" class="h-1.5 w-1.5 rounded-full" :class="dotClass(i)"></span>
      <span class="ml-1 text-[10px] text-slate-500">{{ stageIndex + 1 }}/{{ STAGES.length }}</span>
    </div>
    <button
      v-if="action"
      type="button"
      class="inline-flex h-7 flex-shrink-0 items-center gap-1.5 rounded border border-current/30 px-2.5 font-semibold hover:bg-white/5 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-400 disabled:cursor-not-allowed disabled:opacity-50"
      :disabled="pending"
      :title="action.hint"
      @click="act"
    >
      <Pause v-if="action.kind === 'pause'" class="h-3 w-3" />
      <RotateCw v-else-if="action.kind === 'resume'" class="h-3 w-3" />
      <RefreshCcw v-else-if="action.kind === 'reset'" class="h-3 w-3" />
      <Play v-else class="h-3 w-3" />
      {{ action.label }}
    </button>
  </div>
</template>
