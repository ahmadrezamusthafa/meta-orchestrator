<script setup lang="ts">
import { ref, computed } from 'vue'
import type { BenchmarkCellDTO, BenchmarksResponseDTO, Complexity } from '../../types'
import BenchmarkCellDetailModal from '../benchmark/BenchmarkCellDetailModal.vue'
import { FlaskConical } from 'lucide-vue-next'
import { STAGES, stageName } from '../../composables/taskLifecycle'
import { methodStyle, canonicalMethod, fmtUsd, fpvrHealth, HEALTH_TEXT } from './analyticsFormat'

// Shadow-benchmark results: only measured cells are shown, each marked with whether routing uses
// it. The routing policy for unmeasured cells is under Settings → Model Routing → What will run.
const props = defineProps<{ data: BenchmarksResponseDTO }>()

const selectedCell = ref<BenchmarkCellDTO | null>(null)
const COMPLEXITIES: Complexity[] = ['LOW', 'MEDIUM', 'HIGH', 'SYSTEM']
const stages = computed(() => (props.data.stages?.length ? props.data.stages : [...STAGES]))
const byKey = computed(() => new Map((props.data.matrix ?? []).map((c) => [`${c.stage_id}|${c.complexity}`, c])))
const measured = computed(() => props.data.measured_cells ?? props.data.matrix?.length ?? 0)
const applied = computed(() => props.data.applied_cells ?? 0)
const total = computed(() => props.data.total_cells || stages.value.length * COMPLEXITIES.length)
const minSamples = computed(() => props.data.min_samples ?? 3)
const minFpvr = computed(() => Math.round((props.data.min_fpvr ?? 0.5) * 100))

function cell(stage: string, cx: string) {
  return byKey.value.get(`${stage}|${cx}`)
}

function fmtTimestamp(ts?: string): string {
  if (!ts) return ''
  const d = new Date(ts)
  return isNaN(d.getTime()) ? ts : d.toLocaleString()
}
</script>

<template>
  <section class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex flex-wrap items-start justify-between gap-2 pb-2 border-b border-slate-800">
      <div class="space-y-0.5">
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide flex items-center gap-1.5">
          <FlaskConical class="w-4 h-4 text-emerald-400" />
          Benchmark results
        </h3>
        <p class="text-[11px] text-slate-400 max-w-3xl leading-relaxed">
          The shadow benchmark replays completed tasks on other methods and models and records which one passes first time most
          cheaply. Routing uses a result once it has at least {{ minSamples }} runs and a first-pass rate of {{ minFpvr }}% or more;
          everywhere else the routing policy applies.
        </p>
      </div>
      <div class="text-[11px] font-mono text-right space-y-0.5">
        <div><span class="text-slate-100">{{ measured }}</span><span class="text-slate-500"> of {{ total }} measured</span></div>
        <div><span class="text-emerald-400">{{ applied }}</span><span class="text-slate-500"> used by routing</span></div>
        <div v-if="data.generated_at" class="text-slate-500">{{ fmtTimestamp(data.generated_at) }}</div>
      </div>
    </div>

    <div v-if="measured === 0" class="py-6 px-4 text-center space-y-1.5">
      <p class="text-xs text-slate-300">No benchmark results yet, so routing uses its policy for every stage.</p>
      <p class="text-[11px] text-slate-500">
        <template v-if="data.shadow_enabled">The benchmark daemon is running; results appear after it replays a completed task.</template>
        <template v-else>
          Start the daemon with <span class="font-mono text-slate-300">MO_SHADOW_ENABLED=true</span> to benchmark completed tasks.
          Each replay makes paid model calls.
        </template>
      </p>
    </div>

    <div v-else class="border border-slate-800 rounded-lg overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
          <tr>
            <th scope="col" class="p-2.5 font-medium min-w-[150px]">Stage</th>
            <th v-for="c in COMPLEXITIES" :key="c" scope="col" class="p-2.5 font-mono font-medium min-w-[150px]">{{ c }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="s in stages" :key="s">
            <th scope="row" class="p-2.5 font-normal text-slate-200 whitespace-nowrap align-top">{{ stageName(s) }}</th>
            <td v-for="c in COMPLEXITIES" :key="c" class="p-1.5 align-top">
              <button
                v-if="cell(s, c)"
                type="button"
                class="w-full p-1.5 rounded border text-left space-y-0.5 transition-colors hover:border-slate-500"
                :class="cell(s, c)!.applied
                  ? [methodStyle(cell(s, c)!.optimal_method).bg, methodStyle(cell(s, c)!.optimal_method).border]
                  : 'border-dashed border-amber-700/70 bg-slate-950/40'"
                :aria-label="`${stageName(s)} ${c}: ${canonicalMethod(cell(s, c)!.optimal_method)}, ${cell(s, c)!.applied ? 'used by routing' : 'not used yet'}`"
                @click="selectedCell = cell(s, c) || null"
              >
                <div class="flex items-center justify-between gap-1 text-[11px]">
                  <span class="font-semibold" :class="methodStyle(cell(s, c)!.optimal_method).text">{{ canonicalMethod(cell(s, c)!.optimal_method) }}</span>
                  <span class="font-mono font-bold" :class="HEALTH_TEXT[fpvrHealth(cell(s, c)!.fpvr_percent)]">{{ cell(s, c)!.fpvr_percent }}%</span>
                </div>
                <div class="text-[10px] font-mono text-slate-300 break-all leading-snug">{{ cell(s, c)!.winning_model.split('/').pop() }}</div>
                <div class="flex flex-wrap items-center justify-between gap-1 text-[10px] text-slate-500">
                  <span>{{ cell(s, c)!.samples }} runs · {{ fmtUsd(cell(s, c)!.avg_cost_usd ?? 0) }}</span>
                  <span v-if="!cell(s, c)!.applied" class="text-amber-400">{{ cell(s, c)!.not_applied_reason }}</span>
                </div>
              </button>
              <div v-else class="p-1.5 text-[10px] text-slate-600" aria-label="Not measured">—</div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <p v-if="measured > 0" class="text-[10px] text-slate-500">
      Solid cells are used by routing; dashed cells need more evidence. UAT verification shares the E2E validation results.
    </p>

    <BenchmarkCellDetailModal v-if="selectedCell" :cell="selectedCell" @close="selectedCell = null" />
  </section>
</template>
