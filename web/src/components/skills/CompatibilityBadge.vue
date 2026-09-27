<script setup lang="ts">
import { computed } from 'vue'
import type { SkillCompatibility } from '../../types'
import { CheckCircle2, AlertTriangle, XCircle, ShieldCheck } from 'lucide-vue-next'

const props = defineProps<{
  compatibility?: SkillCompatibility
  interactive?: boolean
}>()

defineEmits<{
  (e: 'click'): void
}>()

const status = computed(() => props.compatibility?.status || 'compatible')
const score = computed(() => props.compatibility?.score ?? 100)

const badgeStyles = computed(() => {
  switch (status.value) {
    case 'compatible':
      return {
        bg: 'bg-emerald-950/60 border-emerald-800/80 text-emerald-300 hover:bg-emerald-900/60',
        dot: 'bg-emerald-400',
        label: `${score.value}% Compatible`,
      }
    case 'warning':
      return {
        bg: 'bg-amber-950/60 border-amber-800/80 text-amber-300 hover:bg-amber-900/60',
        dot: 'bg-amber-400',
        label: `${score.value}% Warning`,
      }
    case 'incompatible':
      return {
        bg: 'bg-rose-950/60 border-rose-800/80 text-rose-300 hover:bg-rose-900/60',
        dot: 'bg-rose-400',
        label: `${score.value}% Incompatible`,
      }
    default:
      return {
        bg: 'bg-slate-800/60 border-slate-700 text-slate-300',
        dot: 'bg-slate-400',
        label: 'Unknown',
      }
  }
})
</script>

<template>
  <button
    type="button"
    @click="$emit('click')"
    :disabled="!interactive"
    class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium border transition-colors shadow-sm"
    :class="[
      badgeStyles.bg,
      interactive ? 'cursor-pointer hover:border-slate-500' : 'cursor-default'
    ]"
    :title="interactive ? 'Click to inspect detailed compatibility report' : ''"
  >
    <CheckCircle2 v-if="status === 'compatible'" class="w-3.5 h-3.5 text-emerald-400" />
    <AlertTriangle v-else-if="status === 'warning'" class="w-3.5 h-3.5 text-amber-400" />
    <XCircle v-else-if="status === 'incompatible'" class="w-3.5 h-3.5 text-rose-400" />
    <ShieldCheck v-else class="w-3.5 h-3.5 text-slate-400" />

    <span>{{ badgeStyles.label }}</span>
  </button>
</template>
