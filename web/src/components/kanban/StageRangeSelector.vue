<script setup lang="ts">
import { computed } from 'vue'
import { Video } from 'lucide-vue-next'

const props = defineProps<{
  startStage: string
  haltStage: string
  produceVideo: boolean
  stages?: Array<{ id: string; name: string }>
}>()

const emit = defineEmits<{
  (e: 'update:startStage', val: string): void
  (e: 'update:haltStage', val: string): void
  (e: 'update:produceVideo', val: boolean): void
}>()

const fallbackStages = [
  { id: 'prd_discovery', name: 'Stage 1: PRD & Discovery' },
  { id: 'atdd_creation', name: 'Stage 2: ATDD Creation (Red Phase)' },
  { id: 'techdoc_rfc', name: 'Stage 3: Tech Doc / RFC Review' },
  { id: 'task_breakdown', name: 'Stage 4: Task Breakdown & Planning' },
  { id: 'task_implementation', name: 'Stage 5: Task Implementation' },
  { id: 'e2e_validation', name: 'Stage 6: Automation & E2E Validation' },
  { id: 'uat_verification', name: 'Stage 7: Manual & UAT Verification' },
  { id: 'signoff_merge', name: 'Stage 8: Sign-Off & Merge' },
]

const availableStages = computed(() => {
  if (props.stages && props.stages.length > 0) {
    return props.stages
  }
  return fallbackStages
})

const startModel = computed({
  get: () => props.startStage,
  set: (val) => emit('update:startStage', val),
})

const haltModel = computed({
  get: () => props.haltStage,
  set: (val) => emit('update:haltStage', val),
})

const videoModel = computed({
  get: () => props.produceVideo,
  set: (val) => emit('update:produceVideo', val),
})
</script>

<template>
  <div class="p-3.5 rounded-lg bg-slate-950 border border-slate-800 space-y-3">
    <div class="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
      Mid-Process Execution Slicing Window
    </div>

    <div class="grid grid-cols-2 gap-3 items-center">
      <div>
        <label class="block text-[11px] text-slate-400 mb-1">Start Stage (Bypasses earlier stages)</label>
        <select
          v-model="startModel"
          class="w-full h-8 px-2.5 bg-slate-900 border border-slate-700 rounded text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
        >
          <option v-for="s in availableStages" :key="s.id" :value="s.id">{{ s.name }}</option>
        </select>
      </div>

      <div>
        <label class="block text-[11px] text-slate-400 mb-1">Halt Stage (Terminates after finish)</label>
        <select
          v-model="haltModel"
          class="w-full h-8 px-2.5 bg-slate-900 border border-slate-700 rounded text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
        >
          <option v-for="s in availableStages" :key="s.id" :value="s.id">{{ s.name }}</option>
        </select>
      </div>
    </div>

    <div class="pt-2 border-t border-slate-900 flex items-center justify-between">
      <label class="flex items-center gap-2 cursor-pointer text-xs text-slate-300">
        <input
          type="checkbox"
          v-model="videoModel"
          class="rounded bg-slate-900 border-slate-700 text-emerald-600 focus:ring-emerald-500"
        />
        <span>Record Playwright .mp4 test execution video</span>
      </label>
      <Video class="w-4 h-4 text-emerald-400" />
    </div>
  </div>
</template>
