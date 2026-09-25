<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../../services/api'
import type { BenchmarkCellDTO } from '../../types'
import BenchmarkCellDetailModal from './BenchmarkCellDetailModal.vue'
import { TableProperties } from 'lucide-vue-next'

const matrix = ref<BenchmarkCellDTO[]>([])
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

const complexities: Array<'LOW' | 'MEDIUM' | 'HIGH' | 'SYSTEM'> = ['LOW', 'MEDIUM', 'HIGH', 'SYSTEM']

onMounted(async () => {
  try {
    const res = await api.getBenchmarks()
    matrix.value = res.matrix || []
  } catch (e) {
    console.error('Failed to load benchmarks:', e)
  }
})

function getCell(stageId: string, comp: string) {
  return matrix.value.find((c) => c.stage_id === stageId && c.complexity === comp)
}
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Empirical Method Benchmark Matrix (36 Permutations)
        </h3>
        <span class="text-[11px] text-slate-400">
          SDLC Stages &times; Task Complexity strata calibrated for optimal FPVR & lowest token burn
        </span>
      </div>
      <TableProperties class="w-4 h-4 text-emerald-400" />
    </div>

    <div class="border border-slate-800 rounded-lg overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px] font-mono">
          <tr>
            <th class="p-2.5 min-w-[180px]">SDLC Stage</th>
            <th v-for="c in complexities" :key="c" class="p-2.5 min-w-[130px] text-center">
              {{ c }}
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="s in stages" :key="s.id" class="hover:bg-slate-900/40">
            <td class="p-2.5 font-medium text-slate-200">{{ s.name }}</td>
            <td
              v-for="c in complexities"
              :key="c"
              class="p-2 text-center cursor-pointer hover:bg-slate-850 transition-colors"
              @click="selectedCell = getCell(s.id, c) || null"
            >
              <div v-if="getCell(s.id, c)" class="p-1.5 rounded bg-slate-950 border border-slate-800 space-y-1">
                <div class="flex items-center justify-between text-[10px] font-mono">
                  <span
                    class="px-1 rounded font-semibold"
                    :class="{
                      'text-emerald-400 bg-emerald-950': getCell(s.id, c)?.optimal_method === 'BMAD',
                      'text-sky-400 bg-sky-950': getCell(s.id, c)?.optimal_method === 'Supervisor',
                      'text-purple-400 bg-purple-950': getCell(s.id, c)?.optimal_method === 'ReAct',
                    }"
                  >
                    {{ getCell(s.id, c)?.optimal_method }}
                  </span>
                  <span class="text-emerald-400 font-bold">{{ getCell(s.id, c)?.fpvr_percent }}%</span>
                </div>
                <div class="text-[9px] font-mono text-slate-500 text-left">
                  {{ Math.round((getCell(s.id, c)?.avg_tokens || 0) / 1000) }}k tok | {{ getCell(s.id, c)?.avg_duration_s }}s
                </div>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <BenchmarkCellDetailModal
      v-if="selectedCell"
      :cell="selectedCell"
      @close="selectedCell = null"
    />
  </div>
</template>
