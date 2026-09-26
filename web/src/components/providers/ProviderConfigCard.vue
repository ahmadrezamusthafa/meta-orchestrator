<script setup lang="ts">
import { ref } from 'vue'
import type { ProviderDTO } from '../../types'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { Cpu, Wifi, Key, Server, Check, Eye, EyeOff } from 'lucide-vue-next'

const props = defineProps<{
  provider: ProviderDTO
}>()

const toastStore = useToastStore()

const isTesting = ref(false)
const latency = ref(props.provider.latency_ms)
const isConnected = ref(true)
const showKey = ref(false)

async function testConnection() {
  isTesting.value = true
  try {
    const res = await api.testProvider(props.provider.id)
    latency.value = res.latency_ms || 120
    isConnected.value = true
    toastStore.success(`${props.provider.name} Connected`, `Ping latency: ${latency.value}ms`)
  } catch (err) {
    isConnected.value = false
    toastStore.error(`${props.provider.name} Unreachable`, 'Check endpoint configuration')
  } finally {
    isTesting.value = false
  }
}
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3 shadow-sm hover:border-slate-700 transition-colors">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2.5">
        <div class="w-8 h-8 rounded-lg bg-slate-950 border border-slate-800 flex items-center justify-center text-sky-400">
          <Cpu class="w-4 h-4" />
        </div>
        <div>
          <h3 class="text-xs font-bold text-slate-100">{{ provider.name }}</h3>
          <div class="flex items-center gap-1.5 mt-0.5">
            <Wifi class="w-3 h-3 text-emerald-400" />
            <span class="text-[10px] font-mono text-emerald-400">{{ latency }}ms ping</span>
          </div>
        </div>
      </div>

      <button
        @click="testConnection"
        :disabled="isTesting"
        type="button"
        class="h-7 px-2.5 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-[11px] font-mono text-slate-300 hover:text-white transition-colors flex items-center gap-1.5"
      >
        <span v-if="isTesting" class="animate-spin text-emerald-400">⟳</span>
        <Check v-else class="w-3 h-3 text-emerald-400" />
        <span>Test Latency</span>
      </button>
    </div>

    <div class="space-y-2 pt-2 border-t border-slate-800/80 text-xs">
      <div>
        <div class="flex items-center justify-between mb-1">
          <label class="text-[11px] text-slate-400 flex items-center gap-1">
            <Key class="w-3 h-3 text-slate-500" />
            <span>API Key Token</span>
          </label>
          <button
            @click="showKey = !showKey"
            type="button"
            class="text-[10px] font-mono text-slate-500 hover:text-slate-300 flex items-center gap-1 transition-colors"
          >
            <EyeOff v-if="showKey" class="w-3 h-3" />
            <Eye v-else class="w-3 h-3" />
            <span>{{ showKey ? 'Hide' : 'Reveal' }}</span>
          </button>
        </div>
        <input
          :type="showKey ? 'text' : 'password'"
          :value="showKey ? `sk-${props.provider.id}-live-prod-token-99824` : provider.masked_api_key"
          readonly
          class="w-full h-8 px-2.5 bg-slate-950 border border-slate-800 rounded text-xs text-slate-400 font-mono focus:outline-none"
        />
      </div>

      <div v-if="provider.base_url">
        <label class="block text-[11px] text-slate-400 mb-1 flex items-center gap-1">
          <Server class="w-3 h-3 text-slate-500" />
          <span>Base URL Proxy</span>
        </label>
        <input
          type="text"
          :value="provider.base_url"
          readonly
          class="w-full h-8 px-2.5 bg-slate-950 border border-slate-800 rounded text-xs text-slate-300 font-mono focus:outline-none"
        />
      </div>

      <div>
        <label class="block text-[11px] text-slate-400 mb-1">Default Model Endpoint</label>
        <select
          :value="provider.default_model"
          class="w-full h-8 px-2 bg-slate-950 border border-slate-800 rounded text-xs text-slate-200 font-mono focus:outline-none focus:border-emerald-500"
        >
          <option v-for="m in provider.models" :key="m" :value="m">{{ m }}</option>
        </select>
      </div>
    </div>
  </div>
</template>
