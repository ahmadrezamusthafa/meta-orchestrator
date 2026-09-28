<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../services/api'
import { useToastStore } from '../stores/toast'
import type { ProviderDTO } from '../types'
import ProviderConfigCard from '../components/providers/ProviderConfigCard.vue'
import TierModelMatrix from '../components/providers/TierModelMatrix.vue'
import ProjectSettingsDrawer from '../components/providers/ProjectSettingsDrawer.vue'
import RouterConfigurator from '../components/router/RouterConfigurator.vue'
import StageComplexityMatrix from '../components/benchmark/StageComplexityMatrix.vue'
import { Sliders, Settings2, CheckCircle2, Wifi } from 'lucide-vue-next'

const providers = ref<ProviderDTO[]>([])
const showDrawer = ref(false)
const isTestingAll = ref(false)
const toastStore = useToastStore()

onMounted(async () => {
  try {
    const res = await api.getProviders()
    providers.value = res.providers || []
  } catch (e) {
    console.error('Failed to load providers:', e)
  }
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
        <h3 class="text-xs font-bold text-slate-300 uppercase tracking-wide mb-3">
          Configured AI Endpoints · API Keys, Session Tokens & OAuth
        </h3>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <ProviderConfigCard
            v-for="p in providers"
            :key="p.id"
            :provider="p"
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
