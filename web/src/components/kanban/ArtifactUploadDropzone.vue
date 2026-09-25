<script setup lang="ts">
import { ref } from 'vue'
import { GitBranch, UploadCloud } from 'lucide-vue-next'

const props = defineProps<{
  branchName: string
}>()

const emit = defineEmits<{
  (e: 'update:branchName', val: string): void
  (e: 'file-selected', file: File): void
}>()

const fileName = ref<string | null>(null)

function onFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files[0]) {
    fileName.value = target.files[0].name
    emit('file-selected', target.files[0])
  }
}
</script>

<template>
  <div class="space-y-3">
    <div>
      <label class="block text-[11px] text-slate-400 mb-1">Pre-Existing Git Feature Branch (Optional)</label>
      <div class="relative">
        <GitBranch class="absolute left-2.5 top-2.5 w-4 h-4 text-slate-500" />
        <input
          :value="branchName"
          @input="$emit('update:branchName', ($event.target as HTMLInputElement).value)"
          type="text"
          placeholder="e.g. feat/order-checkout-v2"
          class="w-full h-8 pl-8 pr-3 bg-slate-950 border border-slate-700 rounded text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:border-emerald-500 font-mono"
        />
      </div>
    </div>

    <div>
      <label class="block text-[11px] text-slate-400 mb-1">External TASK_PLAN.md or PRD.md (Optional)</label>
      <label
        class="border border-dashed border-slate-700 hover:border-emerald-500/80 rounded-lg p-3 flex items-center justify-center gap-2 cursor-pointer bg-slate-950/50 transition-colors"
      >
        <UploadCloud class="w-4 h-4 text-slate-400" />
        <span class="text-xs text-slate-300">
          {{ fileName || 'Click or drag external markdown plan here' }}
        </span>
        <input
          type="file"
          accept=".md,.markdown"
          @change="onFileChange"
          class="hidden"
        />
      </label>
    </div>
  </div>
</template>
