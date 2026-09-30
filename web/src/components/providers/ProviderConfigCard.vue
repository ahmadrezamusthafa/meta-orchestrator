<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import type { ProviderDTO, ProviderUsage, ProviderUsageWindow, ProviderConnection } from '../../types'
import ProviderUsagePanel from './ProviderUsagePanel.vue'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import {
  Cpu, Key, Server, Check, Eye, EyeOff, ChevronDown, ChevronUp, Save, ExternalLink, Plus,
  RefreshCw, CheckCircle2, XCircle, AlertTriangle, CircleDashed, Terminal, Trash2, Info
} from 'lucide-vue-next'

const props = defineProps<{
  provider: ProviderDTO
  connection?: ProviderConnection
  connectionLoading?: boolean
  usage?: ProviderUsage
  usageWindow: ProviderUsageWindow
  usageLoading?: boolean
}>()

const emit = defineEmits<{
  (e: 'usage-changed'): void
  (e: 'connection-changed', conn: ProviderConnection): void
  (e: 'provider-changed'): void
}>()

const toastStore = useToastStore()

// ---------------------------------------------------------------------------
// Connection status — what requests really use, verified by the backend
// ---------------------------------------------------------------------------

const isChecking = ref(false)
const conn = computed(() => props.connection)
const checking = computed(() => isChecking.value || (!!props.connectionLoading && !conn.value))

const statusView = computed(() => {
  if (checking.value) return { label: 'Checking…', pill: 'bg-slate-800 border-slate-700 text-slate-300', box: 'bg-slate-950 border-slate-800', icon: CircleDashed, iconClass: 'text-slate-400 animate-spin' }
  switch (conn.value?.status) {
    case 'connected':
      return { label: 'Connected', pill: 'bg-emerald-950/70 border-emerald-800 text-emerald-300', box: 'bg-emerald-950/20 border-emerald-900/50', icon: CheckCircle2, iconClass: 'text-emerald-400' }
    case 'expired':
      return { label: 'Login expired', pill: 'bg-amber-950/70 border-amber-800 text-amber-300', box: 'bg-amber-950/20 border-amber-900/50', icon: AlertTriangle, iconClass: 'text-amber-400' }
    case 'invalid':
      return { label: 'Key rejected', pill: 'bg-rose-950/70 border-rose-800 text-rose-300', box: 'bg-rose-950/20 border-rose-900/50', icon: XCircle, iconClass: 'text-rose-400' }
    case 'unreachable':
      return { label: "Can't verify", pill: 'bg-amber-950/70 border-amber-800 text-amber-300', box: 'bg-amber-950/20 border-amber-900/50', icon: AlertTriangle, iconClass: 'text-amber-400' }
    default:
      return { label: 'Not connected', pill: 'bg-slate-900 border-slate-700 text-slate-400', box: 'bg-slate-950 border-slate-800', icon: XCircle, iconClass: 'text-slate-500' }
  }
})

const connectionTitle = computed(() => {
  const c = conn.value
  if (!c) return 'Checking connection…'
  if (c.status === 'connected') return `Connected via ${c.label}`
  if (c.status === 'expired') return `${c.label} login expired`
  if (c.status === 'invalid') return `${c.label} rejected`
  if (c.status === 'unreachable') return `${c.label} · couldn't verify`
  return 'Not connected'
})

const now = ref(Date.now())
const nowTimer = setInterval(() => { now.value = Date.now() }, 15_000)
onUnmounted(() => clearInterval(nowTimer))
const checkedAgo = computed(() => {
  const at = conn.value?.checked_at
  if (!at) return ''
  const s = Math.max(0, Math.round((now.value - new Date(at).getTime()) / 1000))
  if (s < 60) return 'checked just now'
  const m = Math.round(s / 60)
  return m < 60 ? `checked ${m}m ago` : `checked ${Math.round(m / 60)}h ago`
})

async function recheck(silent = false) {
  isChecking.value = true
  try {
    const res = await api.testProvider(props.provider.id)
    if (res?.connection) {
      emit('connection-changed', res.connection)
      if (!silent) {
        const c: ProviderConnection = res.connection
        if (c.status === 'connected') toastStore.success(`${props.provider.name} connected`, [c.label, c.detail].filter(Boolean).join(' · '))
        else toastStore.warning(`${props.provider.name}: ${statusLabelFor(c)}`, c.hint || c.detail || '')
      }
    }
  } catch (err: any) {
    if (!silent) toastStore.error('Check failed', err.message || 'Unable to reach the orchestrator')
  } finally {
    isChecking.value = false
  }
}

function statusLabelFor(c: ProviderConnection): string {
  switch (c.status) {
    case 'expired': return 'login expired'
    case 'invalid': return 'key rejected'
    case 'unreachable': return "couldn't verify"
    case 'not_configured': return 'not connected'
    default: return c.status
  }
}

// ---------------------------------------------------------------------------
// API key — the one credential every cloud driver accepts
// ---------------------------------------------------------------------------

const keyGuide = computed(() => {
  switch (props.provider.id) {
    case 'claude':
      return { label: 'Anthropic API key', placeholder: 'sk-ant-api03-…', url: 'https://console.anthropic.com/settings/keys', urlLabel: 'console.anthropic.com' }
    case 'chatgpt':
      return { label: 'OpenAI API key', placeholder: 'sk-…', url: 'https://platform.openai.com/api-keys', urlLabel: 'platform.openai.com' }
    case 'antigravity':
      return { label: 'Gemini API key', placeholder: 'AIza…', url: 'https://aistudio.google.com/apikey', urlLabel: 'aistudio.google.com' }
    default:
      return null
  }
})

const isClaude = computed(() => props.provider.id === 'claude')
const hasKey = computed(() => !!props.provider.has_api_key)
// Claude works through the CLI login, so its key form stays folded unless a key is in play.
const showKeyForm = ref(!isClaude.value)
watch(hasKey, v => { if (v) showKeyForm.value = true }, { immediate: true })

const apiKeyInput = ref('')
const showKey = ref(false)
const isSavingKey = ref(false)
const replacingKey = ref(false)

async function saveApiKey() {
  const key = apiKeyInput.value.trim()
  if (!key) return
  isSavingKey.value = true
  try {
    const res = await api.saveProviderConfig(props.provider.id, { api_key: key })
    apiKeyInput.value = ''
    replacingKey.value = false
    if (res?.connection) emit('connection-changed', res.connection)
    emit('provider-changed')
    if (res?.warning) toastStore.warning('Key saved for this session only', res.warning)
    else if (res?.connection?.status === 'connected') toastStore.success(`${props.provider.name} key verified`, 'Saved and working')
    else toastStore.warning('Key saved, not verified', res?.connection?.detail || 'The provider could not be reached to verify it')
  } catch (err: any) {
    // The backend refuses keys the provider rejects; nothing was saved.
    toastStore.error('Key not saved', err.message || 'The provider rejected this key')
  } finally {
    isSavingKey.value = false
  }
}

async function removeApiKey() {
  try {
    const res = await api.saveProviderConfig(props.provider.id, { clear_api_key: true })
    if (res?.connection) emit('connection-changed', res.connection)
    emit('provider-changed')
    toastStore.info(`${keyGuide.value?.label || 'API key'} removed`, isClaude.value ? 'Claude now uses your Claude Code login' : '')
  } catch (err: any) {
    toastStore.error('Remove failed', err.message)
  }
}

async function removeLegacyAuth() {
  try {
    const res = await api.saveProviderConfig(props.provider.id, { session_token: '', auth_method: 'api_key' })
    if (res?.connection) emit('connection-changed', res.connection)
    emit('provider-changed')
    toastStore.info('Unused session token removed', 'Nothing changes for requests')
  } catch (err: any) {
    toastStore.error('Remove failed', err.message)
  }
}

const selectedModel = ref(props.provider.default_model)
const showAllModels = ref(false)
const isSavingModel = ref(false)

async function changeDefaultModel(model: string) {
  selectedModel.value = model
  isSavingModel.value = true
  try {
    await api.saveProviderConfig(props.provider.id, {
      model: model,
    })
    toastStore.success('Default Model Updated', `${props.provider.name} set to ${model}`)
  } catch (err: any) {
    toastStore.error('Update Failed', err.message)
  } finally {
    isSavingModel.value = false
  }
}

const showAddCustomInline = ref(false)
const customModelInput = ref('')
const isAddingCustom = ref(false)

async function addCustomModelToProvider() {
  const modelId = customModelInput.value.trim()
  if (!modelId) return

  isAddingCustom.value = true
  try {
    await api.registerCustomModel({
      provider_id: props.provider.id,
      model_id: modelId,
      model_name: modelId,
    })
    if (!props.provider.models.includes(modelId)) {
      props.provider.models.unshift(modelId)
    }
    await changeDefaultModel(modelId)
    customModelInput.value = ''
    showAddCustomInline.value = false
    toastStore.success('Custom Model Added', `${modelId} is now active for ${props.provider.name}`)
  } catch (err: any) {
    toastStore.error('Failed to add model', err.message)
  } finally {
    isAddingCustom.value = false
  }
}
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3 shadow-sm hover:border-slate-700 transition-colors">
    <!-- Header: name + one clear status -->
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-2.5 min-w-0">
        <div class="w-8 h-8 shrink-0 rounded-lg bg-slate-950 border border-slate-800 flex items-center justify-center text-sky-400">
          <Cpu class="w-4 h-4" />
        </div>
        <h3 class="text-xs font-bold text-slate-100 truncate">{{ provider.name }}</h3>
      </div>
      <span
        class="shrink-0 h-6 px-2 rounded-full border text-[10px] font-medium flex items-center gap-1"
        :class="statusView.pill"
        role="status"
      >
        <component :is="statusView.icon" class="w-3 h-3" :class="statusView.iconClass" />
        {{ statusView.label }}
      </span>
    </div>

    <!-- Connection: what requests actually use -->
    <div class="p-2.5 rounded-lg border space-y-1.5" :class="statusView.box">
      <div class="flex items-start justify-between gap-2">
        <div class="min-w-0">
          <div class="text-[11px] font-medium text-slate-100 flex items-center gap-1.5">
            <Terminal v-if="conn?.method === 'claude_cli'" class="w-3.5 h-3.5 text-slate-400 shrink-0" />
            <Key v-else-if="conn?.method === 'api_key'" class="w-3.5 h-3.5 text-slate-400 shrink-0" />
            <Server v-else-if="conn?.method === 'local_endpoint'" class="w-3.5 h-3.5 text-slate-400 shrink-0" />
            <span class="truncate">{{ connectionTitle }}</span>
          </div>
          <div v-if="conn?.detail" class="text-[10px] text-slate-400 mt-0.5 break-words">{{ conn.detail }}</div>
          <div class="text-[9px] font-mono text-slate-500 mt-0.5">
            {{ checkedAgo }}<template v-if="conn?.latency_ms"> · {{ conn.latency_ms }}ms</template>
          </div>
        </div>
        <button
          type="button"
          class="shrink-0 h-6 px-2 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-[10px] text-slate-300 hover:text-white flex items-center gap-1 disabled:opacity-50"
          :disabled="checking"
          title="Verify the connection with the provider now"
          @click="recheck()"
        >
          <RefreshCw class="w-3 h-3" :class="checking ? 'animate-spin' : ''" />
          <span>Re-check</span>
        </button>
      </div>
      <div v-if="conn?.hint && conn.status !== 'connected'" class="flex items-start gap-1.5 text-[10px] text-slate-300">
        <Info class="w-3 h-3 mt-0.5 shrink-0 text-sky-400" />
        <span class="break-words">{{ conn.hint }}</span>
      </div>
    </div>

    <!-- A session token / OAuth login saved earlier that no request uses -->
    <div v-if="conn?.legacy_auth" class="p-2.5 rounded-lg bg-amber-950/20 border border-amber-900/40 space-y-1.5">
      <div class="flex items-start gap-1.5 text-[10px] text-amber-200/90">
        <AlertTriangle class="w-3 h-3 mt-0.5 shrink-0 text-amber-400" />
        <span>
          A {{ conn.legacy_auth === 'oauth' ? 'browser (OAuth) login' : 'session token' }} saved earlier is <b>not used</b> for requests —
          {{ provider.name }} is reached through {{ conn.status === 'connected' ? conn.label : 'the method above' }}. You can safely remove it.
        </span>
      </div>
      <button
        type="button"
        class="h-6 px-2 rounded bg-slate-950 border border-amber-900/50 hover:border-amber-700 text-[10px] text-amber-200 flex items-center gap-1"
        @click="removeLegacyAuth"
      >
        <Trash2 class="w-3 h-3" />
        <span>Remove saved {{ conn.legacy_auth === 'oauth' ? 'login' : 'session token' }}</span>
      </button>
    </div>

    <div class="space-y-2 text-xs">
      <!-- Credentials -->
      <div v-if="keyGuide" class="pt-2 border-t border-slate-800/80 space-y-2">
        <!-- Claude: the CLI login is the default; an API key is an optional override -->
        <template v-if="isClaude">
          <p class="text-[10px] text-slate-400 leading-relaxed">
            Uses your <b class="text-slate-300">Claude subscription</b> through the Claude Code CLI login — no key needed.
            Sign in by running <code class="px-1 rounded bg-slate-950 text-slate-300">claude</code> in a terminal.
          </p>
          <button
            v-if="!hasKey"
            type="button"
            class="text-[10px] text-sky-400 hover:text-sky-300 flex items-center gap-1"
            :aria-expanded="showKeyForm"
            @click="showKeyForm = !showKeyForm"
          >
            <ChevronUp v-if="showKeyForm" class="w-3 h-3" />
            <ChevronDown v-else class="w-3 h-3" />
            <span>Use an Anthropic API key instead (pay-as-you-go)</span>
          </button>
        </template>

        <div v-if="showKeyForm" class="space-y-1.5">
          <label class="text-[11px] text-slate-400 flex items-center gap-1">
            <Key class="w-3 h-3 text-slate-500" />
            <span>{{ keyGuide.label }}</span>
          </label>

          <!-- Saved key -->
          <div v-if="hasKey && !replacingKey" class="flex items-center gap-1.5">
            <div class="flex-1 min-w-0 h-8 px-2.5 bg-slate-950 border border-slate-800 rounded text-xs text-slate-300 font-mono flex items-center gap-1.5">
              <Check class="w-3 h-3 text-emerald-400 shrink-0" />
              <span class="truncate">{{ provider.masked_api_key }}</span>
            </div>
            <button
              type="button"
              class="h-8 px-2 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-[10px] text-slate-300"
              @click="replacingKey = true"
            >
              Replace
            </button>
            <button
              type="button"
              class="h-8 w-8 rounded bg-slate-950 border border-slate-800 hover:border-rose-800 text-slate-400 hover:text-rose-300 flex items-center justify-center"
              title="Remove key"
              @click="removeApiKey"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </button>
          </div>

          <!-- New / replacement key -->
          <template v-else>
            <div class="flex gap-1.5">
              <input
                v-model="apiKeyInput"
                :type="showKey ? 'text' : 'password'"
                :placeholder="keyGuide.placeholder"
                autocomplete="off"
                spellcheck="false"
                class="flex-1 min-w-0 h-8 px-2.5 bg-slate-950 border border-slate-800 rounded text-xs text-slate-200 font-mono placeholder:text-slate-600 focus:outline-none focus:border-emerald-500 transition-colors"
                @keyup.enter="saveApiKey"
              />
              <button
                type="button"
                class="h-8 w-8 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 flex items-center justify-center text-slate-500 hover:text-slate-300 shrink-0"
                :title="showKey ? 'Hide key' : 'Show key'"
                @click="showKey = !showKey"
              >
                <EyeOff v-if="showKey" class="w-3.5 h-3.5" />
                <Eye v-else class="w-3.5 h-3.5" />
              </button>
              <button
                type="button"
                :disabled="isSavingKey || !apiKeyInput.trim()"
                class="h-8 px-2.5 rounded bg-emerald-950/70 hover:bg-emerald-900 border border-emerald-800/60 text-[10px] font-medium text-emerald-300 flex items-center gap-1 disabled:opacity-40 disabled:cursor-not-allowed shrink-0"
                @click="saveApiKey"
              >
                <span v-if="isSavingKey" class="animate-spin">⟳</span>
                <Save v-else class="w-3 h-3" />
                <span>{{ isSavingKey ? 'Verifying…' : 'Verify & save' }}</span>
              </button>
            </div>
            <div class="flex items-center justify-between gap-2 text-[9px] text-slate-500">
              <a :href="keyGuide.url" target="_blank" rel="noopener" class="inline-flex items-center gap-1 text-sky-400 hover:text-sky-300">
                <ExternalLink class="w-2.5 h-2.5" />
                <span>Get a key at {{ keyGuide.urlLabel }}</span>
              </a>
              <button v-if="replacingKey" type="button" class="hover:text-slate-300" @click="replacingKey = false; apiKeyInput = ''">Cancel</button>
            </div>
            <p class="text-[9px] text-slate-500">
              The key is checked with the provider before it's saved.<template v-if="isClaude"> When set, it's used instead of your Claude Code login.</template>
            </p>
          </template>
        </div>

        <p v-if="provider.id === 'antigravity'" class="text-[9px] text-slate-500 leading-relaxed">
          Signing in to the Antigravity app doesn't connect it here — requests need a Gemini API key. Your Antigravity app quota is shown below for reference only.
        </p>
      </div>

      <!-- Base URL (shown regardless of auth method) -->
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

      <!-- Model Selector & All Available Models Catalog -->
      <div class="space-y-2">
        <div class="flex items-center justify-between">
          <label class="text-[11px] text-slate-400 flex items-center gap-1">
            <Cpu class="w-3 h-3 text-slate-500" />
            <span>Default Model Endpoint</span>
          </label>
          <button
            @click="showAllModels = !showAllModels"
            type="button"
            class="text-[10px] font-mono text-sky-400 hover:text-sky-300 flex items-center gap-1 transition-colors"
          >
            <span>{{ showAllModels ? 'Hide Catalog' : `All Models (${provider.models.length})` }}</span>
            <ChevronUp v-if="showAllModels" class="w-3 h-3" />
            <ChevronDown v-else class="w-3 h-3" />
          </button>
        </div>

        <div class="relative">
          <select
            :value="selectedModel"
            :disabled="isSavingModel"
            @change="changeDefaultModel(($event.target as HTMLSelectElement).value)"
            class="w-full h-8 px-2 bg-slate-950 border border-slate-800 rounded text-xs text-slate-200 font-mono focus:outline-none focus:border-emerald-500 transition-colors"
          >
            <option v-for="m in provider.models" :key="m" :value="m">{{ m }}</option>
          </select>
          <div v-if="isSavingModel" class="absolute right-2.5 top-2 text-[10px] text-emerald-400 animate-spin">⟳</div>
        </div>

        <!-- All Available Models Pill Cloud -->
        <div v-if="showAllModels" class="p-2.5 bg-slate-950 border border-slate-800/80 rounded-lg space-y-2">
          <div class="text-[10px] text-slate-400 font-medium flex items-center justify-between">
            <span>Available AI Models:</span>
            <span class="text-slate-500 text-[9px]">Click to activate</span>
          </div>
          <div class="flex flex-wrap gap-1.5 max-h-40 overflow-y-auto pr-1">
            <button
              v-for="m in provider.models"
              :key="m"
              @click="changeDefaultModel(m)"
              type="button"
              class="px-2 py-1 rounded text-[10px] font-mono transition-all text-left flex items-center gap-1 border"
              :class="selectedModel === m
                ? 'bg-sky-950/80 border-sky-600 text-sky-200 shadow-sm'
                : 'bg-slate-900 border-slate-800 text-slate-400 hover:border-slate-700 hover:text-slate-200'"
            >
              <Check v-if="selectedModel === m" class="w-2.5 h-2.5 text-sky-400 shrink-0" />
              <span class="truncate">{{ m }}</span>
            </button>
          </div>

          <!-- Add Custom Model to Provider -->
          <div class="pt-2 border-t border-slate-800/80">
            <button
              v-if="!showAddCustomInline"
              @click="showAddCustomInline = true"
              type="button"
              class="text-[10px] text-purple-400 hover:text-purple-300 flex items-center gap-1 font-mono transition-colors"
            >
              <Plus class="w-3 h-3" />
              <span>Add custom model identifier (e.g. claude-opus-5-5)</span>
            </button>
            <div v-else class="flex items-center gap-1.5">
              <input
                v-model="customModelInput"
                type="text"
                placeholder="e.g. claude-opus-5-5"
                class="flex-1 h-7 px-2 bg-slate-900 border border-slate-700 rounded text-[11px] font-mono text-slate-200 focus:outline-none focus:border-purple-500"
                @keyup.enter="addCustomModelToProvider"
              />
              <button
                @click="addCustomModelToProvider"
                :disabled="isAddingCustom || !customModelInput.trim()"
                type="button"
                class="h-7 px-2.5 rounded bg-purple-950 hover:bg-purple-900 border border-purple-700 text-[10px] font-medium text-purple-200 disabled:opacity-40"
              >
                {{ isAddingCustom ? 'Adding…' : 'Add' }}
              </button>
              <button
                @click="showAddCustomInline = false"
                type="button"
                class="h-7 px-1.5 text-slate-500 hover:text-slate-300 text-xs"
              >
                ✕
              </button>
            </div>
          </div>
        </div>
      </div>

      <ProviderUsagePanel
        :provider-id="provider.id"
        :usage="usage"
        :window="usageWindow"
        :loading="usageLoading"
        :connection-method="conn?.method"
        @quota-saved="emit('usage-changed')"
      />
    </div>
  </div>
</template>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.auth-fade-enter-active,
.auth-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.auth-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.auth-fade-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>
