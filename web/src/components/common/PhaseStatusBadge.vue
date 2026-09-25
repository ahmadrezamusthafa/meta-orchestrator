<script setup lang="ts">
import type { TaskState } from '../../types'

defineProps<{
  state: TaskState
}>()
</script>

<template>
  <span
    class="px-2 py-0.5 rounded text-[10px] font-mono font-medium tracking-wide uppercase"
    :class="{
      'bg-emerald-950/80 text-emerald-300 border border-emerald-800/60': state === 'RUNNING',
      'bg-rose-950/80 text-rose-300 border border-rose-800/80 animate-pulse-subtle': state === 'BLOCKED_FRUSTRATION',
      'bg-amber-950/80 text-amber-300 border border-amber-800/60': state === 'WAITING_GATE_APPROVAL',
      'bg-blue-950/80 text-blue-300 border border-blue-800/60': state === 'COMPLETED',
      'bg-slate-800 text-slate-300 border border-slate-700': state === 'PENDING' || state === 'SUSPENDED',
      'bg-red-950 text-red-400 border border-red-900': state === 'FAILED',
    }"
  >
    {{ state.replace('_', ' ') }}
  </span>
</template>
