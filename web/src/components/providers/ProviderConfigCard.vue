<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import type { ProviderDTO, OAuthStatus } from '../../types'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import {
  Cpu, Wifi, Key, Server, Check, Eye, EyeOff,
  LogIn, LogOut, Shield, Globe, User, ChevronDown, ChevronUp, Save,
  Cookie, ExternalLink, Info, Plus
} from 'lucide-vue-next'

const props = defineProps<{
  provider: ProviderDTO
}>()

const toastStore = useToastStore()

const isTesting = ref(false)
const latency = ref(props.provider.latency_ms)
const isConnected = ref(true)
const showKey = ref(false)
const apiKeyInput = ref('')
const isSavingKey = ref(false)
const authMethod = ref<'api_key' | 'oauth' | 'session_token'>(props.provider.auth_method as any || 'api_key')
const isOAuthConnected = ref(false)
const oauthEmail = ref('')
const oauthConnectedAt = ref('')
const isOAuthLoading = ref(false)
const showAuthDropdown = ref(false)

// Session token state
const sessionTokenInput = ref('')
const isSavingSessionToken = ref(false)
const isSessionTokenConnected = ref(false)
const showSessionToken = ref(false)
const sessionTokenHint = ref('')

const supportsOAuth = computed(() => props.provider.supports_oauth)
const supportsSessionToken = computed(() => {
  // Session token is available for cloud providers (not local/vLLM)
  return ['claude', 'chatgpt', 'antigravity'].includes(props.provider.id)
})

const sessionTokenGuide = computed(() => {
  switch (props.provider.id) {
    case 'claude':
      return {
        label: 'claude.ai Session Key',
        placeholder: 'sk-ant-sid01-... or sessionKey cookie value',
        steps: [
          'Open claude.ai and log in with your account',
          'Open DevTools (F12) → Application → Cookies',
          'Find the "sessionKey" cookie and copy its value',
          'Paste the session key above'
        ],
        url: 'https://claude.ai',
        note: 'Works with Claude Free, Pro & Max subscriptions. No API plan required.'
      }
    case 'chatgpt':
      return {
        label: 'ChatGPT Access Token',
        placeholder: 'eyJhbGciOiJSUz... (access token)',
        steps: [
          'Open chatgpt.com and log in',
          'Go to chatgpt.com/api/auth/session',
          'Copy the "accessToken" value from the JSON',
          'Paste the access token above'
        ],
        url: 'https://chatgpt.com/api/auth/session',
        note: 'Works with ChatGPT Plus/Pro subscriptions. No API credits needed.'
      }
    case 'antigravity':
      return {
        label: 'Google AI Studio Cookie',
        placeholder: '__Secure-1PSID cookie value',
        steps: [
          'Open aistudio.google.com and log in',
          'Open DevTools (F12) → Application → Cookies',
          'Find "__Secure-1PSID" and copy its value',
          'Paste the cookie value above'
        ],
        url: 'https://aistudio.google.com',
        note: 'Works with your Google account. Free tier included.'
      }
    default:
      return {
        label: 'Session Token',
        placeholder: 'Paste your session token…',
        steps: ['Log in to the provider web app', 'Extract session token from browser cookies'],
        url: '',
        note: ''
      }
  }
})

// Restore auth state from provider props (backend merges saved state into GET /providers)
onMounted(async () => {
  // The provider.auth_method from the API already reflects saved state
  if (props.provider.auth_method === 'session_token') {
    authMethod.value = 'session_token'
    isSessionTokenConnected.value = true
  } else if (props.provider.auth_method === 'oauth') {
    authMethod.value = 'oauth'
  }

  // Also check the detailed status endpoint for richer data (email, expiry, etc.)
  try {
    const status = await api.getOAuthStatus(props.provider.id)
    if (status.is_connected && status.auth_method === 'oauth') {
      authMethod.value = 'oauth'
      isOAuthConnected.value = true
      oauthEmail.value = status.email || ''
      oauthConnectedAt.value = status.connected_at || ''
    } else if (status.is_connected && status.auth_method === 'session_token') {
      authMethod.value = 'session_token'
      isSessionTokenConnected.value = true
    } else if (status.is_connected && status.auth_method === 'api_key') {
      authMethod.value = 'api_key'
    }
  } catch {
    // Status endpoint not available, rely on provider props
  }
})

// Listen for postMessage from OAuth popup
function handleOAuthMessage(event: MessageEvent) {
  if (event.data?.type !== 'meta-orchestrator-oauth-callback') return
  if (event.data.provider_id !== props.provider.id) return

  isOAuthLoading.value = false

  if (event.data.status === 'success') {
    isOAuthConnected.value = true
    oauthEmail.value = event.data.email || ''
    oauthConnectedAt.value = new Date().toISOString()
    authMethod.value = 'oauth'
    toastStore.success(
      `${props.provider.name} Connected via OAuth`,
      `Authenticated as ${event.data.email || 'user'}`
    )
  } else {
    toastStore.error(
      `${props.provider.name} OAuth Failed`,
      event.data.error || 'Authorization was denied or cancelled'
    )
  }
}

onMounted(() => {
  window.addEventListener('message', handleOAuthMessage)
})

onUnmounted(() => {
  window.removeEventListener('message', handleOAuthMessage)
})

async function saveApiKey() {
  if (!apiKeyInput.value.trim()) {
    toastStore.warning('Empty API Key', 'Please enter an API key before saving')
    return
  }
  isSavingKey.value = true
  try {
    await api.saveProviderConfig(props.provider.id, { api_key: apiKeyInput.value.trim() })
    toastStore.success(`${props.provider.name} Key Saved`, 'API key configured successfully')
  } catch (err: any) {
    toastStore.error('Save Failed', err.message || 'Unable to save API key')
  } finally {
    isSavingKey.value = false
  }
}

async function saveSessionToken() {
  if (!sessionTokenInput.value.trim()) {
    toastStore.warning('Empty Session Token', 'Please paste your session token before saving')
    return
  }
  isSavingSessionToken.value = true
  try {
    await api.saveProviderConfig(props.provider.id, {
      session_token: sessionTokenInput.value.trim(),
      auth_method: 'session_token'
    })
    isSessionTokenConnected.value = true
    toastStore.success(
      `${props.provider.name} Session Configured`,
      'Session token saved — browser session authentication active'
    )
  } catch (err: any) {
    toastStore.error('Save Failed', err.message || 'Unable to save session token')
  } finally {
    isSavingSessionToken.value = false
  }
}

async function clearSessionToken() {
  try {
    await api.saveProviderConfig(props.provider.id, {
      session_token: '',
      auth_method: 'api_key'
    })
    isSessionTokenConnected.value = false
    sessionTokenInput.value = ''
    authMethod.value = 'api_key'
    toastStore.info(`${props.provider.name} Session Cleared`, 'Switched back to API Key')
  } catch (err: any) {
    toastStore.error('Clear Failed', err.message)
  }
}

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

async function startOAuthFlow() {
  isOAuthLoading.value = true
  try {
    const callbackUrl = `${window.location.origin}/api/v1/providers/oauth/callback`
    const res = await api.initiateOAuth(props.provider.id, callbackUrl)

    const width = 520
    const height = 680
    const left = window.screenX + (window.outerWidth - width) / 2
    const top = window.screenY + (window.outerHeight - height) / 2
    const popup = window.open(
      res.auth_url,
      `oauth-${props.provider.id}`,
      `width=${width},height=${height},left=${left},top=${top},toolbar=no,menubar=no,scrollbars=yes,resizable=yes`
    )

    if (!popup) {
      toastStore.warning('Popup Blocked', 'Please allow popups for this site to use OAuth login')
      isOAuthLoading.value = false
      return
    }

    const pollTimer = setInterval(() => {
      if (popup.closed) {
        clearInterval(pollTimer)
        if (isOAuthLoading.value) {
          isOAuthLoading.value = false
        }
      }
    }, 500)
  } catch (err: any) {
    isOAuthLoading.value = false
    toastStore.error('OAuth Initiation Failed', err.message || 'Unable to start OAuth flow')
  }
}

async function disconnectOAuth() {
  try {
    await api.disconnectOAuth(props.provider.id)
    isOAuthConnected.value = false
    oauthEmail.value = ''
    oauthConnectedAt.value = ''
    authMethod.value = 'api_key'
    toastStore.info(
      `${props.provider.name} Disconnected`,
      'OAuth session revoked — switched to API Key'
    )
  } catch (err: any) {
    toastStore.error('Disconnect Failed', err.message)
  }
}

function selectAuthMethod(method: 'api_key' | 'oauth' | 'session_token') {
  if (method === 'oauth' && !isOAuthConnected.value) {
    startOAuthFlow()
  } else {
    authMethod.value = method
  }
  showAuthDropdown.value = false
}

const authDropdownLabel = computed(() => {
  switch (authMethod.value) {
    case 'oauth': return 'OAuth Login'
    case 'session_token': return 'Session Token'
    default: return 'API Key'
  }
})

const authDropdownClass = computed(() => {
  switch (authMethod.value) {
    case 'oauth': return 'bg-violet-950/70 border border-violet-700/60 text-violet-300 hover:bg-violet-900/60'
    case 'session_token': return 'bg-amber-950/70 border border-amber-700/60 text-amber-300 hover:bg-amber-900/60'
    default: return 'bg-slate-950 border border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700'
  }
})

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
    <!-- Header -->
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

    <!-- Auth Method Selector -->
    <div class="pt-2 border-t border-slate-800/80">
      <div class="flex items-center justify-between mb-2">
        <label class="text-[11px] text-slate-400 flex items-center gap-1">
          <Shield class="w-3 h-3 text-slate-500" />
          <span>Authentication</span>
        </label>

        <!-- Auth method dropdown (providers with multiple auth options) -->
        <div v-if="supportsOAuth || supportsSessionToken" class="relative">
          <button
            @click="showAuthDropdown = !showAuthDropdown"
            type="button"
            class="h-6 px-2 rounded-md text-[10px] font-medium flex items-center gap-1 transition-all"
            :class="authDropdownClass"
          >
            <Globe v-if="authMethod === 'oauth'" class="w-3 h-3" />
            <Cookie v-else-if="authMethod === 'session_token'" class="w-3 h-3" />
            <Key v-else class="w-3 h-3" />
            <span>{{ authDropdownLabel }}</span>
            <ChevronDown class="w-3 h-3 opacity-60" />
          </button>

          <!-- Dropdown -->
          <Transition name="dropdown">
            <div
              v-if="showAuthDropdown"
              class="absolute right-0 top-full mt-1 w-52 bg-slate-900 border border-slate-700 rounded-lg shadow-xl z-30 overflow-hidden"
            >
              <!-- API Key option -->
              <button
                @click="selectAuthMethod('api_key')"
                type="button"
                class="w-full px-3 py-2 text-left text-[11px] flex items-center gap-2 transition-colors"
                :class="authMethod === 'api_key'
                  ? 'bg-emerald-950/40 text-emerald-300'
                  : 'text-slate-300 hover:bg-slate-800'"
              >
                <Key class="w-3.5 h-3.5 shrink-0" />
                <div>
                  <div class="font-medium">API Key Token</div>
                  <div class="text-[9px] text-slate-500 mt-0.5">Manual key from provider console</div>
                </div>
              </button>

              <!-- Session Token option -->
              <button
                v-if="supportsSessionToken"
                @click="selectAuthMethod('session_token')"
                type="button"
                class="w-full px-3 py-2 text-left text-[11px] flex items-center gap-2 border-t border-slate-800 transition-colors"
                :class="authMethod === 'session_token'
                  ? 'bg-amber-950/40 text-amber-300'
                  : 'text-slate-300 hover:bg-slate-800'"
              >
                <Cookie class="w-3.5 h-3.5 shrink-0" />
                <div>
                  <div class="font-medium">Session Token</div>
                  <div class="text-[9px] text-slate-500 mt-0.5">Browser cookie · no API plan needed</div>
                </div>
              </button>

              <!-- OAuth option -->
              <button
                v-if="supportsOAuth"
                @click="selectAuthMethod('oauth')"
                type="button"
                class="w-full px-3 py-2 text-left text-[11px] flex items-center gap-2 border-t border-slate-800 transition-colors"
                :class="authMethod === 'oauth'
                  ? 'bg-violet-950/40 text-violet-300'
                  : 'text-slate-300 hover:bg-slate-800'"
              >
                <Globe class="w-3.5 h-3.5 shrink-0" />
                <div>
                  <div class="font-medium">OAuth Browser Login</div>
                  <div class="text-[9px] text-slate-500 mt-0.5">Sign in via provider popup</div>
                </div>
              </button>
            </div>
          </Transition>
        </div>

        <!-- Non-OAuth / non-session-token providers -->
        <div v-else class="h-6 px-2 rounded-md bg-slate-950 border border-slate-800 text-[10px] font-medium text-slate-500 flex items-center gap-1">
          <Key class="w-3 h-3" />
          <span>API Key Only</span>
        </div>
      </div>
    </div>

    <!-- Auth Content -->
    <div class="space-y-2 text-xs">
      <Transition name="auth-fade" mode="out-in">

        <!-- ==================== SESSION TOKEN ==================== -->
        <!-- Session Token Connected -->
        <div v-if="authMethod === 'session_token' && isSessionTokenConnected" key="session-connected" class="space-y-2">
          <div class="p-3 bg-amber-950/20 border border-amber-800/30 rounded-lg space-y-2">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <div class="w-7 h-7 rounded-full bg-amber-900/40 border border-amber-700/40 flex items-center justify-center">
                  <Cookie class="w-3.5 h-3.5 text-amber-300" />
                </div>
                <div>
                  <div class="text-[11px] font-medium text-amber-200">Session Token Active</div>
                  <div class="text-[9px] text-amber-400/60 font-mono">Browser cookie authentication</div>
                </div>
              </div>
              <div class="flex items-center gap-1.5">
                <span class="w-1.5 h-1.5 rounded-full bg-amber-400 animate-pulse"></span>
                <span class="text-[9px] font-mono text-amber-400">Active</span>
              </div>
            </div>

            <button
              @click="clearSessionToken"
              type="button"
              class="w-full h-7 rounded-md bg-red-950/40 hover:bg-red-950/60 border border-red-800/30 hover:border-red-700/50 text-[10px] font-medium text-red-300 flex items-center justify-center gap-1.5 transition-all"
            >
              <LogOut class="w-3 h-3" />
              <span>Clear Session Token</span>
            </button>
          </div>
        </div>

        <!-- Session Token Input -->
        <div v-else-if="authMethod === 'session_token' && !isSessionTokenConnected" key="session-input" class="space-y-2.5">
          <!-- How-to guide -->
          <div class="p-2.5 bg-amber-950/15 border border-amber-900/25 rounded-lg">
            <div class="flex items-start gap-2 mb-2">
              <Info class="w-3.5 h-3.5 text-amber-400 shrink-0 mt-0.5" />
              <div class="text-[10px] text-amber-300/80 font-medium">
                How to get your {{ sessionTokenGuide.label }}
              </div>
            </div>
            <ol class="space-y-1 pl-5 list-decimal">
              <li
                v-for="(step, i) in sessionTokenGuide.steps"
                :key="i"
                class="text-[9px] text-slate-400 leading-relaxed"
              >
                {{ step }}
              </li>
            </ol>
            <a
              v-if="sessionTokenGuide.url"
              :href="sessionTokenGuide.url"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center gap-1 mt-2 text-[9px] font-medium text-amber-400 hover:text-amber-300 transition-colors"
            >
              <ExternalLink class="w-3 h-3" />
              <span>Open {{ sessionTokenGuide.url }}</span>
            </a>
          </div>

          <!-- Token input -->
          <div>
            <label class="text-[11px] text-slate-400 flex items-center gap-1 mb-1">
              <Cookie class="w-3 h-3 text-amber-500/70" />
              <span>{{ sessionTokenGuide.label }}</span>
            </label>
            <div class="flex gap-1.5">
              <input
                v-model="sessionTokenInput"
                :type="showSessionToken ? 'text' : 'password'"
                :placeholder="sessionTokenGuide.placeholder"
                class="flex-1 h-8 px-2.5 bg-slate-950 border border-slate-800 rounded text-xs text-slate-200 font-mono placeholder:text-slate-600 focus:outline-none focus:border-amber-500 transition-colors"
              />
              <button
                @click="showSessionToken = !showSessionToken"
                type="button"
                class="h-8 w-8 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 flex items-center justify-center text-slate-500 hover:text-slate-300 transition-colors shrink-0"
              >
                <EyeOff v-if="showSessionToken" class="w-3.5 h-3.5" />
                <Eye v-else class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>

          <!-- Save button -->
          <button
            @click="saveSessionToken"
            :disabled="isSavingSessionToken || !sessionTokenInput.trim()"
            type="button"
            class="w-full h-9 rounded-lg bg-gradient-to-r from-amber-700 to-orange-700 hover:from-amber-600 hover:to-orange-600 text-[11px] font-semibold text-white flex items-center justify-center gap-2 transition-all shadow-lg shadow-amber-900/20 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <span v-if="isSavingSessionToken" class="animate-spin">⟳</span>
            <Save v-else class="w-3.5 h-3.5" />
            <span>{{ isSavingSessionToken ? 'Saving…' : 'Save Session Token' }}</span>
          </button>

          <p v-if="sessionTokenGuide.note" class="text-[9px] text-slate-500 text-center leading-relaxed">
            {{ sessionTokenGuide.note }}
          </p>
        </div>

        <!-- ==================== OAUTH ==================== -->
        <!-- OAuth Connected State -->
        <div v-else-if="authMethod === 'oauth' && isOAuthConnected" key="oauth-connected" class="space-y-2">
          <div class="p-3 bg-violet-950/30 border border-violet-800/40 rounded-lg space-y-2">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <div class="w-7 h-7 rounded-full bg-violet-900/60 border border-violet-700/50 flex items-center justify-center">
                  <User class="w-3.5 h-3.5 text-violet-300" />
                </div>
                <div>
                  <div class="text-[11px] font-medium text-violet-200">{{ oauthEmail || 'Connected' }}</div>
                  <div class="text-[9px] text-violet-400/70 font-mono">OAuth 2.0 · Bearer Token</div>
                </div>
              </div>
              <div class="flex items-center gap-1.5">
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                <span class="text-[9px] font-mono text-emerald-400">Active</span>
              </div>
            </div>

            <button
              @click="disconnectOAuth"
              type="button"
              class="w-full h-7 rounded-md bg-red-950/40 hover:bg-red-950/60 border border-red-800/30 hover:border-red-700/50 text-[10px] font-medium text-red-300 flex items-center justify-center gap-1.5 transition-all"
            >
              <LogOut class="w-3 h-3" />
              <span>Disconnect OAuth Session</span>
            </button>
          </div>
        </div>

        <!-- OAuth Login Button -->
        <div v-else-if="authMethod === 'oauth' && !isOAuthConnected" key="oauth-login" class="space-y-2">
          <button
            @click="startOAuthFlow"
            :disabled="isOAuthLoading"
            type="button"
            class="w-full h-10 rounded-lg bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-500 hover:to-indigo-500 text-[11px] font-semibold text-white flex items-center justify-center gap-2 transition-all shadow-lg shadow-violet-900/30 disabled:opacity-60"
          >
            <span v-if="isOAuthLoading" class="animate-spin text-white">⟳</span>
            <LogIn v-else class="w-4 h-4" />
            <span>{{ isOAuthLoading ? 'Waiting for authorization…' : `Sign in to ${provider.name}` }}</span>
          </button>
          <p class="text-[9px] text-slate-500 text-center leading-relaxed">
            Opens a secure browser window for authentication. No API key required.
          </p>
        </div>

        <!-- ==================== API KEY ==================== -->
        <div v-else key="api-key" class="space-y-2">
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
            <div class="flex gap-1.5">
              <input
                v-model="apiKeyInput"
                :type="showKey ? 'text' : 'password'"
                :placeholder="provider.masked_api_key || 'Enter your API key…'"
                class="flex-1 h-8 px-2.5 bg-slate-950 border border-slate-800 rounded text-xs text-slate-200 font-mono placeholder:text-slate-600 focus:outline-none focus:border-emerald-500 transition-colors"
              />
              <button
                @click="saveApiKey"
                :disabled="isSavingKey || !apiKeyInput.trim()"
                type="button"
                class="h-8 px-2.5 rounded bg-emerald-950/70 hover:bg-emerald-900 border border-emerald-800/60 text-[10px] font-medium text-emerald-300 flex items-center gap-1 transition-all disabled:opacity-40 disabled:cursor-not-allowed"
              >
                <span v-if="isSavingKey" class="animate-spin">⟳</span>
                <Save v-else class="w-3 h-3" />
                <span>Save</span>
              </button>
            </div>
          </div>
        </div>
      </Transition>

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
              <span>+ Add custom model identifier (e.g. claude-opus-5-5)</span>
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
