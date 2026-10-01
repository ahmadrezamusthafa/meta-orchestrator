<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { RouteMatrixCellDTO, BenchmarkCellDTO, ModelSuggestion, Complexity } from '../../types'
import { STAGES, stageName } from '../../composables/taskLifecycle'
import { METHODS, methodStyle, canonicalMethod } from '../analytics/analyticsFormat'
import { Lightbulb, X } from 'lucide-vue-next'

// What will run: the router's decision for every pipeline stage at every complexity, under the
// (possibly unsaved) mode and model list. Measured benchmark cells are marked with their evidence.
const props = defineProps<{
  cells: RouteMatrixCellDTO[]
  benchmarks?: BenchmarkCellDTO[]
  complexityAware: boolean
  loading?: boolean
}>()
const emit = defineEmits<{ (e: 'apply-suggestion', s: ModelSuggestion): void }>()

const COMPLEXITIES: { id: Complexity; hint: string }[] = [
  { id: 'LOW', hint: 'small, contained' },
  { id: 'MEDIUM', hint: 'one service' },
  { id: 'HIGH', hint: 'cross-service, risky' },
  { id: 'SYSTEM', hint: 'infra, CI, migrations' },
]

const TIER_LABEL: Record<string, string> = { tier1: 'Tier 1', tier2: 'Tier 2', tier3: 'Tier 3' }
const SOURCE: Record<string, { label: string; cls: string; help: string }> = {
  benchmark: { label: 'benchmark', cls: 'text-emerald-300 border-emerald-700/70 bg-emerald-950/50', help: 'A measured shadow-benchmark winner replaced the policy here.' },
  calibrated: { label: 'calibrated', cls: 'text-sky-300 border-sky-700/70 bg-sky-950/50', help: 'Run history showed a cheaper tier is reliable for this task type.' },
  rule: { label: 'your rule', cls: 'text-violet-300 border-violet-700/70 bg-violet-950/50', help: 'A custom routing rule matched.' },
}

const byKey = computed(() => new Map(props.cells.map((c) => [`${c.stage_id}|${c.complexity}`, c])))
const benchByKey = computed(() => new Map((props.benchmarks ?? []).map((b) => [`${b.stage_id}|${b.complexity}`, b])))
const stages = computed(() => STAGES.filter((s) => props.cells.some((c) => c.stage_id === s)))

const selected = ref<string | null>(null)
const selectedCell = computed(() => (selected.value ? byKey.value.get(selected.value) : undefined))
const selectedBench = computed(() => (selected.value ? benchByKey.value.get(selected.value) : undefined))
watch(() => props.cells, () => { if (selected.value && !byKey.value.has(selected.value)) selected.value = null })

// Show "claude-opus-5-5" rather than "claude/claude-opus-5-5"; the provider is in the details.
function shortModel(m: string) {
  const i = m.indexOf('/')
  return i >= 0 ? m.slice(i + 1) : m
}

function sourceTag(c: RouteMatrixCellDTO) {
  const d = c.decision
  if (d.method_source === 'benchmark' || d.tier_source === 'benchmark') return SOURCE.benchmark
  if (d.tier_source === 'calibrated') return SOURCE.calibrated
  if (d.method_source === 'rule') return SOURCE.rule
  return null
}

// One entry per distinct suggestion, so the same "add Opus" tip is not repeated per cell.
const suggestions = computed(() => {
  const seen = new Map<string, { s: ModelSuggestion; cells: number }>()
  for (const c of props.cells) {
    const s = c.decision.suggestion
    if (!s) continue
    const k = `${s.action}|${s.model}`
    const e = seen.get(k)
    if (e) e.cells++
    else seen.set(k, { s, cells: 1 })
  }
  return [...seen.values()]
})

function suggestionLabel(a: ModelSuggestion['action']) {
  return a === 'add' ? 'Add to list' : a === 'enable' ? 'Enable' : 'How to connect'
}
</script>

<template>
  <div class="space-y-3">
    <p v-if="!complexityAware" class="text-[11px] text-amber-300 bg-amber-950/30 border border-amber-800/50 rounded-lg px-3 py-2">
      This mode uses the same model at every complexity. Only the method changes by stage.
      Switch to Tiered Best Practice to run low-complexity work on a cheaper model.
    </p>

    <div class="border border-slate-800 rounded-xl overflow-x-auto bg-slate-950/60" :class="loading ? 'opacity-60' : ''">
      <table class="w-full text-left text-xs border-collapse">
        <thead class="text-[11px] text-slate-400 border-b border-slate-800">
          <tr>
            <th scope="col" class="px-3 py-2 font-medium min-w-[150px]">Stage</th>
            <th v-for="cx in COMPLEXITIES" :key="cx.id" scope="col" class="px-2 py-2 font-medium min-w-[150px]">
              <div class="font-mono text-slate-200">{{ cx.id }}</div>
              <div class="text-[10px] font-normal text-slate-500">{{ cx.hint }}</div>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/70">
          <tr v-for="st in stages" :key="st">
            <th scope="row" class="px-3 py-1.5 font-normal text-slate-200 align-top whitespace-nowrap">{{ stageName(st) }}</th>
            <td v-for="cx in COMPLEXITIES" :key="cx.id" class="px-1.5 py-1.5 align-top">
              <button
                v-if="byKey.get(`${st}|${cx.id}`)"
                type="button"
                class="w-full text-left rounded-lg border px-2 py-1.5 space-y-0.5 transition-colors"
                :class="[
                  selected === `${st}|${cx.id}` ? 'ring-1 ring-sky-500 border-sky-600' : 'hover:border-slate-500',
                  methodStyle(byKey.get(`${st}|${cx.id}`)!.decision.method).bg,
                  byKey.get(`${st}|${cx.id}`)!.decision.suggestion ? 'border-dashed border-amber-600/80' : methodStyle(byKey.get(`${st}|${cx.id}`)!.decision.method).border,
                ]"
                :aria-pressed="selected === `${st}|${cx.id}`"
                :aria-label="`${stageName(st)}, ${cx.id}: ${canonicalMethod(byKey.get(`${st}|${cx.id}`)!.decision.method)} on ${byKey.get(`${st}|${cx.id}`)!.decision.model}`"
                @click="selected = selected === `${st}|${cx.id}` ? null : `${st}|${cx.id}`"
              >
                <div class="flex items-center justify-between gap-1">
                  <span class="text-[11px] font-semibold" :class="methodStyle(byKey.get(`${st}|${cx.id}`)!.decision.method).text">
                    {{ canonicalMethod(byKey.get(`${st}|${cx.id}`)!.decision.method) }}
                  </span>
                </div>
                <div class="text-[11px] font-mono text-slate-100 break-all leading-snug">{{ shortModel(byKey.get(`${st}|${cx.id}`)!.decision.model) }}</div>
                <div v-if="byKey.get(`${st}|${cx.id}`)!.decision.suggestion" class="text-[10px] text-amber-300 leading-snug">
                  Wanted {{ shortModel(byKey.get(`${st}|${cx.id}`)!.decision.suggestion!.model) }} ({{ TIER_LABEL[byKey.get(`${st}|${cx.id}`)!.decision.tier!] || 'best tier' }}), not available
                </div>
                <div class="flex flex-wrap items-center gap-1 text-[10px] text-slate-400">
                  <span v-if="byKey.get(`${st}|${cx.id}`)!.decision.tier && !byKey.get(`${st}|${cx.id}`)!.decision.suggestion">{{ TIER_LABEL[byKey.get(`${st}|${cx.id}`)!.decision.tier!] }}</span>
                  <span
                    v-if="sourceTag(byKey.get(`${st}|${cx.id}`)!)"
                    class="px-1 rounded border"
                    :class="sourceTag(byKey.get(`${st}|${cx.id}`)!)!.cls"
                  >
                    {{ sourceTag(byKey.get(`${st}|${cx.id}`)!)!.label }}
                    <template v-if="benchByKey.get(`${st}|${cx.id}`)?.applied"> {{ benchByKey.get(`${st}|${cx.id}`)!.fpvr_percent }}%</template>
                  </span>
                </div>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Details of the selected cell -->
    <div v-if="selectedCell" class="rounded-xl border border-slate-700 bg-slate-950 p-3.5 space-y-2 text-xs">
      <div class="flex items-start justify-between gap-2">
        <div>
          <div class="font-semibold text-slate-100">{{ stageName(selectedCell.stage_id) }} · {{ selectedCell.complexity }}</div>
          <div class="text-slate-400 mt-0.5">
            <span :class="methodStyle(selectedCell.decision.method).text" class="font-semibold">{{ canonicalMethod(selectedCell.decision.method) }}</span>
            on <span class="font-mono text-slate-200 break-all">{{ selectedCell.decision.model }}</span>
            <template v-if="selectedCell.decision.tier"> · {{ TIER_LABEL[selectedCell.decision.tier] }}</template>
            · budget {{ selectedCell.decision.token_budget.toLocaleString() }} tokens
          </div>
        </div>
        <button type="button" class="p-1 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800" aria-label="Close details" @click="selected = null">
          <X class="w-4 h-4" />
        </button>
      </div>
      <p class="text-slate-300 leading-relaxed">{{ selectedCell.decision.reasoning }}</p>
      <p v-if="sourceTag(selectedCell)" class="text-slate-400">{{ sourceTag(selectedCell)!.help }}</p>
      <p v-if="selectedBench" class="text-slate-400">
        Benchmark: {{ canonicalMethod(selectedBench.optimal_method) }} on <span class="font-mono">{{ selectedBench.winning_model }}</span>,
        {{ selectedBench.fpvr_percent }}% first pass over {{ selectedBench.samples }} runs, ${{ (selectedBench.avg_cost_usd ?? 0).toFixed(4) }}/run
        <template v-if="!selectedBench.applied"> — not used yet: {{ selectedBench.not_applied_reason }}</template>
      </p>
      <p v-if="selectedCell.decision.fallback_chain.length > 1" class="text-slate-400">
        If it fails: <span class="font-mono text-slate-300 break-all">{{ selectedCell.decision.fallback_chain.slice(1).join(' → ') }}</span>
      </p>
    </div>

    <!-- Suggestions, one per model -->
    <div v-for="e in suggestions" :key="`${e.s.action}|${e.s.model}`"
      class="flex flex-col sm:flex-row sm:items-center justify-between gap-2 px-3 py-2 rounded-lg bg-amber-950/30 border border-amber-800/50">
      <div class="flex items-start gap-1.5 text-[11px] text-amber-200">
        <Lightbulb class="w-3.5 h-3.5 shrink-0 mt-0.5" />
        <span>{{ e.s.reason }} <span class="text-amber-400/80">({{ e.cells }} cell{{ e.cells === 1 ? '' : 's' }})</span></span>
      </div>
      <button type="button" @click="emit('apply-suggestion', e.s)"
        class="h-7 px-2.5 rounded bg-amber-900/60 hover:bg-amber-800/70 border border-amber-700/70 text-[11px] text-amber-100 shrink-0 transition-colors">
        {{ suggestionLabel(e.s.action) }}
      </button>
    </div>

    <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[10px] text-slate-500">
      <span v-for="m in METHODS" :key="m" class="flex items-center gap-1">
        <span class="w-2 h-2 rounded-sm" :style="{ backgroundColor: methodStyle(m).fill }"></span>{{ m }}
      </span>
      <span>· Tags show when a benchmark result or run history changed the policy choice.
        Dashed amber cells run a fallback because the recommended model is not allowed or not connected. Click a cell for its reasoning.</span>
    </div>
  </div>
</template>
