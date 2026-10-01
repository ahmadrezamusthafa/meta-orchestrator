<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ShieldCheck, Lightbulb } from 'lucide-vue-next'
import { api } from '../../services/api'
import type { AvailableModelDTO, TierAssignmentDTO, ModelTier } from '../../types'

// Tier assignments are derived from the priority chain (see RouterConfigurator), not configured here.
const TIER_INFO: Record<ModelTier, { title: string; description: string }> = {
  tier1: { title: 'Tier 1: Architectural Reasoning', description: 'PRD analysis, RFC synthesis, task breakdown, ATDD and complex implementation.' },
  tier2: { title: 'Tier 2: Code Generation & ATDD', description: 'Standard implementation and E2E / UAT verification.' },
  tier3: { title: 'Tier 3: Log Parsing & Diagnostics', description: 'Container stderr streams and test failure diffs.' },
}

const tiers = ref<TierAssignmentDTO[]>([])
const models = ref<AvailableModelDTO[]>([])
const loadError = ref('')

onMounted(async () => {
  try {
    const res = await api.getRouterSettings()
    tiers.value = res.tiers || []
    models.value = res.all_models || []
  } catch (err: any) {
    loadError.value = err.message || 'Unable to load tier assignments'
  }
})

function costPer1M(full: string): string {
  const id = full.includes('/') ? full.slice(full.indexOf('/') + 1) : full
  const m = models.value.find(c => c.model_id === id)
  return m ? `$${(m.cost_per_1k * 1000).toFixed(2)} / 1M tokens` : '—'
}
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Model tiers
        </h3>
        <span class="text-[11px] text-slate-400">The model each tier runs on under your saved allowed-models list. Tiered Best Practice picks a tier per stage and complexity.</span>
      </div>
      <ShieldCheck class="w-4 h-4 text-emerald-400" />
    </div>

    <div v-if="loadError" class="text-xs text-rose-400">{{ loadError }}</div>

    <div v-else class="border border-slate-800 rounded-lg overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px] font-mono">
          <tr>
            <th class="p-2.5">Tier</th>
            <th class="p-2.5">Runs On</th>
            <th class="p-2.5">Estimated Cost</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="t in tiers" :key="t.tier" class="hover:bg-slate-900/50 align-top">
            <td class="p-2.5">
              <div class="font-medium text-slate-100">{{ TIER_INFO[t.tier]?.title || t.tier }}</div>
              <div class="text-[10px] text-slate-500">{{ TIER_INFO[t.tier]?.description }}</div>
            </td>
            <td class="p-2.5">
              <div class="font-mono text-[11px] text-emerald-300">{{ t.model }}</div>
              <div v-if="t.suggestion" class="mt-1 flex items-start gap-1 text-[10px] text-amber-300">
                <Lightbulb class="w-3 h-3 shrink-0 mt-px" />
                <span>{{ t.suggestion.reason }}</span>
              </div>
            </td>
            <td class="p-2.5 font-mono text-[11px] text-sky-400">{{ costPer1M(t.model) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
