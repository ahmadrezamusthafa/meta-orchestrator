<script setup lang="ts">
import type { BenchmarkCellDTO } from '../../types'
import { X, CheckCircle2, AlertTriangle } from 'lucide-vue-next'
import { methodStyle, canonicalMethod, fpvrHealth, HEALTH_TEXT } from '../analytics/analyticsFormat'
import { stageName } from '../../composables/taskLifecycle'

defineProps<{ cell: BenchmarkCellDTO }>()
defineEmits<{ (e: 'close'): void }>()
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm" @click.self="$emit('close')">
    <div class="w-full max-w-md max-h-[90vh] overflow-y-auto bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-5 space-y-4" role="dialog" aria-modal="true" aria-labelledby="bench-cell-title">
      <div class="flex items-start justify-between gap-3 pb-2 border-b border-slate-800">
        <div>
          <h3 id="bench-cell-title" class="text-sm font-bold text-slate-100">{{ stageName(cell.stage_id) }} · {{ cell.complexity }}</h3>
          <p v-if="cell.benchmark_stage && cell.benchmark_stage !== cell.stage_id" class="text-[11px] text-slate-500">
            Measured on {{ stageName(cell.benchmark_stage) }}
          </p>
        </div>
        <button type="button" class="p-1 rounded text-slate-500 hover:text-slate-200 hover:bg-slate-800" aria-label="Close" @click="$emit('close')">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div
        class="flex items-start gap-2 p-2.5 rounded-lg border text-xs"
        :class="cell.applied ? 'bg-emerald-950/40 border-emerald-800 text-emerald-200' : 'bg-amber-950/40 border-amber-800 text-amber-200'"
      >
        <CheckCircle2 v-if="cell.applied" class="w-4 h-4 shrink-0 mt-0.5" />
        <AlertTriangle v-else class="w-4 h-4 shrink-0 mt-0.5" />
        <span v-if="cell.applied">Routing uses this result for this stage and complexity.</span>
        <span v-else>Not used by routing yet: {{ cell.not_applied_reason }}. The routing policy applies until there is enough evidence.</span>
      </div>

      <dl class="grid grid-cols-[auto,1fr] gap-x-4 gap-y-1.5 text-xs">
        <dt class="text-slate-400">Winning method</dt>
        <dd class="font-semibold" :class="methodStyle(cell.optimal_method).text">{{ canonicalMethod(cell.optimal_method) }}</dd>
        <dt class="text-slate-400">Model</dt>
        <dd class="font-mono text-slate-200 break-all">{{ cell.winning_model || '—' }}<template v-if="cell.model_tier"> ({{ cell.model_tier }})</template></dd>
        <dt class="text-slate-400">First pass</dt>
        <dd class="font-mono font-bold" :class="HEALTH_TEXT[fpvrHealth(cell.fpvr_percent)]">{{ cell.fpvr_percent }}%</dd>
        <dt class="text-slate-400">Runs measured</dt>
        <dd class="font-mono text-slate-200">{{ cell.samples ?? 0 }}</dd>
        <dt class="text-slate-400">Avg cost / run</dt>
        <dd class="font-mono text-slate-200">${{ (cell.avg_cost_usd ?? 0).toFixed(4) }}</dd>
        <dt class="text-slate-400">Avg tokens</dt>
        <dd class="font-mono text-slate-200">{{ cell.avg_tokens.toLocaleString() }}</dd>
        <dt class="text-slate-400">Avg duration</dt>
        <dd class="font-mono text-slate-200">{{ cell.avg_duration_s }}s</dd>
        <template v-if="cell.runner_up">
          <dt class="text-slate-400">Runner-up</dt>
          <dd class="font-mono text-slate-200 break-all">{{ cell.runner_up }}</dd>
        </template>
        <template v-if="cell.pareto_front?.length">
          <dt class="text-slate-400">Not beaten on every metric</dt>
          <dd class="font-mono text-slate-200 break-all">{{ cell.pareto_front.join(', ') }}</dd>
        </template>
      </dl>

      <p class="text-[11px] text-slate-400 leading-relaxed">
        The winner has the best score among candidates no other option beats on every metric: the lower bound of its first-pass
        rate's 95% confidence interval, minus cost and duration penalties. Failed or errored runs count against a candidate.
      </p>

      <div class="pt-3 border-t border-slate-800 flex justify-end">
        <button type="button" class="h-8 px-4 rounded bg-slate-800 hover:bg-slate-700 text-xs text-slate-200" @click="$emit('close')">Close</button>
      </div>
    </div>
  </div>
</template>
