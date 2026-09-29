<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../services/api'
import { useToastStore } from '../stores/toast'
import type { ProviderDTO, ProviderUsage, ProviderUsageWindow } from '../types'
import ProviderConfigCard from '../components/providers/ProviderConfigCard.vue'
import TierModelMatrix from '../components/providers/TierModelMatrix.vue'
import ProjectSettingsDrawer from '../components/providers/ProjectSettingsDrawer.vue'
import RouterConfigurator from '../components/router/RouterConfigurator.vue'
import StageComplexityMatrix from '../components/benchmark/StageComplexityMatrix.vue'
import { Sliders, Settings2, CheckCircle2, Wifi, RefreshCw } from 'lucide-vue-next'

const providers = ref<ProviderDTO[]>([])
const showDrawer = ref(false)
const isTestingAll = ref(false)
const toastStore = useToastStore()

const usageWindow = ref<ProviderUsageWindow>('7d')
const usageByProvider = ref<Record<string, ProviderUsage>>({})
const usageLoading = ref(false)
const usageWindows: ProviderUsageWindow[] = ['24h', '7d', '30d']
let usageTimer: ReturnType<typeof setInterval> | null = null

async function loadUsage(refresh = false) {
  usageLoading.value = true
  try {
    const res = await api.getProviderUsage(usageWindow.value, refresh)
    const next: Record<string, ProviderUsage> = {}
    for (const u of res.providers || []) next[u.provider_id] = u
    usageByProvider.value = next
  } catch (e) {
    console.error('Failed to load provider usage:', e)
  } finally {
    usageLoading.value = false
  }
}

function setUsageWindow(w: ProviderUsageWindow) {
  usageWindow.value = w
  loadUsage()
}

onMounted(async () => {
  try {
    const res = await api.getProviders()
    providers.value = res.providers || []
  } catch (e) {
    console.error('Failed to load providers:', e)
  }
  loadUsage()
  usageTimer = setInterval(() => loadUsage(), 60_000)
})

onUnmounted(() => {
  if (usageTimer) clearInterval(usageTimer)
})

async function testAllConnections() {
  isTestingAll.value = true
  try {
    const promises = providers.value.map(p => api.testProvider(p.id))
    await Promise.all(promises)
    toastStore.success('All Endpoints Verified', `${providers.value.length} AI providers healthy and responsive`)
  } catch (err: any) {
    toastStore.warning('Partial Connectivity', 'Some provider endpoints reported high latency')
  } finally {
    isTestingAll.value = false
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Sub-header -->
    <div class="h-14 px-6 bg-slate-900/50 border-b border-slate-800 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <Sliders class="w-4 h-4 text-emerald-400" />
        <h2 class="text-xs font-semibold text-slate-100 uppercase tracking-wide">
          AI Provider, Router & Benchmark Settings
        </h2>
      </div>

      <div class="flex items-center gap-2.5">
        <button
          @click="testAllConnections"
          :disabled="isTestingAll || providers.length === 0"
          type="button"
          class="h-8 px-3 rounded-lg bg-emerald-950/70 hover:bg-emerald-900 border border-emerald-800 text-xs font-mono text-emerald-300 flex items-center gap-1.5 transition-colors disabled:opacity-50"
        >
          <span v-if="isTestingAll" class="animate-spin text-emerald-400">⟳</span>
          <Wifi v-else class="w-3.5 h-3.5 text-emerald-400" />
          <span>{{ isTestingAll ? 'Testing All...' : 'Test All Connections' }}</span>
        </button>

        <button
          @click="showDrawer = true"
          type="button"
          class="h-8 px-3 rounded-lg bg-slate-900 border border-slate-800 hover:border-slate-700 text-xs text-slate-200 flex items-center gap-1.5 transition-colors"
        >
          <Settings2 class="w-3.5 h-3.5 text-emerald-400" />
          <span>Project Config (.sdlc/config.yaml)</span>
        </button>
      </div>
    </div>

    <!-- Content -->
    <main class="flex-1 p-6 overflow-y-auto space-y-6">
      <!-- 1. AI Provider Cards Grid -->
      <div>
        <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
          <h3 class="text-xs font-bold text-slate-300 uppercase tracking-wide">
            Configured AI Endpoints · API Keys, Session Tokens & OAuth
          </h3>
          <div class="flex items-center gap-1.5">
            <span class="text-[10px] text-slate-500">Usage window</span>
            <div class="flex rounded-md border border-slate-800 overflow-hidden">
              <button
                v-for="w in usageWindows"
                :key="w"
                type="button"
                class="h-6 px-2 text-[10px] font-mono transition-colors"
                :class="usageWindow === w ? 'bg-sky-950 text-sky-200' : 'bg-slate-950 text-slate-400 hover:text-slate-200'"
                @click="setUsageWindow(w)"
              >
                {{ w }}
              </button>
            </div>
            <button
              type="button"
              class="h-6 w-6 rounded-md bg-slate-950 border border-slate-800 hover:border-slate-700 flex items-center justify-center text-slate-400 hover:text-slate-200"
              title="Refresh usage"
              :disabled="usageLoading"
              @click="loadUsage(true)"
            >
              <RefreshCw class="w-3 h-3" :class="usageLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <ProviderConfigCard
            v-for="p in providers"
            :key="p.id"
            :provider="p"
            :usage="usageByProvider[p.id]"
            :usage-window="usageWindow"
            :usage-loading="usageLoading"
            @usage-changed="loadUsage()"
          />
        </div>
      </div>

      <!-- 2. Tier Matrix -->
      <TierModelMatrix />

      <!-- 3. Dual Router Configurator -->
      <RouterConfigurator />

      <!-- 4. Stage x Complexity Empirical Benchmark Matrix -->
      <StageComplexityMatrix />
    </main>

    <!-- Project Settings Drawer -->
    <ProjectSettingsDrawer
      :is-open="showDrawer"
      @close="showDrawer = false"
    />
  </div>
</template>
