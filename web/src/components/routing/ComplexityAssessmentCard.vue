<script setup lang="ts">
import type { ComplexityAssessment, Complexity } from '../../types'
import { BrainCircuit, AlertTriangle, Loader2 } from 'lucide-vue-next'

defineProps<{
  assessment: ComplexityAssessment
  busy?: boolean
}>()
const emit = defineEmits<{ (e: 'regrade', complexity: Complexity): void }>()

const LEVELS: { id: Complexity; hint: string }[] = [
  { id: 'LOW', hint: 'Small contained change' },
  { id: 'MEDIUM', hint: 'Feature in one service' },
  { id: 'HIGH', hint: 'Cross-service or risky' },
  { id: 'SYSTEM', hint: 'Infra, CI, migrations' },
]
const LEVEL_STYLE: Record<Complexity, string> = {
  LOW: 'border-emerald-600 bg-emerald-950/50 text-emerald-300',
  MEDIUM: 'border-sky-600 bg-sky-950/50 text-sky-300',
  HIGH: 'border-amber-600 bg-amber-950/50 text-amber-300',
  SYSTEM: 'border-violet-600 bg-violet-950/50 text-violet-300',
}
const SOURCE_LABEL: Record<string, string> = { ai: 'Analysed by AI', heuristic: 'Keyword estimate', operator: 'Set by you' }
</script>

<template>
  <div class="p-3.5 rounded-lg bg-slate-950 border border-slate-800 space-y-2.5">
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-2 text-xs font-medium text-slate-200">
        <BrainCircuit class="w-4 h-4 text-emerald-400" />
        Task complexity
        <Loader2 v-if="busy" class="w-3.5 h-3.5 animate-spin text-slate-400" />
      </div>
      <span class="text-[10px] font-mono text-slate-500">
        {{ SOURCE_LABEL[assessment.source] || assessment.source }}<template v-if="assessment.model"> · {{ assessment.model }}</template>
      </span>
    </div>

    <div class="grid grid-cols-2 sm:grid-cols-4 gap-1.5" role="radiogroup" aria-label="Complexity">
      <button
        v-for="l in LEVELS"
        :key="l.id"
        type="button"
        role="radio"
        :aria-checked="assessment.complexity === l.id"
        :disabled="busy"
        class="px-2 py-1.5 rounded border text-left transition-colors disabled:opacity-60"
        :class="assessment.complexity === l.id ? LEVEL_STYLE[l.id] : 'border-slate-800 bg-slate-900 text-slate-400 hover:border-slate-600'"
        @click="assessment.complexity !== l.id && emit('regrade', l.id)"
      >
        <div class="text-[11px] font-bold font-mono">{{ l.id }}</div>
        <div class="text-[10px] opacity-80">{{ l.hint }}</div>
      </button>
    </div>

    <p class="text-[11px] text-slate-300 leading-relaxed">{{ assessment.rationale }}</p>
    <div v-if="assessment.signals?.length" class="flex flex-wrap gap-1">
      <span v-for="s in assessment.signals" :key="s" class="px-1.5 py-0.5 rounded bg-slate-800 text-[10px] font-mono text-slate-400">{{ s }}</span>
    </div>
    <p v-if="assessment.fallback" class="flex items-start gap-1.5 text-[11px] text-amber-400">
      <AlertTriangle class="w-3.5 h-3.5 mt-0.5 shrink-0" />
      <span>{{ assessment.fallback }}. Check the grade and change it if it looks wrong.</span>
    </p>
    <p class="text-[10px] text-slate-500">Choosing a different level rebuilds the proposed plan below.</p>
  </div>
</template>
