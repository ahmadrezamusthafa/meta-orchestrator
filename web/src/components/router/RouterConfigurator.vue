<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import type { RouterMode, PriorityModelItem, RouterModeDTO, AvailableModelDTO } from '../../types'
import {
  ArrowUp, ArrowDown, Plus, Trash2, Save, RotateCcw,
  Zap, DollarSign, Layers, Cpu, Check, Search, Sparkles,
  RefreshCw, ShieldCheck, HelpCircle, Activity, ChevronDown, ChevronUp,
  X, Tag
} from 'lucide-vue-next'

const toastStore = useToastStore()

const isLoading = ref(true)
const isSaving = ref(false)
const selectedMode = ref<RouterMode>('priority_sequence')
const priorityChain = ref<PriorityModelItem[]>([])
const availableModes = ref<RouterModeDTO[]>([])
const allModels = ref<AvailableModelDTO[]>([])

// Add model state
const selectedNewModelId = ref('')
const catalogSearchQuery = ref('')
const showFullCatalog = ref(false)

// Custom model modal state
const showCustomModal = ref(false)
const isRegisteringCustom = ref(false)
const customModelForm = ref({
  provider_id: 'claude',
  model_id: 'claude-opus-5-5',
  model_name: 'Claude Opus 5.5 (Next-Gen Frontier)',
  cost_per_1k: 0.015,
  latency_ms: 250,
})

const quickPresets = [
  { provider_id: 'claude', model_id: 'claude-opus-5-5', model_name: 'Claude Opus 5.5 (Next-Gen)', cost_per_1k: 0.015, latency_ms: 250 },
  { provider_id: 'claude', model_id: 'claude-3-7-sonnet-20250219', model_name: 'Claude 3.7 Sonnet (Hybrid Reasoning)', cost_per_1k: 0.003, latency_ms: 140 },
  { provider_id: 'claude', model_id: 'claude-3-5-opus', model_name: 'Claude 3.5 Opus', cost_per_1k: 0.015, latency_ms: 260 },
  { provider_id: 'antigravity', model_id: 'gemini-2.5-pro', model_name: 'Gemini 2.5 Pro (Ultra Reasoning)', cost_per_1k: 0.00125, latency_ms: 120 },
  { provider_id: 'chatgpt', model_id: 'gpt-4.5-preview', model_name: 'OpenAI GPT-4.5 Preview (Orion)', cost_per_1k: 0.075, latency_ms: 320 },
]

function applyPreset(preset: typeof quickPresets[0]) {
  customModelForm.value = { ...preset }
}

onMounted(async () => {
  await loadRouterSettings()
})

async function loadRouterSettings() {
  isLoading.value = true
  try {
    const res = await api.getRouterSettings()
    selectedMode.value = res.mode || 'priority_sequence'
    priorityChain.value = res.priority_chain || []
    availableModes.value = res.available_modes || []
    allModels.value = res.all_models || []

    selectFirstCandidateModel()
  } catch (err: any) {
    console.error('Failed to load router settings:', err)
    toastStore.error('Router Settings Load Failed', err.message || 'Unable to connect to router API')
  } finally {
    isLoading.value = false
  }
}

function selectFirstCandidateModel() {
  const currentModelIds = new Set(priorityChain.value.map(item => item.model))
  const candidate = allModels.value.find(m => !currentModelIds.has(m.model_id))
  if (candidate) {
    selectedNewModelId.value = candidate.model_id
  } else if (allModels.value.length > 0) {
    selectedNewModelId.value = allModels.value[0].model_id
  }
}

function moveUp(index: number) {
  if (index <= 0) return
  const item = priorityChain.value[index]
  priorityChain.value.splice(index, 1)
  priorityChain.value.splice(index - 1, 0, item)
}

function moveDown(index: number) {
  if (index >= priorityChain.value.length - 1) return
  const item = priorityChain.value[index]
  priorityChain.value.splice(index, 1)
  priorityChain.value.splice(index + 1, 0, item)
}

function toggleItem(index: number) {
  priorityChain.value[index].enabled = !priorityChain.value[index].enabled
}

function removeItem(index: number) {
  priorityChain.value.splice(index, 1)
  selectFirstCandidateModel()
}

function addModelToChain(modelId?: string) {
  const targetId = modelId || selectedNewModelId.value
  if (!targetId) return

  const found = allModels.value.find(m => m.model_id === targetId)
  if (!found) return

  const existing = priorityChain.value.find(item => item.model === found.model_id)
  if (existing) {
    toastStore.warning('Already in Sequence', `${found.model_name} is already part of the priority chain`)
    return
  }

  priorityChain.value.push({
    id: `item-${Date.now()}-${Math.random().toString(36).substring(2, 6)}`,
    provider: found.provider_id,
    model: found.model_id,
    name: found.model_name,
    enabled: true,
    cost_per_1k: found.cost_per_1k,
    latency_ms: found.latency_ms,
  })

  toastStore.info('Model Added', `Added ${found.model_name} to priority sequence`)
  selectFirstCandidateModel()
}

async function submitCustomModel() {
  if (!customModelForm.value.model_id.trim()) {
    toastStore.error('Validation Error', 'Model identifier cannot be empty')
    return
  }

  isRegisteringCustom.value = true
  try {
    const res = await api.registerCustomModel({
      provider_id: customModelForm.value.provider_id,
      model_id: customModelForm.value.model_id.trim(),
      model_name: customModelForm.value.model_name.trim() || customModelForm.value.model_id.trim(),
      cost_per_1k: Number(customModelForm.value.cost_per_1k) || 0.003,
      latency_ms: Number(customModelForm.value.latency_ms) || 120,
    })

    if (res.all_models) {
      allModels.value = res.all_models
    }

    addModelToChain(customModelForm.value.model_id.trim())

    toastStore.success(
      'Model Registered & Added',
      `${customModelForm.value.model_name || customModelForm.value.model_id} added to priority sequence`
    )
    showCustomModal.value = false
  } catch (err: any) {
    toastStore.error('Registration Failed', err.message || 'Unable to register custom model')
  } finally {
    isRegisteringCustom.value = false
  }
}

async function saveSettings() {
  isSaving.value = true
  try {
    await api.updateRouterSettings({
      mode: selectedMode.value,
      priority_chain: priorityChain.value,
    })
    toastStore.success('Routing Configuration Saved', `Mode: ${selectedMode.value} · ${priorityChain.value.filter(i => i.enabled).length} active models in waterfall`)
  } catch (err: any) {
    toastStore.error('Save Failed', err.message || 'Could not persist router configuration')
  } finally {
    isSaving.value = false
  }
}

function resetToDefault() {
  priorityChain.value = [
    {
      id: 'default-0',
      provider: 'claude',
      model: 'claude-opus-5-5',
      name: 'Claude Opus 5.5 (Next-Gen Frontier)',
      enabled: true,
      cost_per_1k: 0.015,
      latency_ms: 250,
    },
    {
      id: 'default-1',
      provider: 'claude',
      model: 'claude-3-7-sonnet-20250219',
      name: 'Claude 3.7 Sonnet (Hybrid Reasoning)',
      enabled: true,
      cost_per_1k: 0.003,
      latency_ms: 140,
    },
    {
      id: 'default-2',
      provider: 'antigravity',
      model: 'gemini-2.0-flash',
      name: 'Gemini 2.0 Flash (Fast & Capable)',
      enabled: true,
      cost_per_1k: 0.0001,
      latency_ms: 65,
    },
    {
      id: 'default-3',
      provider: 'chatgpt',
      model: 'gpt-4o',
      name: 'OpenAI GPT-4o Omni',
      enabled: true,
      cost_per_1k: 0.0025,
      latency_ms: 185,
    },
    {
      id: 'default-4',
      provider: 'opencode',
      model: 'deepseek-coder-v2',
      name: 'DeepSeek Coder V2 (Local MoE)',
      enabled: true,
      cost_per_1k: 0.0,
      latency_ms: 12,
    },
  ]
  selectedMode.value = 'priority_sequence'
  toastStore.info('Defaults Restored', 'Click Save to persist default sequence')
}

const filteredCatalogModels = computed(() => {
  const q = catalogSearchQuery.value.trim().toLowerCase()
  if (!q) return allModels.value
  return allModels.value.filter(m =>
    m.model_name.toLowerCase().includes(q) ||
    m.model_id.toLowerCase().includes(q) ||
    m.provider_name.toLowerCase().includes(q)
  )
})

function getProviderBadgeColor(provider: string): string {
  switch (provider) {
    case 'claude':
      return 'bg-amber-950/80 border-amber-700/60 text-amber-300'
    case 'antigravity':
      return 'bg-sky-950/80 border-sky-700/60 text-sky-300'
    case 'chatgpt':
    case 'openai':
      return 'bg-emerald-950/80 border-emerald-700/60 text-emerald-300'
    case 'opencode':
      return 'bg-purple-950/80 border-purple-700/60 text-purple-300'
    default:
      return 'bg-slate-800 border-slate-700 text-slate-300'
  }
}

function getModeIcon(mode: RouterMode) {
  switch (mode) {
    case 'priority_sequence': return Layers
    case 'best_practice': return ShieldCheck
    case 'cost_optimized': return DollarSign
    case 'latency_optimized': return Zap
    case 'round_robin': return RefreshCw
    default: return Activity
  }
}
</script>

<template>
  <div class="p-5 bg-slate-900/90 border border-slate-800 rounded-xl space-y-6 shadow-xl relative">
    <!-- Header with Action Buttons -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-4 border-b border-slate-800">
      <div>
        <div class="flex items-center gap-2">
          <div class="p-1.5 rounded-lg bg-sky-500/10 border border-sky-500/20 text-sky-400">
            <Layers class="w-4 h-4" />
          </div>
          <h3 class="text-sm font-bold text-slate-100 uppercase tracking-wide">
            9Router Multi-Provider Routing & Priority Sorter
          </h3>
        </div>
        <p class="text-xs text-slate-400 mt-1">
          Customize automated model selection, reorder fallback priority sequence, and manage all flagship & custom AI models.
        </p>
      </div>

      <div class="flex items-center gap-2 shrink-0">
        <button
          @click="resetToDefault"
          type="button"
          class="h-8 px-3 rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-xs font-mono text-slate-300 hover:text-white flex items-center gap-1.5 transition-colors"
          title="Reset to recommended balanced sequence"
        >
          <RotateCcw class="w-3.5 h-3.5 text-slate-400" />
          <span>Reset Defaults</span>
        </button>

        <button
          @click="saveSettings"
          :disabled="isSaving || isLoading"
          type="button"
          class="h-8 px-4 rounded-lg bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-xs font-semibold text-white flex items-center gap-2 transition-all shadow-md shadow-emerald-950/40 disabled:opacity-50"
        >
          <span v-if="isSaving" class="animate-spin text-white">⟳</span>
          <Save v-else class="w-3.5 h-3.5" />
          <span>Save Routing Configuration</span>
        </button>
      </div>
    </div>

    <!-- 1. Router Mode Selector -->
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <label class="text-xs font-bold text-slate-300 uppercase tracking-wide flex items-center gap-1.5">
          <Activity class="w-3.5 h-3.5 text-sky-400" />
          <span>Select Operational Routing Mode</span>
        </label>
        <span class="text-[11px] font-mono text-slate-500">Active Mode: <span class="text-sky-400 font-semibold uppercase">{{ selectedMode.replace('_', ' ') }}</span></span>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-3">
        <div
          v-for="m in availableModes"
          :key="m.id"
          @click="selectedMode = m.id"
          class="p-3.5 rounded-xl border cursor-pointer select-none transition-all flex flex-col justify-between gap-2.5 relative group"
          :class="selectedMode === m.id
            ? 'bg-sky-950/50 border-sky-500/80 shadow-lg shadow-sky-950/40 ring-1 ring-sky-500/40'
            : 'bg-slate-950/70 border-slate-800/90 text-slate-400 hover:border-slate-700 hover:bg-slate-900/60'"
        >
          <div class="flex items-center justify-between">
            <div
              class="w-7 h-7 rounded-lg flex items-center justify-center border text-xs"
              :class="selectedMode === m.id
                ? 'bg-sky-500/20 border-sky-500/40 text-sky-300'
                : 'bg-slate-900 border-slate-800 text-slate-500 group-hover:text-slate-300'"
            >
              <component :is="getModeIcon(m.id)" class="w-3.5 h-3.5" />
            </div>

            <span
              v-if="selectedMode === m.id"
              class="px-1.5 py-0.5 rounded text-[9px] font-mono font-bold bg-sky-500 text-slate-950 uppercase"
            >
              Active
            </span>
          </div>

          <div>
            <div class="font-semibold text-xs text-slate-200 line-clamp-1">
              {{ m.name }}
            </div>
            <div class="text-[10px] text-slate-400 leading-relaxed mt-1 line-clamp-2">
              {{ m.description }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 2. Priority Sequence & Sorter (Waterfall) -->
    <div class="space-y-3 pt-2 border-t border-slate-800/80">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
        <div>
          <h4 class="text-xs font-bold text-slate-200 uppercase tracking-wide flex items-center gap-1.5">
            <Sparkles class="w-3.5 h-3.5 text-amber-400" />
            <span>AI Model Priority Chain & Sort Order</span>
          </h4>
          <p class="text-[11px] text-slate-400 mt-0.5">
            Arranged from highest priority (Rank #1) to secondary fallbacks. Use Move Up/Down to customize the sequence.
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <!-- Button to open Add Custom Model Modal -->
          <button
            @click="showCustomModal = true"
            type="button"
            class="h-7 px-2.5 rounded-lg bg-purple-950/80 hover:bg-purple-900 border border-purple-700/80 text-[11px] font-medium text-purple-300 hover:text-white flex items-center gap-1.5 transition-colors shadow-sm"
          >
            <Plus class="w-3.5 h-3.5" />
            <span>+ Custom Model</span>
          </button>

          <!-- Add Model Selector Dropdown -->
          <div class="flex items-center gap-1.5 bg-slate-950 border border-slate-800 rounded-lg px-2 py-1">
            <select
              v-model="selectedNewModelId"
              class="bg-transparent text-xs text-slate-200 font-mono focus:outline-none max-w-[210px] truncate"
            >
              <option
                v-for="m in allModels"
                :key="m.model_id"
                :value="m.model_id"
                class="bg-slate-900 text-slate-200"
              >
                [{{ m.provider_name.split(' ')[0] }}] {{ m.model_name }}
              </option>
            </select>

            <button
              @click="addModelToChain()"
              type="button"
              class="h-6 px-2 rounded bg-sky-950 hover:bg-sky-900 border border-sky-700/80 text-[11px] font-medium text-sky-300 flex items-center gap-1 transition-colors"
            >
              <Plus class="w-3 h-3" />
              <span>Add to Chain</span>
            </button>
          </div>

          <button
            @click="showFullCatalog = !showFullCatalog"
            type="button"
            class="h-7 px-2.5 rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-[11px] font-mono text-slate-300 hover:text-white flex items-center gap-1.5 transition-colors"
          >
            <span>All Models ({{ allModels.length }})</span>
            <ChevronUp v-if="showFullCatalog" class="w-3 h-3 text-sky-400" />
            <ChevronDown v-else class="w-3 h-3 text-sky-400" />
          </button>
        </div>
      </div>

      <!-- Priority Table / Waterfall List -->
      <div class="border border-slate-800 rounded-xl overflow-hidden bg-slate-950/60 shadow-inner">
        <div v-if="priorityChain.length === 0" class="p-8 text-center text-slate-500 text-xs">
          No models configured in priority sequence. Click "Reset Defaults" or "Add to Chain" above.
        </div>

        <div v-else class="divide-y divide-slate-800/80">
          <div
            v-for="(item, idx) in priorityChain"
            :key="item.id || item.model"
            class="p-3.5 flex flex-col sm:flex-row sm:items-center justify-between gap-3 hover:bg-slate-900/50 transition-colors"
            :class="!item.enabled ? 'opacity-50 bg-slate-950/40' : ''"
          >
            <!-- Left Info: Rank, Provider, Model Details -->
            <div class="flex items-center gap-3">
              <!-- Reorder Controls -->
              <div class="flex flex-col gap-0.5 shrink-0">
                <button
                  @click="moveUp(idx)"
                  :disabled="idx === 0"
                  type="button"
                  title="Move Up in Priority"
                  class="w-6 h-5 rounded bg-slate-900 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-sky-300 flex items-center justify-center disabled:opacity-20 disabled:cursor-not-allowed transition-colors"
                >
                  <ArrowUp class="w-3 h-3" />
                </button>
                <button
                  @click="moveDown(idx)"
                  :disabled="idx === priorityChain.length - 1"
                  type="button"
                  title="Move Down in Priority"
                  class="w-6 h-5 rounded bg-slate-900 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-sky-300 flex items-center justify-center disabled:opacity-20 disabled:cursor-not-allowed transition-colors"
                >
                  <ArrowDown class="w-3 h-3" />
                </button>
              </div>

              <!-- Rank Badge -->
              <div
                class="w-16 text-center py-1 rounded text-[10px] font-mono font-bold border"
                :class="idx === 0
                  ? 'bg-amber-500/20 border-amber-500/40 text-amber-300'
                  : 'bg-slate-900 border-slate-800 text-slate-400'"
              >
                {{ idx === 0 ? 'PRIMARY #1' : `FAILOVER #${idx + 1}` }}
              </div>

              <!-- Provider Badge & Model Name -->
              <div>
                <div class="flex items-center gap-2">
                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-mono font-medium border"
                    :class="getProviderBadgeColor(item.provider)"
                  >
                    {{ item.provider.toUpperCase() }}
                  </span>
                  <span class="text-xs font-semibold text-slate-100">
                    {{ item.name || item.model }}
                  </span>
                </div>
                <div class="text-[10px] font-mono text-slate-500 mt-0.5">
                  ID: <span class="text-slate-400">{{ item.model }}</span>
                </div>
              </div>
            </div>

            <!-- Right Info: Latency, Cost, Enabled Toggle, Delete -->
            <div class="flex items-center gap-3 shrink-0 self-end sm:self-auto">
              <!-- Latency Pill -->
              <div class="px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] font-mono text-emerald-400 flex items-center gap-1">
                <Zap class="w-2.5 h-2.5" />
                <span>{{ item.latency_ms || 120 }}ms</span>
              </div>

              <!-- Cost Pill -->
              <div class="px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] font-mono text-sky-400 flex items-center gap-1">
                <DollarSign class="w-2.5 h-2.5" />
                <span>${{ (item.cost_per_1k || 0).toFixed(4) }} / 1k</span>
              </div>

              <!-- Enable/Disable Switch -->
              <button
                @click="toggleItem(idx)"
                type="button"
                class="h-7 px-2 rounded text-[10px] font-mono border transition-colors flex items-center gap-1"
                :class="item.enabled
                  ? 'bg-emerald-950/50 border-emerald-700/60 text-emerald-300 hover:bg-emerald-900/50'
                  : 'bg-slate-900 border-slate-800 text-slate-500 hover:text-slate-300'"
              >
                <Check v-if="item.enabled" class="w-3 h-3" />
                <span>{{ item.enabled ? 'Enabled' : 'Disabled' }}</span>
              </button>

              <!-- Remove Button -->
              <button
                @click="removeItem(idx)"
                type="button"
                title="Remove from chain"
                class="w-7 h-7 rounded bg-slate-900 border border-slate-800 hover:border-rose-800 text-slate-500 hover:text-rose-400 flex items-center justify-center transition-colors"
              >
                <Trash2 class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 3. All Available Models Catalog (Collapsible) -->
    <div v-if="showFullCatalog" class="p-4 bg-slate-950 border border-slate-800 rounded-xl space-y-3">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-2 border-b border-slate-800">
        <div>
          <h4 class="text-xs font-bold text-slate-100 uppercase tracking-wide flex items-center gap-1.5">
            <Cpu class="w-3.5 h-3.5 text-sky-400" />
            <span>Complete AI Provider Model Catalog ({{ allModels.length }} Models Available)</span>
          </h4>
          <p class="text-[11px] text-slate-400 mt-0.5">
            All registered models ready to be prioritized across Claude (Opus 5.5, Sonnet 3.7), Gemini, OpenAI, and OpenCode.
          </p>
        </div>

        <!-- Search Bar -->
        <div class="relative w-64">
          <Search class="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-2.5" />
          <input
            v-model="catalogSearchQuery"
            type="text"
            placeholder="Search provider or model (e.g. opus, sonnet)…"
            class="w-full h-8 pl-8 pr-3 bg-slate-900 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:border-sky-500"
          />
        </div>
      </div>

      <!-- Models Grid -->
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2.5 max-h-80 overflow-y-auto pr-1">
        <div
          v-for="m in filteredCatalogModels"
          :key="m.model_id"
          class="p-2.5 rounded-lg bg-slate-900/80 border border-slate-800/90 hover:border-slate-700 flex items-center justify-between gap-2 transition-all"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-1.5">
              <span
                class="px-1.5 py-0.5 rounded text-[9px] font-mono font-medium border shrink-0"
                :class="getProviderBadgeColor(m.provider_id)"
              >
                {{ m.provider_id }}
              </span>
              <span class="text-xs font-semibold text-slate-200 truncate" :title="m.model_name">
                {{ m.model_name }}
              </span>
            </div>
            <div class="text-[10px] font-mono text-slate-500 truncate mt-0.5">
              {{ m.model_id }}
            </div>
            <div class="flex items-center gap-2 mt-1 text-[9px] font-mono text-slate-400">
              <span class="text-emerald-400">~{{ m.latency_ms }}ms</span>
              <span>•</span>
              <span class="text-sky-400">${{ m.cost_per_1k.toFixed(4) }}/1k</span>
            </div>
          </div>

          <button
            @click="addModelToChain(m.model_id)"
            type="button"
            class="h-7 px-2 rounded bg-sky-950/80 hover:bg-sky-900 border border-sky-800/80 text-[10px] font-mono text-sky-300 flex items-center gap-1 shrink-0 transition-colors"
          >
            <Plus class="w-3 h-3" />
            <span>Add</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 4. Add Custom Model Modal -->
    <div
      v-if="showCustomModal"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm"
    >
      <div class="bg-slate-900 border border-slate-700 rounded-xl shadow-2xl max-w-lg w-full p-5 space-y-4">
        <div class="flex items-center justify-between pb-3 border-b border-slate-800">
          <div class="flex items-center gap-2">
            <Tag class="w-4 h-4 text-purple-400" />
            <h3 class="text-sm font-bold text-slate-100">Register Custom AI Model Identifier</h3>
          </div>
          <button
            @click="showCustomModal = false"
            type="button"
            class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <!-- Quick Presets -->
        <div>
          <label class="block text-[11px] font-medium text-slate-400 mb-1.5">Quick Presets (Latest Flagship Models):</label>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="p in quickPresets"
              :key="p.model_id"
              @click="applyPreset(p)"
              type="button"
              class="px-2 py-1 rounded text-[10px] font-mono border transition-all"
              :class="customModelForm.model_id === p.model_id
                ? 'bg-purple-950 border-purple-500 text-purple-200'
                : 'bg-slate-950 border-slate-800 text-slate-400 hover:border-slate-700 hover:text-slate-200'"
            >
              {{ p.model_id }}
            </button>
          </div>
        </div>

        <div class="space-y-3">
          <!-- Provider -->
          <div>
            <label class="block text-xs text-slate-300 mb-1">AI Provider</label>
            <select
              v-model="customModelForm.provider_id"
              class="w-full h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-purple-500"
            >
              <option value="claude">Anthropic Claude</option>
              <option value="antigravity">Google Antigravity / Gemini</option>
              <option value="chatgpt">OpenAI ChatGPT</option>
              <option value="opencode">OpenCode / Local vLLM / Ollama</option>
            </select>
          </div>

          <!-- Model Identifier -->
          <div>
            <label class="block text-xs text-slate-300 mb-1">Model Identifier (API / CLI string)</label>
            <input
              v-model="customModelForm.model_id"
              type="text"
              placeholder="e.g. claude-opus-5-5, claude-3-7-sonnet"
              class="w-full h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono placeholder:text-slate-600 focus:outline-none focus:border-purple-500"
            />
            <span class="text-[10px] text-slate-500 mt-1 block">
              Exact string passed to Claude CLI (`claude -p`) or API client.
            </span>
          </div>

          <!-- Display Name -->
          <div>
            <label class="block text-xs text-slate-300 mb-1">Display Label</label>
            <input
              v-model="customModelForm.model_name"
              type="text"
              placeholder="e.g. Claude Opus 5.5 (Next-Gen)"
              class="w-full h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-purple-500"
            />
          </div>

          <!-- Cost & Latency Grid -->
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-[11px] text-slate-400 mb-1">Cost Per 1k Tokens (USD)</label>
              <input
                v-model.number="customModelForm.cost_per_1k"
                type="number"
                step="0.0001"
                class="w-full h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-purple-500"
              />
            </div>
            <div>
              <label class="block text-[11px] text-slate-400 mb-1">Est. Latency (ms)</label>
              <input
                v-model.number="customModelForm.latency_ms"
                type="number"
                step="5"
                class="w-full h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-purple-500"
              />
            </div>
          </div>
        </div>

        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-800">
          <button
            @click="showCustomModal = false"
            type="button"
            class="h-8 px-3 rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-xs text-slate-300 transition-colors"
          >
            Cancel
          </button>
          <button
            @click="submitCustomModel"
            :disabled="isRegisteringCustom"
            type="button"
            class="h-8 px-4 rounded-lg bg-purple-600 hover:bg-purple-500 text-xs font-semibold text-white flex items-center gap-1.5 transition-colors disabled:opacity-50"
          >
            <span v-if="isRegisteringCustom" class="animate-spin text-white">⟳</span>
            <Check v-else class="w-3.5 h-3.5" />
            <span>Save & Add to Priority Sequence</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
