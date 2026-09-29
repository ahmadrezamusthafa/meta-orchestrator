<script setup lang="ts">
import { ref, computed } from 'vue'
import type { BenchmarkCellDTO } from '../../types'
import BenchmarkCellDetailModal from '../benchmark/BenchmarkCellDetailModal.vue'
import { TableProperties } from 'lucide-vue-next'
import { METHODS, methodStyle, fmtUsd, fpvrHealth, HEALTH_TEXT } from './analyticsFormat'

const props = defineProps<{
  cells: BenchmarkCellDTO[]
  source?: string
  generatedAt?: string
}>()

const selectedCell = ref<BenchmarkCellDTO | null>(null)

const stages = [
  { id: 'prd_discovery', name: '1. PRD Discovery' },
  { id: 'repo_discovery', name: '2. Dynamic Repo Discovery' },
  { id: 'atdd_creation', name: '3. ATDD Creation (Red Phase)' },
  { id: 'techdoc_rfc', name: '4. Tech Doc / RFC Review' },
  { id: 'task_breakdown', name: '5. Task Breakdown & Plan' },
  { id: 'red_verification', name: '6. Red Verification' },
  { id: 'task_implementation', name: '7. Task Implementation' },
  { id: 'e2e_validation', name: '8. Automation & E2E' },
  { id: 'signoff_merge', name: '9. Sign-Off & Evidence' },
]

const complexities: Array<BenchmarkCellDTO['complexity']> = ['LOW', 'MEDIUM', 'HIGH', 'SYSTEM']

const cellIndex = computed(() => {
  const m = new Map<string, BenchmarkCellDTO>()
  for (const c of props.cells ?? []) m.set(`${c.stage_id}|${c.complexity}`, c)
  return m
})

const filledCount = computed(() => cellIndex.value.size)

function getCell(stageId: string, comp: string): BenchmarkCellDTO | undefined {
  return cellIndex.value.get(`${stageId}|${comp}`)
}

function fmtTimestamp(ts?: string): string {
  if (!ts) return ''
  const d = new Date(ts)
  return isNaN(d.getTime()) ? ts : d.toLocaleString()
}
</script>

<template>
  <section class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2 pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Method Benchmark Matrix
          <span class="ml-1 text-sky-400 font-mono text-xs">{{ filledCount }}/36</span>
        </h3>
        <span class="text-[11px] text-slate-400">
          Winning method per SDLC stage &times; complexity
          <template v-if="source"> &middot; source <span class="font-mono text-slate-300">{{ source }}</span></template>
          <template v-if="generatedAt"> &middot; {{ fmtTimestamp(generatedAt) }}</template>
        </span>
      </div>
      <div class="flex items-center gap-3">
        <div class="flex items-center gap-2 text-[10px] font-mono">
          <span v-for="m in METHODS" :key="m" class="flex items-center gap-1">
            <span class="w-2 h-2 rounded-sm" :style="{ backgroundColor: methodStyle(m).fill }"></span>
            <span class="text-slate-400">{{ m }}</span>
          </span>
        </div>
        <TableProperties class="w-4 h-4 text-emerald-400" />
      </div>
    </div>

    <div class="border border-slate-800 rounded-lg overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px] font-mono">
          <tr>
            <th class="p-2.5 min-w-[180px]">SDLC Stage</th>
            <th v-for="c in complexities" :key="c" class="p-2.5 min-w-[140px] text-center">{{ c }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="s in stages" :key="s.id">
            <td class="p-2.5 font-medium text-slate-200 whitespace-nowrap">{{ s.name }}</td>
            <td v-for="c in complexities" :key="c" class="p-1.5">
              <button
                v-if="getCell(s.id, c)"
                type="button"
                class="w-full p-1.5 rounded border space-y-1 text-left transition-colors hover:border-slate-500"
                :class="[methodStyle(getCell(s.id, c)?.optimal_method).bg, methodStyle(getCell(s.id, c)?.optimal_method).border]"
                :title="`${getCell(s.id, c)?.optimal_method} · ${getCell(s.id, c)?.winning_model}`"
                @click="selectedCell = getCell(s.id, c) || null"
              >
                <div class="flex items-center justify-between text-[10px] font-mono">
                  <span class="font-semibold" :class="methodStyle(getCell(s.id, c)?.optimal_method).text">
                    {{ getCell(s.id, c)?.optimal_method }}
                  </span>
                  <span class="font-bold" :class="HEALTH_TEXT[fpvrHealth(getCell(s.id, c)?.fpvr_percent ?? 0)]">
                    {{ getCell(s.id, c)?.fpvr_percent }}%
                  </span>
                </div>
                <div class="flex items-center justify-between text-[9px] font-mono text-slate-500">
                  <span class="uppercase">{{ getCell(s.id, c)?.model_tier || '—' }}</span>
                  <span class="text-sky-400">
                    {{ getCell(s.id, c)?.avg_cost_usd !== undefined ? fmtUsd(getCell(s.id, c)?.avg_cost_usd) : '—' }}
                  </span>
                </div>
              </button>
              <div
                v-else
                class="w-full p-1.5 rounded border border-dashed border-slate-800 bg-slate-950/40 text-center text-[10px] font-mono text-slate-600"
              >
                no data
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <BenchmarkCellDetailModal v-if="selectedCell" :cell="selectedCell" @close="selectedCell = null" />
  </section>
</template>
