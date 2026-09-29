<script setup lang="ts">
import { ref, computed } from 'vue'
import type { MethodStatDTO } from '../../types'
import { BarChart3 } from 'lucide-vue-next'
import { METHODS, methodStyle, fmtInt, fmtUsd, fmtPct, fmtDuration, fpvrHealth, HEALTH_TEXT } from './analyticsFormat'

const props = defineProps<{
  methods: MethodStatDTO[]
}>()

const hovered = ref<string | null>(null)

// Always render the four canonical methods (zeroed when absent), plus any extra reported methods.
const rows = computed<MethodStatDTO[]>(() => {
  const byName = new Map((props.methods ?? []).map((m) => [m.method, m]))
  const canonical = METHODS.map((name) => byName.get(name) ?? {
    method: name, runs: 0, total_cost_usd: 0, avg_cost_usd: 0, avg_tpf_tokens: 0, fpvr_percent: 0, avg_ttr_seconds: 0,
  })
  const extras = (props.methods ?? []).filter((m) => !(METHODS as readonly string[]).includes(m.method))
  return [...canonical, ...extras]
})

const maxCost = computed(() => Math.max(0, ...rows.value.map((r) => r.avg_cost_usd || 0)) || 1)
const hasData = computed(() => rows.value.some((r) => r.runs > 0))

// Lowest avg cost among methods with runs AND FPVR > 80% → best ROI.
const bestRoi = computed(() => {
  const eligible = rows.value.filter((r) => r.runs > 0 && r.fpvr_percent > 80)
  if (!eligible.length) return null
  return eligible.reduce((a, b) => (b.avg_cost_usd < a.avg_cost_usd ? b : a)).method
})

function barPct(r: MethodStatDTO): number {
  return Math.max(r.runs > 0 ? 2 : 0, ((r.avg_cost_usd || 0) / maxCost.value) * 100)
}
</script>

<template>
  <section class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">Methodology ROI</h3>
        <span class="text-[11px] text-slate-400">Avg cost per run by orchestration method, with first-pass verification rate</span>
      </div>
      <BarChart3 class="w-4 h-4 text-sky-400" />
    </div>

    <div v-if="!hasData" class="py-8 text-center text-xs text-slate-500">
      No methodology runs recorded for this window.
    </div>

    <div v-else class="space-y-3">
      <div
        v-for="r in rows"
        :key="r.method"
        class="relative"
        @mouseenter="hovered = r.method"
        @mouseleave="hovered = null"
      >
        <div class="flex items-center justify-between text-[11px] mb-1">
          <span class="font-semibold" :class="methodStyle(r.method).text">
            {{ r.method }}
            <span
              v-if="bestRoi === r.method"
              class="ml-1.5 px-1 py-px rounded bg-emerald-950 border border-emerald-800 text-[9px] font-mono text-emerald-300 uppercase"
            >best roi</span>
          </span>
          <span class="flex items-center gap-3">
            <span class="text-sky-400 font-mono text-xs">{{ fmtUsd(r.avg_cost_usd) }}</span>
            <span class="font-mono text-xs w-14 text-right" :class="r.runs ? HEALTH_TEXT[fpvrHealth(r.fpvr_percent)] : 'text-slate-600'">
              {{ r.runs ? fmtPct(r.fpvr_percent) : '—' }}
            </span>
          </span>
        </div>
        <svg class="w-full h-4 block" viewBox="0 0 100 4" preserveAspectRatio="none" role="img" :aria-label="`${r.method} average cost`">
          <rect x="0" y="0" width="100" height="4" rx="0.6" fill="#0f172a" />
          <rect
            x="0"
            y="0"
            :width="barPct(r)"
            height="4"
            rx="0.6"
            :fill="methodStyle(r.method).fill"
            :fill-opacity="hovered === null || hovered === r.method ? 0.9 : 0.35"
            class="transition-all duration-500"
          />
        </svg>

        <div
          v-if="hovered === r.method"
          class="absolute right-0 top-full mt-1 z-10 pointer-events-none px-2.5 py-2 rounded-lg bg-slate-950 border border-slate-700 shadow-xl text-[11px] font-mono space-y-0.5 whitespace-nowrap"
        >
          <div class="text-slate-200 font-semibold">{{ r.method }}</div>
          <div class="text-slate-400">Runs: <span class="text-sky-400">{{ fmtInt(r.runs) }}</span></div>
          <div class="text-slate-400">Avg tokens / feature: <span class="text-sky-400">{{ fmtInt(r.avg_tpf_tokens) }}</span></div>
          <div class="text-slate-400">Avg cost / run: <span class="text-amber-400">{{ fmtUsd(r.avg_cost_usd) }}</span></div>
          <div class="text-slate-400">Total cost: <span class="text-amber-400">{{ fmtUsd(r.total_cost_usd) }}</span></div>
          <div class="text-slate-400">FPVR: <span :class="HEALTH_TEXT[fpvrHealth(r.fpvr_percent)]">{{ fmtPct(r.fpvr_percent) }}</span></div>
          <div class="text-slate-400">Avg TTR: <span class="text-slate-200">{{ fmtDuration(r.avg_ttr_seconds) }}</span></div>
        </div>
      </div>
    </div>
  </section>
</template>
