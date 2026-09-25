<script setup lang="ts">
import { ref } from 'vue'
import type { ToolDTO } from '../../types'
import { RotateCcw, X } from 'lucide-vue-next'
import BtnDestructive from '../common/BtnDestructive.vue'

const props = defineProps<{
  tool: ToolDTO
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', version: string): void
}>()

const selectedVersion = ref(props.tool.past_versions?.[0] || 'v1.0.0')
const isRollingBack = ref(false)

function onConfirm() {
  isRollingBack.value = true
  emit('confirm', selectedVersion.value)
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div class="p-2 rounded bg-amber-950 border border-amber-800 text-amber-400">
            <RotateCcw class="w-5 h-5" />
          </div>
          <div>
            <h3 class="text-sm font-bold text-slate-100">Rollback Tool Version</h3>
            <span class="text-xs text-slate-400">{{ tool.name }}</span>
          </div>
        </div>
        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-3">
        <div>
          <label class="block text-xs text-slate-300 mb-1">Select Previous Snapshot</label>
          <select
            v-model="selectedVersion"
            class="w-full h-9 px-3 bg-slate-950 border border-slate-700 rounded text-xs text-slate-200 focus:outline-none focus:border-emerald-500 font-mono"
          >
            <option v-for="v in tool.past_versions" :key="v" :value="v">
              {{ v }} (Previous stable snapshot)
            </option>
          </select>
        </div>

        <p class="text-xs text-slate-400 leading-relaxed">
          The daemon will immediately swap active filesystem symlinks to <code class="text-amber-400 font-mono">{{ selectedVersion }}</code> in &lt; 1s.
        </p>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200"
        >
          Cancel
        </button>

        <BtnDestructive
          :loading="isRollingBack"
          @click="onConfirm"
        >
          <RotateCcw class="w-3.5 h-3.5" />
          <span>Confirm Rollback</span>
        </BtnDestructive>
      </div>
    </div>
  </div>
</template>
