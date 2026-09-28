<script setup lang="ts">
import { computed } from 'vue'
import type { RepoStabilityDTO } from '../../types'
import { Activity, AlertTriangle } from 'lucide-vue-next'
import { flakinessHealth, fmtInt, HEALTH_TEXT, HEALTH_BG, HEALTH_BAR } from './analyticsFormat'

const props = defineProps<{
  stability: RepoStabilityDTO[]
}>()

// Flakiest first.
const rows = computed(() => [...(props.stability ?? [])].sort((a, b) => b.flakiness_index - a.flakiness_index))

const breachCount = computed(() => rows.value.filter((r) => flakinessHealth(r.flakiness_index) === 'breach').length)

function pct(idx: number): number {
  return Math.min(100, Math.max(0, (idx || 0) * 100))
}
</script>

<template>
  <section class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">Test Stability Index</h3>
        <span class="text-[11px] text-slate-400">ATDD flakiness per repository (failure loops / runs)</span>
      </div>
      <div class="flex items-center gap-2">
        <span
          v-if="breachCount"
          class="flex items-center gap-1 px-1.5 py-0.5 rounded bg-rose-950/40 border border-rose-800 text-[10px] font-mono text-rose-400"
        >
          <AlertTriangle class="w-3 h-3" /> {{ breachCount }} flaky
        </span>
        <Activity class="w-4 h-4 text-emerald-400" />
      </div>
    </div>

    <div v-if="!rows.length" class="py-8 text-center text-xs text-slate-500">
      No test runs recorded for this window.
    </div>

    <ul v-else class="space-y-2">
      <li
        v-for="r in rows"
        :key="r.repo"
        class="p-2.5 rounded-lg border"
        :class="flakinessHealth(r.flakiness_index) === 'pass' ? 'bg-slate-950/60 border-slate-800' : HEALTH_BG[flakinessHealth(r.flakiness_index)]"
      >
        <div class="flex items-center justify-between gap-2 text-xs">
          <span class="font-medium text-slate-200 truncate">{{ r.repo }}</span>
          <span class="font-mono text-xs font-bold" :class="HEALTH_TEXT[flakinessHealth(r.flakiness_index)]">
            {{ r.flakiness_index.toFixed(2) }}
          </span>
        </div>
        <div class="mt-1.5 h-1.5 rounded-full bg-slate-800 overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-700"
            :class="HEALTH_BAR[flakinessHealth(r.flakiness_index)]"
            :style="{ width: `${pct(r.flakiness_index)}%` }"
          ></div>
        </div>
        <div class="mt-1.5 flex flex-wrap gap-x-4 gap-y-0.5 text-[10px] text-slate-500">
          <span>runs <span class="text-sky-400 font-mono text-xs">{{ fmtInt(r.runs) }}</span></span>
          <span>failure loops <span class="font-mono text-xs" :class="r.failure_loops > 0 ? HEALTH_TEXT[flakinessHealth(r.flakiness_index)] : 'text-sky-400'">{{ fmtInt(r.failure_loops) }}</span></span>
          <span>avg test iterations <span class="text-sky-400 font-mono text-xs">{{ r.avg_test_iterations.toFixed(1) }}</span></span>
        </div>
      </li>
    </ul>
  </section>
</template>
