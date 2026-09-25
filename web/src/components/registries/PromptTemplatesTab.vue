<script setup lang="ts">
import { ref } from 'vue'
import { MessageSquare } from 'lucide-vue-next'

defineProps<{
  prompts: Array<{
    id: string
    name: string
    source: string
    variables: string[]
    system_override: boolean
  }>
}>()

function formatVars(vars: string[]): string {
  return vars.map(v => '{{' + v + '}}').join(', ')
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Cascading Parameterized Prompt Templates
        </h3>
        <span class="text-[11px] text-slate-400">
          Resolved in hierarchy: Project Local (.sdlc/prompts/) &succ; User System &succ; Built-in
        </span>
      </div>
      <span class="text-xs font-mono text-sky-400 px-2 py-0.5 rounded bg-slate-900 border border-slate-800">
        {{ prompts.length }} Templates
      </span>
    </div>

    <div class="border border-slate-800 rounded-lg overflow-hidden font-mono text-xs">
      <table class="w-full text-left">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
          <tr>
            <th class="p-3">Template ID</th>
            <th class="p-3">Source Precedence</th>
            <th class="p-3">Slot Variables</th>
            <th class="p-3">Status</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="p in prompts" :key="p.id" class="hover:bg-slate-900/40">
            <td class="p-3 font-sans font-medium text-slate-100">{{ p.name }} ({{ p.id }})</td>
            <td class="p-3">
              <span class="px-2 py-0.5 rounded text-[10px] bg-slate-950 border border-slate-800 text-slate-400">
                {{ p.source }}
              </span>
            </td>
            <td class="p-3 text-sky-400">{{ formatVars(p.variables) }}</td>
            <td class="p-3">
              <span
                class="px-2 py-0.5 rounded text-[10px]"
                :class="p.system_override ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' : 'bg-slate-800 text-slate-400'"
              >
                {{ p.system_override ? 'Custom Override' : 'System Default' }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
