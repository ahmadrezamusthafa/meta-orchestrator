<script setup lang="ts">
import { ref } from 'vue'
import { AlertTriangle, RotateCcw, X } from 'lucide-vue-next'
import BtnDestructive from '../common/BtnDestructive.vue'

defineProps<{
  taskId: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm'): void
}>()

const isResetting = ref(false)

function onConfirm() {
  isResetting.value = true
  emit('confirm')
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3 text-rose-400">
          <div class="p-2 rounded bg-rose-950/80 border border-rose-800">
            <AlertTriangle class="w-5 h-5 text-rose-400" />
          </div>
          <div>
            <h3 class="text-sm font-bold text-slate-100 uppercase tracking-wide">
              Reset Workspace Volumes?
            </h3>
            <span class="text-xs text-slate-500 font-mono">Task {{ taskId }}</span>
          </div>
        </div>

        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <p class="text-xs text-slate-300 leading-relaxed">
        This action will purge all Docker ephemeral volume allocations for this task, run <code class="text-rose-400 font-mono">git clean -fd && git reset --hard</code> to revert corrupted file mutations, and restart the current stage from a clean checkpoint.
      </p>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200 transition-colors"
        >
          Cancel
        </button>

        <BtnDestructive
          :loading="isResetting"
          @click="onConfirm"
        >
          <RotateCcw class="w-3.5 h-3.5" />
          <span>Purge Volumes & Reset</span>
        </BtnDestructive>
      </div>
    </div>
  </div>
</template>
