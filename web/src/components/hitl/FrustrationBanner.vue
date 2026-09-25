<script setup lang="ts">
import { AlertOctagon, RotateCcw, MessageSquare } from 'lucide-vue-next'
import BtnPrimary from '../common/BtnPrimary.vue'
import BtnDestructive from '../common/BtnDestructive.vue'

defineProps<{
  trace?: string
}>()

defineEmits<{
  (e: 'inject-guidance'): void
  (e: 'reset-workspace'): void
}>()
</script>

<template>
  <div class="bg-rose-950 border-l-4 border-rose-600 p-4 shadow-2xl flex items-start gap-3.5 z-20">
    <AlertOctagon class="w-5 h-5 text-rose-400 flex-shrink-0 mt-0.5" />

    <div class="flex-1 space-y-1.5">
      <div class="flex items-center justify-between">
        <h3 class="text-xs font-bold text-rose-200 uppercase tracking-wide">
          Execution Halted: Frustration Threshold Exceeded (Attempt 3/3 Failed)
        </h3>
        <span class="text-[10px] font-mono text-rose-400 font-semibold bg-rose-900/60 px-2 py-0.5 rounded border border-rose-800">
          STATE: BLOCKED_FRUSTRATION
        </span>
      </div>

      <p class="text-xs text-rose-300/90 leading-relaxed font-mono bg-rose-900/40 p-2 rounded border border-rose-800/60">
        {{ trace || 'Assertion failure: Expected status 200 within 5000ms. Received 504 Gateway Timeout.' }}
      </p>

      <div class="pt-2 flex items-center gap-3">
        <BtnPrimary @click="$emit('inject-guidance')">
          <MessageSquare class="w-3.5 h-3.5" />
          <span>Inject Guidance & Resume</span>
        </BtnPrimary>

        <BtnDestructive @click="$emit('reset-workspace')">
          <RotateCcw class="w-3.5 h-3.5" />
          <span>Purge Volumes & Reset Workspace</span>
        </BtnDestructive>
      </div>
    </div>
  </div>
</template>
