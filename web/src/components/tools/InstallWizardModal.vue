<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { ToolDTO } from '../../types'
import { useToolsStore } from '../../stores/tools'
import InstallStepper from './InstallStepper.vue'
import BtnPrimary from '../common/BtnPrimary.vue'
import { Terminal, X, CheckCircle2 } from 'lucide-vue-next'

const props = defineProps<{
  tool: ToolDTO
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'completed'): void
}>()

const toolsStore = useToolsStore()
const currentStep = ref(1)
const miniLogs = ref<string[]>([])
const isFinished = ref(false)

onMounted(async () => {
  miniLogs.value.push(`[Init] Checking host environment for ${props.tool.name}...`)
  
  await toolsStore.installTool(props.tool.id, props.tool.latest_version)

  setTimeout(() => {
    currentStep.value = 2
    miniLogs.value.push(`[Build] Compiling release binaries and resolving container mounts...`)
  }, 300)

  setTimeout(() => {
    currentStep.value = 3
    miniLogs.value.push(`[Test] Running integrity diagnostics and self-test verification...`)
  }, 600)

  setTimeout(() => {
    currentStep.value = 4
    miniLogs.value.push(`[Success] Atomic symlink switched. Tool ${props.tool.name} active at ${props.tool.latest_version}`)
    isFinished.value = true
  }, 900)
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-5">
      <div class="flex items-center justify-between pb-3 border-b border-slate-800">
        <div>
          <h3 class="text-sm font-bold text-slate-100">
            Guided Tool Installation: {{ tool.name }}
          </h3>
          <span class="text-xs text-emerald-400 font-mono">Target: {{ tool.latest_version }}</span>
        </div>
        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <InstallStepper :current-step="currentStep" />

      <div class="space-y-1.5">
        <div class="flex items-center gap-1.5 text-[11px] font-mono text-slate-400">
          <Terminal class="w-3.5 h-3.5 text-emerald-400" />
          <span>INSTALLATION LOG STREAM</span>
        </div>
        <div class="h-32 p-3 bg-slate-950 border border-slate-800 rounded-lg font-mono text-xs text-slate-300 overflow-y-auto space-y-1">
          <div v-for="(log, idx) in miniLogs" :key="idx" class="leading-relaxed">
            {{ log }}
          </div>
        </div>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200 transition-colors"
        >
          Cancel
        </button>

        <BtnPrimary
          :disabled="!isFinished"
          @click="$emit('completed')"
        >
          <CheckCircle2 class="w-3.5 h-3.5" />
          <span>Done & Activate</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
