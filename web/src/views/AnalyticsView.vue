<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { api } from '../services/api'
import { wsService } from '../services/websocket'
import type {
  TelemetryWindow, TelemetrySummaryDTO, TelemetryTrendsDTO, BenchmarksResponseDTO, OrchestratorEvent,
} from '../types'
import ModelLeaderboard from '../components/analytics/ModelLeaderboard.vue'
import MethodBenchmarkMatrix from '../components/analytics/MethodBenchmarkMatrix.vue'
import TokenBurnChart from '../components/analytics/TokenBurnChart.vue'
import MethodRoiChart from '../components/analytics/MethodRoiChart.vue'
import MttrGauge from '../components/analytics/MttrGauge.vue'
import StabilityIndex from '../components/analytics/StabilityIndex.vue'
import { fmtInt, fmtCompact, fmtUsd } from '../components/analytics/analyticsFormat'
import { LineChart, RefreshCw, AlertOctagon, Radio } from 'lucide-vue-next'

const windows: Array<{ value: TelemetryWindow; label: string }> = [
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
  { value: 'all', label: 'All-time' },
]

const selectedWindow = ref<TelemetryWindow>('7d')
const selectedRepo = ref('')

const summary = ref<TelemetrySummaryDTO | null>(null)
const trends = ref<TelemetryTrendsDTO | null>(null)
const benchmarks = ref<BenchmarksResponseDTO | null>(null)

const isLoading = ref(false)
const isBenchLoading = ref(false)
const telemetryError = ref<string | null>(null)
const benchError = ref<string | null>(null)
const liveFlash = ref(false)

// Keep repo options stable even when a filtered summary narrows `repos`.
const knownRepos = ref<string[]>([])
const repoOptions = computed(() => {
  const set = new Set([...knownRepos.value, ...(summary.value?.repos ?? [])])
  if (selectedRepo.value) set.add(selectedRepo.value)
  return [...set].sort()
})

let telemetrySeq = 0
let benchSeq = 0

async function loadTelemetry(opts: { silent?: boolean } = {}) {
  const seq = ++telemetrySeq
  if (!opts.silent) isLoading.value = true
  try {
    const [s, t] = await Promise.all([
      api.getTelemetrySummary(selectedWindow.value, selectedRepo.value),
      api.getTelemetryTrends(selectedWindow.value, selectedRepo.value),
    ])
    if (seq !== telemetrySeq) return // stale response
    summary.value = s
    trends.value = t
    telemetryError.value = null
    if (!selectedRepo.value && s.repos?.length) knownRepos.value = [...s.repos]
  } catch (e) {
    if (seq !== telemetrySeq) return
    telemetryError.value = e instanceof Error ? e.message : 'Failed to load telemetry'
  } finally {
    if (seq === telemetrySeq) isLoading.value = false
  }
}

async function loadBenchmarks() {
  const seq = ++benchSeq
  isBenchLoading.value = true
  try {
    const b = await api.getBenchmarks()
    if (seq !== benchSeq) return
    benchmarks.value = b
    benchError.value = null
  } catch (e) {
    if (seq !== benchSeq) return
    benchError.value = e instanceof Error ? e.message : 'Failed to load benchmark matrix'
  } finally {
    if (seq === benchSeq) isBenchLoading.value = false
  }
}

function refreshAll() {
  loadTelemetry()
  loadBenchmarks()
}

watch([selectedWindow, selectedRepo], () => loadTelemetry())

// ─── Live updates ──────────────────────────────────────────────────────────
const TOKEN_DEBOUNCE_MS = 2000
let tokenTimer: ReturnType<typeof setTimeout> | null = null
let flashTimer: ReturnType<typeof setTimeout> | null = null
let unsubscribe: (() => void) | null = null

function flash() {
  liveFlash.value = true
  if (flashTimer) clearTimeout(flashTimer)
  flashTimer = setTimeout(() => { liveFlash.value = false }, 1200)
}

function onEvent(event: OrchestratorEvent) {
  if (event.type === 'router.matrix.updated') {
    flash()
    loadBenchmarks()
  } else if (event.type === 'telemetry.tokens.consumed') {
    // Ignore events for other repos when a repo filter is active.
    const repo = event.payload?.repo
    if (selectedRepo.value && repo && repo !== selectedRepo.value) return
    if (tokenTimer) clearTimeout(tokenTimer)
    tokenTimer = setTimeout(() => {
      tokenTimer = null
      flash()
      loadTelemetry({ silent: true })
    }, TOKEN_DEBOUNCE_MS)
  }
}

onMounted(() => {
  refreshAll()
  unsubscribe = wsService.subscribe(onEvent)
})

onBeforeUnmount(() => {
  unsubscribe?.()
  unsubscribe = null
  if (tokenTimer) clearTimeout(tokenTimer)
  if (flashTimer) clearTimeout(flashTimer)
})

const totals = computed(() => summary.value?.totals)
const isEmpty = computed(() => !!summary.value && (summary.value.totals?.runs ?? 0) === 0)
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Sub-header Toolbar -->
    <div class="h-14 px-6 bg-slate-900/50 border-b border-slate-800 flex items-center justify-between gap-4">
      <div class="flex items-center gap-2">
        <LineChart class="w-4 h-4 text-emerald-400" />
        <h2 class="text-xs font-semibold text-slate-100 uppercase tracking-wide">Executive Analytics</h2>
        <span
          class="ml-2 flex items-center gap-1 text-[10px] font-mono"
          :class="wsService.isConnected.value ? 'text-emerald-400' : 'text-slate-500'"
          :title="wsService.isConnected.value ? 'Live updates connected' : 'Live updates disconnected'"
        >
          <Radio class="w-3 h-3" :class="{ 'animate-pulse': liveFlash }" />
          {{ wsService.isConnected.value ? 'LIVE' : 'OFFLINE' }}
        </span>
      </div>

      <div class="flex items-center gap-2.5">
        <div class="flex items-center gap-1" role="group" aria-label="Timeframe">
          <button
            v-for="w in windows"
            :key="w.value"
            type="button"
            class="h-8 px-3 rounded-lg text-xs font-medium transition-colors"
            :class="selectedWindow === w.value ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
            @click="selectedWindow = w.value"
          >
            {{ w.label }}
          </button>
        </div>

        <select
          v-model="selectedRepo"
          aria-label="Repository"
          class="h-8 px-2 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
        >
          <option value="">All repositories</option>
          <option v-for="r in repoOptions" :key="r" :value="r">{{ r }}</option>
        </select>

        <button
          type="button"
          title="Refresh analytics"
          :disabled="isLoading"
          class="h-8 w-8 flex items-center justify-center rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
          @click="refreshAll"
        >
          <RefreshCw class="w-4 h-4" :class="{ 'animate-spin text-emerald-400': isLoading || isBenchLoading }" />
        </button>
      </div>
    </div>

    <main class="flex-1 p-6 overflow-y-auto space-y-6">
      <!-- Telemetry error -->
      <div
        v-if="telemetryError"
        class="p-3 rounded-lg bg-rose-950/40 border border-rose-800 flex items-center justify-between gap-3 text-xs"
      >
        <div class="flex items-center gap-2 text-rose-300">
          <AlertOctagon class="w-4 h-4 text-rose-400" />
          <span>Telemetry unavailable: {{ telemetryError }}</span>
        </div>
        <button type="button" class="h-7 px-3 rounded bg-slate-800 hover:bg-slate-700 text-slate-200" @click="loadTelemetry()">
          Retry
        </button>
      </div>

      <!-- Initial loading skeleton -->
      <div v-if="isLoading && !summary" class="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <div v-for="i in 4" :key="i" class="h-20 rounded-xl bg-slate-900/80 border border-slate-800 animate-pulse"></div>
      </div>

      <template v-if="summary">
        <!-- KPI strip -->
        <div class="grid grid-cols-2 lg:grid-cols-4 gap-4" :class="{ 'opacity-60 transition-opacity': isLoading }">
          <div class="p-4 rounded-xl bg-slate-900/80 border border-slate-800">
            <div class="text-[10px] uppercase tracking-wide text-slate-400">Runs</div>
            <div class="mt-1 text-sky-400 font-mono text-lg">{{ fmtInt(totals?.runs) }}</div>
          </div>
          <div class="p-4 rounded-xl bg-slate-900/80 border border-slate-800">
            <div class="text-[10px] uppercase tracking-wide text-slate-400">Total Tokens</div>
            <div class="mt-1 text-sky-400 font-mono text-lg" :title="fmtInt(totals?.total_tokens)">{{ fmtCompact(totals?.total_tokens) }}</div>
            <div class="text-[10px] font-mono text-slate-500">cached {{ fmtCompact(totals?.cached_tokens) }}</div>
          </div>
          <div class="p-4 rounded-xl bg-slate-900/80 border border-slate-800">
            <div class="text-[10px] uppercase tracking-wide text-slate-400">Spend</div>
            <div class="mt-1 text-sky-400 font-mono text-lg">{{ fmtUsd(totals?.cost_usd) }}</div>
          </div>
          <div class="p-4 rounded-xl bg-slate-900/80 border border-slate-800">
            <div class="text-[10px] uppercase tracking-wide text-slate-400">Query Latency</div>
            <div class="mt-1 text-sky-400 font-mono text-lg">{{ (summary.query_latency_ms ?? 0).toFixed(1) }}ms</div>
          </div>
        </div>

        <div
          v-if="isEmpty"
          class="p-4 rounded-xl bg-slate-900/60 border border-dashed border-slate-700 text-xs text-slate-400 text-center"
        >
          No telemetry recorded for
          <span class="font-mono text-slate-200">{{ selectedWindow }}</span>
          <template v-if="selectedRepo"> in <span class="font-mono text-slate-200">{{ selectedRepo }}</span></template>.
          Run a task to start collecting token, cost and resolution metrics.
        </div>

        <MttrGauge
          :fpvr-percent="totals?.fpvr_percent ?? 0"
          :mttr-seconds="totals?.mttr_seconds ?? 0"
          :runs="totals?.runs ?? 0"
        />

        <div class="grid grid-cols-1 xl:grid-cols-3 gap-4">
          <div class="xl:col-span-2">
            <TokenBurnChart :points="trends?.burn ?? []" :bucket="trends?.bucket" />
          </div>
          <MethodRoiChart :methods="summary.methods ?? []" />
        </div>

        <div class="grid grid-cols-1 xl:grid-cols-3 gap-4">
          <div class="xl:col-span-2">
            <ModelLeaderboard :entries="summary.leaderboard ?? []" />
          </div>
          <StabilityIndex :stability="summary.stability ?? []" />
        </div>
      </template>

      <!-- Benchmark matrix (independent of telemetry window/repo) -->
      <div
        v-if="benchError"
        class="p-3 rounded-lg bg-rose-950/40 border border-rose-800 flex items-center justify-between gap-3 text-xs"
      >
        <div class="flex items-center gap-2 text-rose-300">
          <AlertOctagon class="w-4 h-4 text-rose-400" />
          <span>Benchmark matrix unavailable: {{ benchError }}</span>
        </div>
        <button type="button" class="h-7 px-3 rounded bg-slate-800 hover:bg-slate-700 text-slate-200" @click="loadBenchmarks">
          Retry
        </button>
      </div>
      <div v-else-if="isBenchLoading && !benchmarks" class="h-64 rounded-xl bg-slate-900/80 border border-slate-800 animate-pulse"></div>
      <MethodBenchmarkMatrix
        v-if="benchmarks"
        :cells="benchmarks.matrix ?? []"
        :source="benchmarks.source"
        :generated-at="benchmarks.generated_at"
      />
    </main>
  </div>
</template>
