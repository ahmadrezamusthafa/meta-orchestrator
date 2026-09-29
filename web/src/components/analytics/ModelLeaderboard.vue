<script setup lang="ts">
import { ref, computed } from 'vue'
import type { LeaderboardEntryDTO } from '../../types'
import { Trophy, ArrowUp, ArrowDown, ArrowUpDown } from 'lucide-vue-next'
import { fmtInt, fmtPct, fmtDuration, fpvrHealth, HEALTH_TEXT } from './analyticsFormat'

const props = defineProps<{
  entries: LeaderboardEntryDTO[]
}>()

type SortKey = 'model' | 'tier' | 'tasks_completed' | 'fpvr_percent' | 'avg_tpf_tokens' | 'avg_ttr_seconds'

const columns: Array<{ key: SortKey; label: string; numeric: boolean }> = [
  { key: 'model', label: 'Model Name', numeric: false },
  { key: 'tier', label: 'Tier', numeric: false },
  { key: 'tasks_completed', label: 'Tasks Completed', numeric: true },
  { key: 'fpvr_percent', label: 'First-Pass Pass Rate (%)', numeric: true },
  { key: 'avg_tpf_tokens', label: 'Avg TPF', numeric: true },
  { key: 'avg_ttr_seconds', label: 'Avg TTR', numeric: true },
]

const sortKey = ref<SortKey>('fpvr_percent')
const sortDir = ref<'asc' | 'desc'>('desc')

function toggleSort(key: SortKey) {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    // Numeric columns default to descending, text columns to ascending.
    sortDir.value = columns.find((c) => c.key === key)?.numeric ? 'desc' : 'asc'
  }
}

const sorted = computed(() => {
  const dir = sortDir.value === 'asc' ? 1 : -1
  const key = sortKey.value
  return [...(props.entries ?? [])].sort((a, b) => {
    const av = a[key]
    const bv = b[key]
    if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir
    return String(av ?? '').localeCompare(String(bv ?? '')) * dir
  })
})

function ariaSort(key: SortKey): 'ascending' | 'descending' | 'none' {
  if (sortKey.value !== key) return 'none'
  return sortDir.value === 'asc' ? 'ascending' : 'descending'
}
</script>

<template>
  <section class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">Model Leaderboard</h3>
        <span class="text-[11px] text-slate-400">Ranked by first-pass verification, token-per-feature and time-to-resolution</span>
      </div>
      <Trophy class="w-4 h-4 text-amber-400" />
    </div>

    <div v-if="!sorted.length" class="py-8 text-center text-xs text-slate-500">
      No model runs recorded for this window.
    </div>

    <div v-else class="border border-slate-800 rounded-lg overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px] font-mono">
          <tr>
            <th v-for="col in columns" :key="col.key" class="p-0" :aria-sort="ariaSort(col.key)">
              <button
                type="button"
                class="w-full p-2.5 flex items-center gap-1.5 hover:text-slate-200 transition-colors"
                :class="[col.numeric ? 'justify-end text-right' : 'justify-start', sortKey === col.key ? 'text-emerald-400' : '']"
                @click="toggleSort(col.key)"
              >
                <span class="whitespace-nowrap">{{ col.label }}</span>
                <ArrowUp v-if="sortKey === col.key && sortDir === 'asc'" class="w-3 h-3" />
                <ArrowDown v-else-if="sortKey === col.key && sortDir === 'desc'" class="w-3 h-3" />
                <ArrowUpDown v-else class="w-3 h-3 opacity-40" />
              </button>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="(e, i) in sorted" :key="e.model + ':' + e.tier" class="hover:bg-slate-900/40">
            <td class="p-2.5 font-medium text-slate-200 whitespace-nowrap">
              <span class="text-slate-500 font-mono text-[10px] mr-1.5">#{{ i + 1 }}</span>{{ e.model }}
            </td>
            <td class="p-2.5">
              <span class="px-1.5 py-0.5 rounded bg-slate-950 border border-slate-800 text-[10px] font-mono text-slate-300 uppercase">
                {{ e.tier }}
              </span>
            </td>
            <td class="p-2.5 text-right text-sky-400 font-mono text-xs">{{ fmtInt(e.tasks_completed) }}</td>
            <td class="p-2.5 text-right font-mono text-xs" :class="HEALTH_TEXT[fpvrHealth(e.fpvr_percent)]">
              {{ fmtPct(e.fpvr_percent) }}
            </td>
            <td class="p-2.5 text-right text-sky-400 font-mono text-xs">{{ fmtInt(e.avg_tpf_tokens) }}</td>
            <td class="p-2.5 text-right text-sky-400 font-mono text-xs">{{ fmtDuration(e.avg_ttr_seconds) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
