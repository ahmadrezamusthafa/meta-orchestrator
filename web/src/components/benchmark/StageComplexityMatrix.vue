<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { api } from '../../services/api'
import { wsService } from '../../services/websocket'
import type { BenchmarksResponseDTO, OrchestratorEvent } from '../../types'
import MethodBenchmarkMatrix from '../analytics/MethodBenchmarkMatrix.vue'
import { AlertOctagon } from 'lucide-vue-next'

const benchmarks = ref<BenchmarksResponseDTO | null>(null)
const error = ref<string | null>(null)
const isLoading = ref(false)
let seq = 0
let unsubscribe: (() => void) | null = null

async function load() {
  const mine = ++seq
  isLoading.value = true
  try {
    const b = await api.getBenchmarks()
    if (mine !== seq) return
    benchmarks.value = b
    error.value = null
  } catch (e) {
    if (mine !== seq) return
    error.value = e instanceof Error ? e.message : 'Failed to load benchmark matrix'
  } finally {
    if (mine === seq) isLoading.value = false
  }
}

function onEvent(event: OrchestratorEvent) {
  if (event.type === 'router.matrix.updated') load()
}

onMounted(() => {
  load()
  unsubscribe = wsService.subscribe(onEvent)
})

onBeforeUnmount(() => {
  unsubscribe?.()
  unsubscribe = null
})
</script>

<template>
  <div
    v-if="error"
    class="p-3 rounded-lg bg-rose-950/40 border border-rose-800 flex items-center justify-between gap-3 text-xs"
  >
    <div class="flex items-center gap-2 text-rose-300">
      <AlertOctagon class="w-4 h-4 text-rose-400" />
      <span>Benchmark matrix unavailable: {{ error }}</span>
    </div>
    <button type="button" class="h-7 px-3 rounded bg-slate-800 hover:bg-slate-700 text-slate-200" @click="load">
      Retry
    </button>
  </div>
  <div v-else-if="isLoading && !benchmarks" class="h-64 rounded-xl bg-slate-900/80 border border-slate-800 animate-pulse"></div>
  <MethodBenchmarkMatrix
    v-if="benchmarks"
    :cells="benchmarks.matrix ?? []"
    :source="benchmarks.source"
    :generated-at="benchmarks.generated_at"
    :measured-cells="benchmarks.measured_cells"
  />
</template>
