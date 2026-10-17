<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import type { ToolDTO, ToolchainJob, ToolManualInstall } from '../../types'
import { useToolsStore } from '../../stores/tools'
import { toolchainApi } from '../../services/toolchain'
import BtnPrimary from '../common/BtnPrimary.vue'
import { Terminal, X, CheckCircle2, ExternalLink, Loader2 } from 'lucide-vue-next'

const props = defineProps<{
  tool: ToolDTO
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'completed'): void
}>()

const toolsStore = useToolsStore()
const job = ref<ToolchainJob | null>(null)
const manual = ref<{ reason: string; install: ToolManualInstall } | null>(null)
const error = ref('')
let stopped = false

onMounted(async () => {
  try {
    const res = await toolsStore.installTool(props.tool.id)
    if (res.kind === 'manual') {
      manual.value = { reason: res.reason, install: res.install }
      return
    }
    job.value = res.job
    while (!stopped && job.value.state === 'running') {
      await new Promise((r) => setTimeout(r, 1000))
      try {
        job.value = await toolchainApi.job(job.value.id)
      } catch {
        // the daemon restarted or the network blinked: keep following
      }
    }
  } catch (err: any) {
    error.value = err?.message || 'The install could not start'
  }
})

onBeforeUnmount(() => {
  stopped = true
})

function copy(text: string) {
  void navigator.clipboard?.writeText(text)
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-5">
      <div class="flex items-center justify-between pb-3 border-b border-slate-800">
        <div>
          <h3 class="text-sm font-bold text-slate-100">Install {{ tool.name }}</h3>
          <span v-if="job" class="text-xs text-emerald-400 font-mono">$ {{ job.command }}</span>
        </div>
        <button @click="emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <p v-if="error" class="text-xs text-rose-300">{{ error }}</p>

      <!-- No unattended installer: show how to install it by hand -->
      <div v-else-if="manual" class="space-y-3 text-xs text-slate-300">
        <p>{{ manual.reason }}</p>
        <div v-for="cmd in manual.install.commands || []" :key="cmd" class="flex items-center gap-2">
          <code class="flex-1 px-2.5 py-1.5 rounded bg-slate-950 border border-slate-800 font-mono text-slate-200">{{ cmd }}</code>
          <button type="button" class="text-[11px] text-slate-400 hover:text-slate-200" @click="copy(cmd)">Copy</button>
        </div>
        <p v-if="manual.install.note" class="text-slate-400">{{ manual.install.note }}</p>
        <a v-if="manual.install.docs" :href="manual.install.docs" target="_blank" rel="noopener"
          class="inline-flex items-center gap-1 text-emerald-400 hover:text-emerald-300">
          <ExternalLink class="w-3.5 h-3.5" /> Installation docs
        </a>
        <p class="text-slate-500">After installing, use Re-check on the Tool Hub.</p>
      </div>

      <!-- Toolchain install job: its real log -->
      <div v-else class="space-y-1.5">
        <div class="flex items-center gap-1.5 text-[11px] font-mono text-slate-400">
          <Terminal class="w-3.5 h-3.5 text-emerald-400" />
          <span>INSTALL LOG</span>
          <Loader2 v-if="!job || job.state === 'running'" class="w-3.5 h-3.5 animate-spin" />
        </div>
        <div class="h-40 p-3 bg-slate-950 border border-slate-800 rounded-lg font-mono text-xs text-slate-300 overflow-y-auto space-y-1">
          <div v-for="(line, idx) in job?.log || []" :key="idx" class="leading-relaxed whitespace-pre-wrap">{{ line }}</div>
        </div>
        <p v-if="job && job.state !== 'running'" class="text-xs"
          :class="job.state === 'succeeded' ? 'text-emerald-300' : 'text-rose-300'">{{ job.message }}</p>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button @click="emit('close')" type="button" class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200 transition-colors">
          Close
        </button>
        <BtnPrimary v-if="job" :disabled="job.state !== 'succeeded'" @click="emit('completed')">
          <CheckCircle2 class="w-3.5 h-3.5" />
          <span>Done</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
