<script setup lang="ts">
import { ShieldCheck } from 'lucide-vue-next'

const tiers = [
  {
    tier: 'Tier 1: Architectural Reasoning',
    description: 'Used for PRD analysis, AST tree queries, and RFC schema synthesis.',
    provider: 'Anthropic Claude',
    model: 'claude-3-5-sonnet-20241022',
    cost: '$3.00 / 1M tokens',
  },
  {
    tier: 'Tier 2: Code Generation & ATDD',
    description: 'Used for Playwright test suite authoring and application source implementation.',
    provider: 'Anthropic Claude',
    model: 'claude-3-5-sonnet-20241022',
    cost: '$3.00 / 1M tokens',
  },
  {
    tier: 'Tier 3: Log Parsing & Diagnostics',
    description: 'High-speed parsing for container stderr streams and test failure diffs.',
    provider: 'Google Antigravity / Gemini',
    model: 'gemini-2.0-flash',
    cost: '$0.10 / 1M tokens',
  },
]
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Three-Tier Model Stratification Matrix
        </h3>
        <span class="text-[11px] text-slate-400">Routes task sub-operations to cost-optimal reasoning tiers</span>
      </div>
      <ShieldCheck class="w-4 h-4 text-emerald-400" />
    </div>

    <div class="border border-slate-800 rounded-lg overflow-hidden">
      <table class="w-full text-left text-xs">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px] font-mono">
          <tr>
            <th class="p-2.5">Tier</th>
            <th class="p-2.5">Assigned Provider</th>
            <th class="p-2.5">Model Identifier</th>
            <th class="p-2.5">Estimated Cost</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="t in tiers" :key="t.tier" class="hover:bg-slate-900/50">
            <td class="p-2.5">
              <div class="font-medium text-slate-100">{{ t.tier }}</div>
              <div class="text-[10px] text-slate-500">{{ t.description }}</div>
            </td>
            <td class="p-2.5 font-medium text-emerald-400">{{ t.provider }}</td>
            <td class="p-2.5 font-mono text-[11px] text-slate-300">{{ t.model }}</td>
            <td class="p-2.5 font-mono text-[11px] text-sky-400">{{ t.cost }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
