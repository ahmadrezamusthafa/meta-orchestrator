<script setup lang="ts">
import { computed } from 'vue'
import type { Task, WorkflowStage } from '../../types'
import { getStageGuidance } from '../../composables/useStageGuidance'
import {
  Compass,
  ArrowRight,
  Lightbulb,
  ShieldAlert,
  AlertTriangle,
  CheckCircle2,
  Lock,
  Unlock,
} from 'lucide-vue-next'

const props = defineProps<{
  task: Task | null
  customStages?: WorkflowStage[]
}>()

const emit = defineEmits<{
  (e: 'approve-gate'): void
  (e: 'reject-gate'): void
  (e: 'steer-click'): void
}>()

const guidance = computed(() => getStageGuidance(props.task, props.customStages))
const progressPercent = computed(() => {
  if (!guidance.value.totalStages) return 0
  return Math.round((guidance.value.stageNumber / guidance.value.totalStages) * 100)
})
</script>

<template>
  <div
    class="p-4 bg-slate-900 border rounded-xl space-y-3 transition-colors shadow-md"
    :class="{
      'border-rose-800/80 bg-rose-950/20': guidance.actionType === 'blocked_steer',
      'border-amber-700/80 bg-amber-950/20': guidance.actionType === 'gate_approval',
      'border-emerald-800/80 bg-emerald-950/20': guidance.actionType === 'completed_review',
      'border-slate-800': guidance.actionType === 'autonomous_observe',
    }"
  >
    <!-- Header: Step Info & Stage Badge -->
    <div class="flex items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <Compass class="w-4 h-4 text-emerald-400" />
        <span class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Step {{ guidance.stageNumber }} of {{ guidance.totalStages }}: {{ guidance.stageName }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <span
          v-if="guidance.isWriteLocked"
          class="flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-mono bg-amber-950/80 text-amber-300 border border-amber-800"
        >
          <Lock class="w-3 h-3" />
          <span>Write-Locked</span>
        </span>
        <span
          v-else
          class="flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-mono bg-emerald-950/80 text-emerald-300 border border-emerald-800"
        >
          <Unlock class="w-3 h-3" />
          <span>Write-Unlocked</span>
        </span>

        <span class="text-[11px] font-mono text-slate-400">
          {{ progressPercent }}% Done
        </span>
      </div>
    </div>

    <!-- Progress Bar -->
    <div class="w-full h-1.5 bg-slate-950 rounded-full overflow-hidden border border-slate-800">
      <div
        class="h-full transition-all duration-300 rounded-full"
        :class="{
          'bg-rose-500': guidance.actionType === 'blocked_steer',
          'bg-amber-500': guidance.actionType === 'gate_approval',
          'bg-emerald-500': guidance.actionType === 'autonomous_observe' || guidance.actionType === 'completed_review',
        }"
        :style="{ width: `${progressPercent}%` }"
      ></div>
    </div>

    <!-- Status Information (What is currently happening) -->
    <div class="text-xs text-slate-300 leading-relaxed font-sans flex items-start gap-2">
      <span class="text-slate-500 font-semibold uppercase text-[10px] tracking-wider mt-0.5 whitespace-nowrap">
        Current Status:
      </span>
      <span>{{ guidance.statusDescription }}</span>
    </div>

    <!-- Next Step & Operator Suggestion -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-2 pt-2 border-t border-slate-800/80 text-xs">
      <!-- Next Step -->
      <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-start gap-2">
        <ArrowRight class="w-3.5 h-3.5 text-sky-400 mt-0.5 flex-shrink-0" />
        <div>
          <span class="block text-[10px] font-mono text-sky-400 font-semibold uppercase">Next Step:</span>
          <span class="text-[11px] text-slate-300 leading-snug">{{ guidance.nextStepDescription }}</span>
        </div>
      </div>

      <!-- Actionable Suggestion -->
      <div
        class="p-2.5 rounded-lg border flex items-start gap-2"
        :class="{
          'bg-rose-950/40 border-rose-800 text-rose-200': guidance.actionType === 'blocked_steer',
          'bg-amber-950/40 border-amber-800 text-amber-200': guidance.actionType === 'gate_approval',
          'bg-emerald-950/40 border-emerald-800 text-emerald-200': guidance.actionType === 'completed_review',
          'bg-slate-950/60 border-slate-800/80 text-slate-300': guidance.actionType === 'autonomous_observe',
        }"
      >
        <Lightbulb class="w-3.5 h-3.5 text-amber-400 mt-0.5 flex-shrink-0" />
        <div>
          <span class="block text-[10px] font-mono font-semibold uppercase" :class="guidance.actionType === 'blocked_steer' ? 'text-rose-400' : 'text-amber-400'">
            Operator Suggestion:
          </span>
          <span class="text-[11px] leading-snug">{{ guidance.suggestion }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
