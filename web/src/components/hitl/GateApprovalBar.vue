<script setup lang="ts">
import { ShieldAlert, CheckCircle2, XCircle } from 'lucide-vue-next'
import BtnPrimary from '../common/BtnPrimary.vue'
import BtnDestructive from '../common/BtnDestructive.vue'

defineProps<{
  stageId: string
  stageName: string
}>()

defineEmits<{
  (e: 'approve'): void
  (e: 'reject'): void
}>()
</script>

<template>
  <div class="p-3 bg-amber-950/60 border-b border-amber-800/80 flex items-center justify-between gap-4">
    <div class="flex items-center gap-2.5">
      <ShieldAlert class="w-4 h-4 text-amber-400 flex-shrink-0" />
      <div>
        <div class="text-xs font-semibold text-amber-200">
          Human Approval Gate Required: {{ stageName }}
        </div>
        <div class="text-[11px] text-amber-300/80">
          Verify technical documentation, test suites, or evidence artifacts before advancing.
        </div>
      </div>
    </div>

    <div class="flex items-center gap-2">
      <BtnDestructive @click="$emit('reject')">
        <XCircle class="w-3.5 h-3.5" />
        <span>Reject / Revise</span>
      </BtnDestructive>

      <BtnPrimary @click="$emit('approve')">
        <CheckCircle2 class="w-3.5 h-3.5" />
        <span>Authorize & Advance</span>
      </BtnPrimary>
    </div>
  </div>
</template>
