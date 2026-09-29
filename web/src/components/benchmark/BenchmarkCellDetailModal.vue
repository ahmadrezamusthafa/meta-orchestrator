<script setup lang="ts">
import { computed } from 'vue'
import type { BenchmarkCellDTO } from '../../types'
import { X } from 'lucide-vue-next'
import { methodStyle, fpvrHealth, HEALTH_TEXT, isMeasured } from '../analytics/analyticsFormat'

const props = defineProps<{
  cell: BenchmarkCellDTO
}>()

defineEmits<{
  (e: 'close'): void
}>()

const measured = computed(() => isMeasured(props.cell))
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-slate-800">
        <div>
          <h3 class="text-sm font-bold text-slate-100">Benchmark Cell Analytics</h3>
          <span class="text-xs text-emerald-400 font-mono">{{ cell.stage_id }} [{{ cell.complexity }}]</span>
        </div>
        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-3 font-mono text-xs">
        <div class="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1.5">
          <div class="text-slate-400">
            {{ measured ? 'Winning Method' : 'Policy Method' }}:
            <span class="font-bold" :class="methodStyle(cell.optimal_method).text">{{ cell.optimal_method }}</span>
          </div>
          <div v-if="cell.model_tier" class="text-slate-400">Model Tier: <span class="text-slate-200">{{ cell.model_tier }}</span></div>
          <template v-if="measured">
            <div class="text-slate-400">Winning Model: <span class="text-slate-200">{{ cell.winning_model || '—' }}</span></div>
            <div class="text-slate-400">
              First-Pass Verification:
              <span class="font-bold" :class="HEALTH_TEXT[fpvrHealth(cell.fpvr_percent)]">{{ cell.fpvr_percent }}%</span>
            </div>
            <div class="text-slate-400">Average Token Burn: <span class="text-sky-400">{{ cell.avg_tokens.toLocaleString() }} tokens</span></div>
            <div class="text-slate-400">Mean Duration: <span class="text-slate-200">{{ cell.avg_duration_s }}s</span></div>
            <div v-if="cell.avg_cost_usd !== undefined" class="text-slate-400">Avg Cost / Run: <span class="text-sky-400">${{ cell.avg_cost_usd.toFixed(4) }}</span></div>
            <div v-if="cell.score !== undefined" class="text-slate-400">Composite Score: <span class="text-sky-400">{{ cell.score.toFixed(2) }}</span></div>
            <div v-if="cell.samples !== undefined" class="text-slate-400">Samples: <span class="text-sky-400">{{ cell.samples }}</span></div>
          </template>
          <div v-else class="text-amber-400">Not yet benchmarked &mdash; no FPVR, cost or token data.</div>
        </div>

        <p v-if="measured" class="text-[11px] font-sans text-slate-400 leading-relaxed">
          Shadow Benchmarking periodically replays past tasks in the background across alternative methods to verify that this pairing retains the maximum pass rate and lowest cost.
        </p>
        <p v-else class="text-[11px] font-sans text-slate-400 leading-relaxed">
          This cell shows the built-in best-practice policy. The router keeps its own heuristics here until a shadow benchmark
          (enabled with <span class="font-mono text-slate-300">MO_SHADOW_ENABLED=true</span>) replays a completed task for this stage and complexity.
        </p>
      </div>

      <div class="pt-3 border-t border-slate-800 flex justify-end">
        <button
          @click="$emit('close')"
          type="button"
          class="h-8 px-4 rounded bg-slate-800 hover:bg-slate-700 text-xs text-slate-200"
        >
          Close
        </button>
      </div>
    </div>
  </div>
</template>
