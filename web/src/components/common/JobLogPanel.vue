<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Terminal, Loader2, CheckCircle2, XCircle, Clock, ChevronDown, ChevronRight } from 'lucide-vue-next'
import type { PackageJob } from '../../types'

const props = defineProps<{
  job: PackageJob
  /** Label before the action, e.g. the package name. */
  label?: string
  /** Start with the log folded (finished jobs). */
  collapsed?: boolean
}>()

const open = ref(!props.collapsed || props.job.state === 'running' || props.job.state === 'queued')
const box = ref<HTMLElement | null>(null)

const active = computed(() => props.job.state === 'queued' || props.job.state === 'running')
const stateClass = computed(() => ({
  queued: 'bg-slate-900 text-slate-300 border-slate-700',
  running: 'bg-sky-950/80 text-sky-300 border-sky-800',
  succeeded: 'bg-emerald-950/80 text-emerald-300 border-emerald-800',
  failed: 'bg-rose-950/80 text-rose-300 border-rose-800',
}[props.job.state] || 'bg-slate-900 text-slate-300 border-slate-700'))

const versionText = computed(() => {
  const { from_version: from, version: to } = props.job
  if (from && to && from !== to) return `${from} → ${to}`
  return to || from || ''
})

// Follow the stream: keep the newest line in view unless the reader scrolled up.
watch(
  () => props.job.log?.length,
  async () => {
    const el = box.value
    const atBottom = !el || el.scrollHeight - el.scrollTop - el.clientHeight < 24
    await nextTick()
    if (atBottom && box.value) box.value.scrollTop = box.value.scrollHeight
  },
)
watch(active, (now) => { if (now) open.value = true })
</script>

<template>
  <div class="rounded-lg border border-slate-800 bg-slate-950/60">
    <button type="button" class="w-full px-3 py-2 flex items-center gap-2 text-left" @click="open = !open">
      <component :is="open ? ChevronDown : ChevronRight" class="w-3.5 h-3.5 text-slate-500 shrink-0" />
      <Terminal class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
      <span class="text-[11px] font-mono text-slate-300 truncate">
        <span v-if="label" class="text-slate-100">{{ label }} · </span>{{ job.action }}<span v-if="versionText" class="text-slate-400"> {{ versionText }}</span>
      </span>
      <span class="ml-auto px-1.5 py-0.5 rounded border text-[10px] font-mono flex items-center gap-1 shrink-0" :class="stateClass">
        <Loader2 v-if="job.state === 'running'" class="w-3 h-3 animate-spin" />
        <Clock v-else-if="job.state === 'queued'" class="w-3 h-3" />
        <CheckCircle2 v-else-if="job.state === 'succeeded'" class="w-3 h-3" />
        <XCircle v-else class="w-3 h-3" />
        {{ job.state }}
      </span>
    </button>
    <div v-if="open" class="px-3 pb-3 space-y-1.5">
      <!-- Log lines are rendered as text only: never v-html. -->
      <div ref="box" class="max-h-48 overflow-y-auto p-2.5 rounded bg-slate-950 border border-slate-800 font-mono text-[11px] text-slate-300 space-y-0.5">
        <div v-if="!job.log?.length" class="text-slate-500">{{ active ? 'Waiting for output…' : 'No output.' }}</div>
        <div v-for="(line, i) in job.log || []" :key="i" class="whitespace-pre-wrap break-words leading-relaxed">{{ line }}</div>
      </div>
      <p v-if="job.message" class="text-[11px]" :class="job.state === 'failed' ? 'text-rose-300' : job.state === 'succeeded' ? 'text-emerald-300' : 'text-slate-400'">
        {{ job.message }}
      </p>
    </div>
  </div>
</template>
