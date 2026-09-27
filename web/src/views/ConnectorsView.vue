<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { api } from '../services/api'
import { useToastStore } from '../stores/toast'
import { wsService } from '../services/websocket'
import type { 
  ConnectorItem, ConnectorCategory, TestConnectorResponse, 
  JiraIssueDTO, MCPConfig, ConnectorPingConfig, PingAllSummary 
} from '../types'
import JiraImportModal from '../components/kanban/JiraImportModal.vue'
import ConfirmDeleteModal from '../components/common/ConfirmDeleteModal.vue'
import { 
  Plug, CheckCircle2, AlertCircle, RefreshCw, Wifi, Save, ExternalLink, 
  Eye, EyeOff, Search, Download, FileText, Check, ShieldCheck, ArrowRight, 
  ArrowLeft, Layers, GitBranch, MessageSquare, BookOpen, Globe, Sliders, 
  CheckSquare, Sparkles, Zap, Activity, Settings, Cpu, Terminal, Copy, Plus, Trash2, Code,
  Radio, Clock, ChevronDown, Play, Pause
} from 'lucide-vue-next'

const toastStore = useToastStore()

// View Modes: 'list' (catalog gallery) vs 'edit' (setup/edit connector)
const currentView = ref<'list' | 'edit'>('list')
const selectedConnector = ref<ConnectorItem | null>(null)
const activeEditTab = ref<'rest' | 'mcp' | 'explorer'>('rest')

// Catalog state
const catalog = ref<ConnectorItem[]>([])
const isLoading = ref(true)
const searchQuery = ref('')
const activeCategory = ref<'all' | ConnectorCategory>('all')
const togglingIds = ref<Set<string>>(new Set())

// Edit & Diagnostics state
const isSaving = ref(false)
const isTesting = ref(false)
const testResult = ref<TestConnectorResponse | null>(null)
const showToken = ref(false)

// MCP State
const isTestingMCP = ref(false)
const mcpTestResult = ref<TestConnectorResponse | null>(null)
const showMCPExportModal = ref(false)
const mcpExportJSON = ref('')
const copiedMCP = ref(false)
const newEnvKey = ref('')
const newEnvVal = ref('')

// Periodic Ping State
const isPingingAll = ref(false)
const pingConfig = ref<ConnectorPingConfig>({ enabled: true, interval_seconds: 30 })
const showPingSettings = ref(false)
const isUpdatingPing = ref(false)
const recentlyPingedIds = ref<Set<string>>(new Set())
const lastPingTime = ref<Date | null>(null)
let unsubscribeWS: (() => void) | null = null
let backgroundSyncTimer: any = null

// Explorer Tab State (for JIRA)
const explorerQuery = ref('')
const explorerIssues = ref<JiraIssueDTO[]>([])
const isSearchingExplorer = ref(false)
const showImportModal = ref(false)

// Categories metadata
const categoryTabs = [
  { key: 'all', label: 'All Connectors' },
  { key: 'issue_tracker', label: 'Issue Trackers' },
  { key: 'documentation', label: 'Documentation' },
  { key: 'chatops', label: 'ChatOps & Alerts' },
  { key: 'vcs', label: 'Source Control (VCS)' },
  { key: 'custom', label: 'Custom Webhooks' },
]

// Computed Stats (Real Statuses only!)
const stats = computed(() => {
  const total = catalog.value.length
  const enabledCount = catalog.value.filter(c => c.enabled).length
  const connectedCount = catalog.value.filter(c => c.status === 'connected' && c.enabled).length
  const mcpCount = catalog.value.filter(c => c.mcp && c.mcp.enabled).length
  return { total, enabledCount, connectedCount, mcpCount }
})

// Filtered Catalog
const filteredCatalog = computed(() => {
  return catalog.value.filter(c => {
    // Category filter
    if (activeCategory.value !== 'all' && c.category !== activeCategory.value) {
      return false
    }
    // Search query filter
    if (searchQuery.value.trim()) {
      const q = searchQuery.value.toLowerCase().trim()
      const matchName = c.name.toLowerCase().includes(q)
      const matchId = c.id.toLowerCase().includes(q)
      const matchDesc = c.description.toLowerCase().includes(q)
      const matchCategory = c.category_label?.toLowerCase().includes(q)
      const matchCaps = c.capabilities?.some(cap => cap.toLowerCase().includes(q))
      return matchName || matchId || matchDesc || matchCategory || matchCaps
    }
    return true
  })
})

function getCategoryCount(catKey: string): number {
  if (catKey === 'all') return catalog.value.length
  return catalog.value.filter(c => c.category === catKey).length
}

// Brand Visual Helpers
function getBrandColor(c: ConnectorItem) {
  switch (c.id) {
    case 'jira':
      return {
        bg: 'bg-blue-500/10',
        border: 'border-blue-500/30',
        text: 'text-blue-400',
        badgeBg: 'bg-blue-950 text-blue-300 border-blue-800/60',
        accentBorder: 'group-hover:border-blue-500/50',
        gradient: 'from-blue-600/10 via-transparent to-transparent'
      }
    case 'confluence':
      return {
        bg: 'bg-indigo-500/10',
        border: 'border-indigo-500/30',
        text: 'text-indigo-400',
        badgeBg: 'bg-indigo-950 text-indigo-300 border-indigo-800/60',
        accentBorder: 'group-hover:border-indigo-500/50',
        gradient: 'from-indigo-600/10 via-transparent to-transparent'
      }
    case 'github':
      return {
        bg: 'bg-slate-700/30',
        border: 'border-slate-600/40',
        text: 'text-slate-200',
        badgeBg: 'bg-slate-800 text-slate-300 border-slate-700',
        accentBorder: 'group-hover:border-slate-500/60',
        gradient: 'from-slate-700/20 via-transparent to-transparent'
      }
    case 'gitlab':
      return {
        bg: 'bg-amber-500/10',
        border: 'border-amber-500/30',
        text: 'text-amber-400',
        badgeBg: 'bg-amber-950 text-amber-300 border-amber-800/60',
        accentBorder: 'group-hover:border-amber-500/50',
        gradient: 'from-amber-600/10 via-transparent to-transparent'
      }
    case 'bitbucket':
      return {
        bg: 'bg-cyan-500/10',
        border: 'border-cyan-500/30',
        text: 'text-cyan-400',
        badgeBg: 'bg-cyan-950 text-cyan-300 border-cyan-800/60',
        accentBorder: 'group-hover:border-cyan-500/50',
        gradient: 'from-cyan-600/10 via-transparent to-transparent'
      }
    case 'slack':
      return {
        bg: 'bg-purple-500/10',
        border: 'border-purple-500/30',
        text: 'text-purple-400',
        badgeBg: 'bg-purple-950 text-purple-300 border-purple-800/60',
        accentBorder: 'group-hover:border-purple-500/50',
        gradient: 'from-purple-600/10 via-transparent to-transparent'
      }
    case 'linear':
      return {
        bg: 'bg-violet-500/10',
        border: 'border-violet-500/30',
        text: 'text-violet-400',
        badgeBg: 'bg-violet-950 text-violet-300 border-violet-800/60',
        accentBorder: 'group-hover:border-violet-500/50',
        gradient: 'from-violet-600/10 via-transparent to-transparent'
      }
    case 'notion':
      return {
        bg: 'bg-stone-700/30',
        border: 'border-stone-600/40',
        text: 'text-stone-300',
        badgeBg: 'bg-stone-800 text-stone-300 border-stone-700',
        accentBorder: 'group-hover:border-stone-500/60',
        gradient: 'from-stone-700/20 via-transparent to-transparent'
      }
    default:
      return {
        bg: 'bg-emerald-500/10',
        border: 'border-emerald-500/30',
        text: 'text-emerald-400',
        badgeBg: 'bg-emerald-950 text-emerald-300 border-emerald-800/60',
        accentBorder: 'group-hover:border-emerald-500/50',
        gradient: 'from-emerald-600/10 via-transparent to-transparent'
      }
  }
}

// Load Catalog
async function loadCatalog() {
  isLoading.value = true
  try {
    const items = await api.getConnectorCatalog()
    catalog.value = items
  } catch (err: any) {
    toastStore.error('Connectors', 'Failed to load connector catalog')
  } finally {
    isLoading.value = false
  }
}

// Quick Card-Level Enable / Disable Toggle
async function handleToggle(c: ConnectorItem, event?: Event) {
  if (event) {
    event.stopPropagation()
  }
  const nextState = !c.enabled
  togglingIds.value.add(c.id)
  try {
    const updated = await api.toggleConnector(c.id, nextState)
    c.enabled = updated.enabled
    c.status = updated.status
    if (selectedConnector.value && selectedConnector.value.id === c.id) {
      selectedConnector.value.enabled = updated.enabled
      selectedConnector.value.status = updated.status
    }
    toastStore.success(
      c.name,
      nextState ? 'Connector enabled successfully' : 'Connector disabled'
    )
  } catch (err: any) {
    toastStore.error('Toggle Failed', err.message)
  } finally {
    togglingIds.value.delete(c.id)
  }
}

// Enter Setup / Edit Mode
function openConnectorSetup(c: ConnectorItem) {
  selectedConnector.value = JSON.parse(JSON.stringify(c))
  if (!selectedConnector.value?.extra_settings) {
    selectedConnector.value!.extra_settings = {}
  }
  // Initialize default toggle values if undefined
  const es = selectedConnector.value!.extra_settings
  if (es.auto_detect_keys === undefined) es.auto_detect_keys = true
  if (es.auto_sync_status === undefined) es.auto_sync_status = true
  if (es.auto_publish_tech_docs === undefined) es.auto_publish_tech_docs = true
  if (es.auto_publish_prd === undefined) es.auto_publish_prd = true
  if (es.auto_sync_prs === undefined) es.auto_sync_prs = true
  if (es.sync_pipelines === undefined) es.sync_pipelines = true
  if (es.post_channel_notifications === undefined) es.post_channel_notifications = true
  if (es.sync_pull_requests === undefined) es.sync_pull_requests = true
  if (es.sign_payloads === undefined) es.sign_payloads = true

  if (!selectedConnector.value?.mcp) {
    selectedConnector.value!.mcp = {
      enabled: false,
      command: 'npx',
      args: [],
      env: {},
      transport: 'stdio'
    }
  }
  if (!selectedConnector.value?.mcp.env) {
    selectedConnector.value!.mcp.env = {}
  }
  testResult.value = null
  mcpTestResult.value = null
  showToken.value = false
  activeEditTab.value = 'rest'
  currentView.value = 'edit'

  // If JIRA, trigger explorer search in background
  if (c.id === 'jira') {
    searchExplorer()
  }
}

// Back to Cards Catalog
async function returnToCatalog() {
  currentView.value = 'list'
  selectedConnector.value = null
  testResult.value = null
  mcpTestResult.value = null
  await loadCatalog()
}

// Save Changes
async function saveConnector() {
  if (!selectedConnector.value) return
  isSaving.value = true
  try {
    const updated = await api.updateConnector(selectedConnector.value.id, selectedConnector.value)
    selectedConnector.value = { ...updated }
    
    // In-place catalog update
    const idx = catalog.value.findIndex(item => item.id === updated.id)
    if (idx !== -1) {
      catalog.value[idx] = { ...updated }
    }
    
    toastStore.success('Settings Saved', `${updated.name} configuration updated successfully`)
  } catch (err: any) {
    toastStore.error('Save Failed', err.message)
  } finally {
    isSaving.value = false
  }
}

// Real REST API Connection Test
async function testConnection() {
  if (!selectedConnector.value) return
  isTesting.value = true
  testResult.value = null
  try {
    const res = await api.testGenericConnector(selectedConnector.value.id, selectedConnector.value)
    testResult.value = res
    if (res.success) {
      selectedConnector.value.status = 'connected'
      selectedConnector.value.latency_ms = res.latency_ms
      selectedConnector.value.last_tested_at = new Date().toISOString()
      selectedConnector.value.error_message = ''
      toastStore.success('Connection Successful', `${res.message} (${res.latency_ms}ms)`)
    } else {
      selectedConnector.value.status = 'error'
      selectedConnector.value.latency_ms = res.latency_ms
      selectedConnector.value.last_tested_at = new Date().toISOString()
      selectedConnector.value.error_message = res.message
      toastStore.error('Connection Test Failed', res.message)
    }
  } catch (err: any) {
    testResult.value = {
      success: false,
      latency_ms: 0,
      message: err.message || 'Connection failed'
    }
    selectedConnector.value.status = 'error'
    selectedConnector.value.error_message = err.message || 'Connection failed'
    selectedConnector.value.last_tested_at = new Date().toISOString()
    toastStore.error('Test Failed', err.message)
  } finally {
    // In-place catalog sync
    const idx = catalog.value.findIndex(item => item.id === selectedConnector.value?.id)
    if (idx !== -1 && selectedConnector.value) {
      catalog.value[idx].status = selectedConnector.value.status
      catalog.value[idx].latency_ms = selectedConnector.value.latency_ms
      catalog.value[idx].last_tested_at = selectedConnector.value.last_tested_at
      catalog.value[idx].error_message = selectedConnector.value.error_message
    }
    isTesting.value = false
  }
}

// Real MCP Runtime Test
async function testMCP() {
  if (!selectedConnector.value || !selectedConnector.value.mcp) return
  isTestingMCP.value = true
  mcpTestResult.value = null
  try {
    const res = await api.testMCPConnector(selectedConnector.value.id, selectedConnector.value.mcp)
    mcpTestResult.value = res
    if (res.success) {
      if (selectedConnector.value.config_mode === 'mcp') {
        selectedConnector.value.status = 'connected'
        selectedConnector.value.latency_ms = res.latency_ms
        selectedConnector.value.last_tested_at = new Date().toISOString()
        selectedConnector.value.error_message = ''
      }
      toastStore.success('MCP Verified', `${res.message} (${res.latency_ms}ms)`)
    } else {
      if (selectedConnector.value.config_mode === 'mcp') {
        selectedConnector.value.status = 'error'
        selectedConnector.value.latency_ms = res.latency_ms
        selectedConnector.value.last_tested_at = new Date().toISOString()
        selectedConnector.value.error_message = res.message
      }
      toastStore.error('MCP Validation Failed', res.message)
    }
  } catch (err: any) {
    mcpTestResult.value = {
      success: false,
      latency_ms: 0,
      message: err.message || 'MCP test failed'
    }
    if (selectedConnector.value.config_mode === 'mcp') {
      selectedConnector.value.status = 'error'
      selectedConnector.value.error_message = err.message || 'MCP test failed'
      selectedConnector.value.last_tested_at = new Date().toISOString()
    }
    toastStore.error('MCP Test Failed', err.message)
  } finally {
    const idx = catalog.value.findIndex(item => item.id === selectedConnector.value?.id)
    if (idx !== -1 && selectedConnector.value) {
      catalog.value[idx].status = selectedConnector.value.status
      catalog.value[idx].latency_ms = selectedConnector.value.latency_ms
      catalog.value[idx].last_tested_at = selectedConnector.value.last_tested_at
      catalog.value[idx].error_message = selectedConnector.value.error_message
    }
    isTestingMCP.value = false
  }
}

// Environment Variables Management
function addEnvVar() {
  if (!selectedConnector.value?.mcp) return
  if (!selectedConnector.value.mcp.env) {
    selectedConnector.value.mcp.env = {}
  }
  const k = newEnvKey.value.trim()
  if (!k) return
  selectedConnector.value.mcp.env[k] = newEnvVal.value.trim()
  newEnvKey.value = ''
  newEnvVal.value = ''
}

function removeEnvVar(k: string) {
  if (!selectedConnector.value?.mcp?.env) return
  delete selectedConnector.value.mcp.env[k]
}

// Confirmation modal state
const isConfirmDeleteOpen = ref(false)
const confirmDeleteTitle = ref('Confirm Deletion')
const confirmDeleteMessage = ref('Are you sure you want to proceed?')
const confirmDeleteItemName = ref('')
const confirmDeleteNote = ref('')
const confirmDeleteBtnText = ref('Delete')
const isConfirmDeleteLoading = ref(false)
let onConfirmDeleteCallback: (() => Promise<void>) | null = null

function triggerConfirmDelete(options: {
  title: string
  message: string
  itemName: string
  note?: string
  confirmText?: string
  onConfirm: () => Promise<void>
}) {
  confirmDeleteTitle.value = options.title
  confirmDeleteMessage.value = options.message
  confirmDeleteItemName.value = options.itemName
  confirmDeleteNote.value = options.note || ''
  confirmDeleteBtnText.value = options.confirmText || 'Delete'
  onConfirmDeleteCallback = options.onConfirm
  isConfirmDeleteOpen.value = true
}

async function handleExecuteConfirmDelete() {
  if (!onConfirmDeleteCallback) return
  isConfirmDeleteLoading.value = true
  try {
    await onConfirmDeleteCallback()
    isConfirmDeleteOpen.value = false
  } catch (err: any) {
    // Handled in callback
  } finally {
    isConfirmDeleteLoading.value = false
  }
}

function promptRemoveEnvVar(k: string) {
  triggerConfirmDelete({
    title: 'Delete Environment Variable',
    message: `Are you sure you want to delete environment variable "${k}" from connector "${selectedConnector.value?.name || 'MCP'}"?`,
    itemName: k,
    note: 'The variable can be re-added at any time.',
    confirmText: 'Delete Variable',
    onConfirm: async () => {
      removeEnvVar(k)
      toastStore.info('Variable Removed', `Deleted environment variable ${k}`)
    }
  })
}

// Open Global MCP Export Modal
async function openMCPExport() {
  try {
    const data = await api.getMCPConfigExport()
    mcpExportJSON.value = JSON.stringify(data, null, 2)
    showMCPExportModal.value = true
  } catch (err: any) {
    toastStore.error('MCP Export', err.message)
  }
}

function copyMCPConfig() {
  navigator.clipboard.writeText(mcpExportJSON.value)
  copiedMCP.value = true
  setTimeout(() => { copiedMCP.value = false }, 2000)
  toastStore.success('Copied', 'MCP configuration copied to clipboard')
}

// Explorer Tab (JIRA)
async function searchExplorer() {
  isSearchingExplorer.value = true
  try {
    explorerIssues.value = await api.getJiraIssues(explorerQuery.value)
  } catch (err: any) {
    toastStore.error('Search Failed', err.message)
  } finally {
    isSearchingExplorer.value = false
  }
}

// Periodic Ping Methods
async function loadPingConfig() {
  try {
    pingConfig.value = await api.getPingConfig()
  } catch (e) {
    // default
  }
}

async function updatePingSetting(enabled: boolean, interval: number) {
  isUpdatingPing.value = true
  try {
    const updated = await api.updatePingConfig({ enabled, interval_seconds: interval })
    pingConfig.value = updated
    toastStore.success('Auto-Ping Updated', enabled ? `Pinging active connectors every ${interval}s` : 'Periodic ping paused')
    showPingSettings.value = false
  } catch (err: any) {
    toastStore.error('Ping Config Failed', err.message)
  } finally {
    isUpdatingPing.value = false
  }
}

function flashConnectorPing(id: string) {
  recentlyPingedIds.value.add(id)
  setTimeout(() => {
    recentlyPingedIds.value.delete(id)
  }, 2500)
}

async function triggerPingAll() {
  isPingingAll.value = true
  try {
    const summary = await api.pingAllConnectors()
    lastPingTime.value = new Date()
    for (const [id, res] of Object.entries(summary.results)) {
      const item = catalog.value.find(c => c.id === id)
      if (item) {
        item.status = res.success ? 'connected' : 'error'
        item.latency_ms = res.latency_ms
        item.last_tested_at = summary.timestamp
        if (!res.success) {
          item.error_message = res.message
        } else {
          item.error_message = ''
        }
        flashConnectorPing(id)
      }
    }
    toastStore.success('Ping All Completed', `Tested ${summary.total_pinged} active connectors`)
  } catch (err: any) {
    toastStore.error('Ping All Failed', err.message)
  } finally {
    isPingingAll.value = false
  }
}

function formatRelativeTime(dateStr?: string): string {
  if (!dateStr || dateStr.startsWith('0001-01-01')) return ''
  try {
    const d = new Date(dateStr)
    const diffSec = Math.floor((Date.now() - d.getTime()) / 1000)
    if (diffSec < 5) return 'just now'
    if (diffSec < 60) return `${diffSec}s ago`
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return `${diffMin}m ago`
    return `${Math.floor(diffMin / 60)}h ago`
  } catch {
    return ''
  }
}

onMounted(async () => {
  await loadCatalog()
  await loadPingConfig()

  // Real-time WebSocket listener: automatically updates cards whenever background pinger or live test completes
  unsubscribeWS = wsService.subscribe((event) => {
    if (event.type === 'connector.status') {
      const payload = event.payload as any
      if (!payload) return
      const targetId = payload.connector_id || payload.item?.id
      const item = catalog.value.find(c => c.id === targetId)
      if (item) {
        item.status = payload.status
        item.latency_ms = payload.latency_ms
        item.last_tested_at = payload.last_tested_at
        if (payload.error_message !== undefined) {
          item.error_message = payload.error_message
        }
        flashConnectorPing(targetId)
      }
      if (selectedConnector.value && selectedConnector.value.id === targetId) {
        selectedConnector.value.status = payload.status
        selectedConnector.value.latency_ms = payload.latency_ms
        selectedConnector.value.last_tested_at = payload.last_tested_at
      }
    }
  })

  // Resilient background auto-sync to ensure UI accuracy even through brief network disruptions
  backgroundSyncTimer = setInterval(async () => {
    try {
      const items = await api.getConnectorCatalog()
      for (const u of items) {
        const existing = catalog.value.find(c => c.id === u.id)
        if (existing) {
          existing.status = u.status
          existing.latency_ms = u.latency_ms
          existing.last_tested_at = u.last_tested_at
          existing.error_message = u.error_message
        }
        if (selectedConnector.value && selectedConnector.value.id === u.id) {
          selectedConnector.value.status = u.status
          selectedConnector.value.latency_ms = u.latency_ms
          selectedConnector.value.last_tested_at = u.last_tested_at
          selectedConnector.value.error_message = u.error_message
        }
      }
    } catch {
      // background silent
    }
  }, 15000)
})

onUnmounted(() => {
  if (unsubscribeWS) unsubscribeWS()
  if (backgroundSyncTimer) clearInterval(backgroundSyncTimer)
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden text-slate-100">

    <!-- ==================== VIEW 1: CATALOG OF CARDS (DEFAULT) ==================== -->
    <template v-if="currentView === 'list'">
      <!-- Catalog Header -->
      <div class="px-8 py-5 border-b border-slate-800 bg-slate-900/60 flex flex-wrap items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-blue-950/80 border border-blue-800/80 flex items-center justify-center text-blue-400 shadow-md">
            <Plug class="w-5 h-5" />
          </div>
          <div>
            <div class="flex items-center gap-2.5">
              <h1 class="text-base font-semibold text-slate-100">Third-Party Connectors</h1>
              <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-blue-950 text-blue-400 border border-blue-800/50">
                Modular Integration Hub
              </span>
              <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-purple-950 text-purple-300 border border-purple-800/50 flex items-center gap-1">
                <Cpu class="w-3 h-3" /> MCP Enabled
              </span>
            </div>
            <p class="text-xs text-slate-400 mt-0.5">
              Connect external issue trackers, documentation wikis, version control, and ChatOps via REST API and Model Context Protocol (MCP).
            </p>
          </div>
        </div>

        <!-- Ecosystem Status Badges, Live Auto-Ping, & Actions -->
        <div class="flex items-center gap-2.5">
          <div class="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 flex items-center gap-2 text-xs font-mono">
            <span class="text-slate-400">Catalog:</span>
            <span class="text-slate-200 font-semibold">{{ stats.total }} Connectors</span>
          </div>

          <div class="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 flex items-center gap-2 text-xs font-mono">
            <span class="w-2 h-2 rounded-full" :class="stats.connectedCount > 0 ? 'bg-emerald-400 animate-pulse' : 'bg-slate-600'"></span>
            <span class="text-slate-400">Live:</span>
            <span :class="stats.connectedCount > 0 ? 'text-emerald-400 font-semibold' : 'text-slate-400'">{{ stats.connectedCount }} Connected</span>
          </div>

          <!-- Live Auto-Ping Badge & Settings Popover -->
          <div class="relative">
            <button
              @click="showPingSettings = !showPingSettings"
              class="px-3 py-1.5 rounded-lg border text-xs font-medium flex items-center gap-2 transition shadow cursor-pointer"
              :class="pingConfig.enabled 
                ? 'border-emerald-800/60 bg-emerald-950/40 hover:bg-emerald-900/50 text-emerald-300' 
                : 'border-slate-800 bg-slate-900 text-slate-400 hover:bg-slate-850'"
              title="Configure periodic background ping"
            >
              <span class="relative flex h-2 w-2">
                <span v-if="pingConfig.enabled" class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span class="relative inline-flex rounded-full h-2 w-2" :class="pingConfig.enabled ? 'bg-emerald-500' : 'bg-slate-600'"></span>
              </span>
              <span class="font-mono text-[11px]">
                {{ pingConfig.enabled ? `Auto-Ping: ${pingConfig.interval_seconds}s` : 'Auto-Ping: Paused' }}
              </span>
              <ChevronDown class="w-3 h-3 opacity-60" />
            </button>

            <!-- Dropdown Menu for Ping Configuration -->
            <div 
              v-if="showPingSettings" 
              class="absolute right-0 top-full mt-2 w-64 rounded-xl bg-slate-900 border border-slate-800 shadow-2xl p-3 z-50 flex flex-col gap-3"
            >
              <div class="flex items-center justify-between pb-2 border-b border-slate-800">
                <div class="flex items-center gap-1.5 text-xs font-semibold text-slate-200">
                  <Activity class="w-3.5 h-3.5 text-emerald-400" />
                  <span>Periodic Live Ping</span>
                </div>
                <button
                  @click="updatePingSetting(!pingConfig.enabled, pingConfig.interval_seconds)"
                  :disabled="isUpdatingPing"
                  class="px-2 py-0.5 rounded text-[10px] font-medium transition cursor-pointer"
                  :class="pingConfig.enabled ? 'bg-rose-950 text-rose-300 hover:bg-rose-900 border border-rose-800/60' : 'bg-emerald-950 text-emerald-300 hover:bg-emerald-900 border border-emerald-800/60'"
                >
                  {{ pingConfig.enabled ? 'Pause' : 'Enable' }}
                </button>
              </div>

              <div>
                <span class="text-[10px] uppercase font-mono text-slate-400 block mb-1.5">Ping Interval</span>
                <div class="grid grid-cols-4 gap-1">
                  <button
                    v-for="sec in [10, 30, 60, 120]"
                    :key="sec"
                    @click="updatePingSetting(true, sec)"
                    :disabled="isUpdatingPing"
                    class="py-1 px-1.5 rounded text-[11px] font-mono border transition text-center cursor-pointer"
                    :class="pingConfig.interval_seconds === sec && pingConfig.enabled
                      ? 'bg-emerald-500/20 border-emerald-500 text-emerald-300 font-semibold'
                      : 'bg-slate-950 border-slate-800 text-slate-400 hover:bg-slate-850 hover:text-slate-200'"
                  >
                    {{ sec }}s
                  </button>
                </div>
              </div>

              <p class="text-[10px] text-slate-500 leading-normal">
                Autonomous background daemon pings all enabled REST & MCP services to keep statuses and latency real-time without user action.
              </p>
            </div>
          </div>

          <!-- Ping All Now Button -->
          <button
            @click="triggerPingAll"
            :disabled="isPingingAll"
            class="px-3 py-1.5 rounded-lg border border-sky-800/60 bg-sky-950/40 hover:bg-sky-900/60 text-sky-300 hover:text-white text-xs font-medium flex items-center gap-1.5 transition shadow disabled:opacity-50 cursor-pointer"
            title="Immediately trigger live connection probe across all active connectors"
          >
            <Activity class="w-3.5 h-3.5" :class="{ 'animate-spin': isPingingAll }" />
            <span>{{ isPingingAll ? 'Pinging All...' : 'Ping All' }}</span>
          </button>

          <!-- MCP Config Export Button -->
          <button
            @click="openMCPExport"
            class="px-3 py-1.5 rounded-lg border border-purple-800/60 bg-purple-950/40 hover:bg-purple-900/60 text-purple-300 hover:text-white text-xs font-medium flex items-center gap-1.5 transition shadow cursor-pointer"
            title="Export standard mcp_config.json configuration"
          >
            <Code class="w-3.5 h-3.5" />
            <span>Export MCP</span>
          </button>

          <button
            @click="loadCatalog"
            :disabled="isLoading"
            class="p-2 rounded-lg border border-slate-800 bg-slate-900 hover:bg-slate-850 text-slate-400 hover:text-white transition cursor-pointer"
            title="Refresh Catalog"
          >
            <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': isLoading }" />
          </button>
        </div>
      </div>

      <!-- Filters & Category Tabs -->
      <div class="px-8 border-b border-slate-800 bg-slate-900/30 flex flex-wrap items-center justify-between gap-4 py-2.5">
        <!-- Category Filter Pills -->
        <div class="flex items-center gap-2 overflow-x-auto py-1 scrollbar-none">
          <button
            v-for="cat in categoryTabs"
            :key="cat.key"
            @click="activeCategory = (cat.key as any)"
            class="px-3 py-1.5 rounded-lg text-xs font-medium transition-all flex items-center gap-1.5 whitespace-nowrap"
            :class="activeCategory === cat.key
              ? 'bg-blue-600 text-white shadow-md shadow-blue-950 font-semibold'
              : 'bg-slate-900 border border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700'"
          >
            <span>{{ cat.label }}</span>
            <span 
              class="px-1.5 py-0.2 rounded-full text-[10px] font-mono"
              :class="activeCategory === cat.key ? 'bg-blue-700 text-white' : 'bg-slate-800 text-slate-400'"
            >
              {{ getCategoryCount(cat.key) }}
            </span>
          </button>
        </div>

        <!-- Search Bar -->
        <div class="relative w-full sm:w-72">
          <Search class="w-3.5 h-3.5 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Filter connectors..."
            class="w-full pl-9 pr-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-200 text-xs font-mono placeholder-slate-500 focus:outline-none focus:border-blue-500 transition"
          />
        </div>
      </div>

      <!-- Cards Grid Area -->
      <div class="flex-1 overflow-y-auto p-8">
        <div class="max-w-7xl mx-auto space-y-6">
          
          <!-- Loading State -->
          <div v-if="isLoading" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
            <div 
              v-for="i in 8" 
              :key="i"
              class="p-5 rounded-2xl border border-slate-800 bg-slate-900/40 h-56 animate-pulse flex flex-col justify-between"
            >
              <div class="space-y-3">
                <div class="flex items-center justify-between">
                  <div class="w-10 h-10 rounded-xl bg-slate-800"></div>
                  <div class="w-12 h-6 rounded-full bg-slate-800"></div>
                </div>
                <div class="w-32 h-4 rounded bg-slate-800"></div>
                <div class="w-full h-8 rounded bg-slate-800/60"></div>
              </div>
              <div class="w-24 h-4 rounded bg-slate-800"></div>
            </div>
          </div>

          <!-- Empty Search State -->
          <div 
            v-else-if="filteredCatalog.length === 0" 
            class="py-16 px-4 rounded-2xl border border-slate-800 bg-slate-900/40 text-center space-y-3"
          >
            <Plug class="w-10 h-10 text-slate-500 mx-auto" />
            <h3 class="text-sm font-bold text-white">No Connectors Found</h3>
            <p class="text-xs text-slate-400 max-w-sm mx-auto">
              {{ searchQuery ? `No connectors match "${searchQuery}".` : 'No connectors available in this category.' }}
            </p>
            <button
              v-if="searchQuery"
              @click="searchQuery = ''; activeCategory = 'all'"
              class="px-3.5 py-1.5 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-semibold text-white inline-flex items-center gap-1.5 transition shadow"
            >
              Clear Filters
            </button>
          </div>

          <!-- Connector Cards Grid -->
          <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
            <div
              v-for="c in filteredCatalog"
              :key="c.id"
              @click="openConnectorSetup(c)"
              class="p-5 rounded-2xl border bg-slate-900/40 hover:bg-slate-900/80 transition-all duration-300 cursor-pointer flex flex-col justify-between group relative overflow-hidden shadow-lg hover:shadow-2xl"
              :class="[
                recentlyPingedIds.has(c.id) 
                  ? 'border-emerald-500/80 ring-2 ring-emerald-400/80 shadow-[0_0_20px_rgba(52,211,153,0.35)]' 
                  : 'border-slate-800',
                getBrandColor(c).accentBorder,
                c.enabled ? 'ring-1 ring-emerald-500/20' : 'opacity-90'
              ]"
            >
              <!-- Card Top Ambient Gradient -->
              <div 
                class="absolute -top-12 -left-12 w-32 h-32 rounded-full blur-2xl pointer-events-none opacity-30 transition-opacity group-hover:opacity-60 bg-gradient-to-br"
                :class="getBrandColor(c).gradient"
              ></div>

              <!-- Top Row: Icon + Badges + Direct Toggle Switch -->
              <div class="space-y-3.5 relative z-10">
                <div class="flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <!-- Icon Container -->
                    <div 
                      class="w-10 h-10 rounded-xl border flex items-center justify-center shadow-md transition-transform group-hover:scale-105"
                      :class="[getBrandColor(c).bg, getBrandColor(c).border, getBrandColor(c).text]"
                    >
                      <CheckSquare v-if="c.id === 'jira'" class="w-5 h-5" />
                      <BookOpen v-else-if="c.id === 'confluence'" class="w-5 h-5" />
                      <GitBranch v-else-if="c.id === 'github' || c.id === 'gitlab' || c.id === 'bitbucket'" class="w-5 h-5" />
                      <MessageSquare v-else-if="c.id === 'slack'" class="w-5 h-5" />
                      <Layers v-else-if="c.id === 'linear'" class="w-5 h-5" />
                      <FileText v-else-if="c.id === 'notion'" class="w-5 h-5" />
                      <Zap v-else class="w-5 h-5" />
                    </div>

                    <div class="flex flex-col gap-1">
                      <span 
                        class="px-2 py-0.5 rounded text-[10px] font-mono border w-fit"
                        :class="getBrandColor(c).badgeBg"
                      >
                        {{ c.category_label || c.category }}
                      </span>
                    </div>
                  </div>

                  <!-- DIRECT TOGGLE SWITCH (Stops Propagation) -->
                  <div 
                    @click.stop="handleToggle(c, $event)"
                    class="flex items-center gap-1.5 p-1 -m-1 rounded-lg hover:bg-slate-800/60 transition"
                    :title="c.enabled ? 'Click to disable' : 'Click to enable'"
                  >
                    <div 
                      class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                      :class="c.enabled ? 'bg-emerald-500 shadow-sm shadow-emerald-950' : 'bg-slate-700'"
                    >
                      <span 
                        class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                        :class="c.enabled ? 'translate-x-4' : 'translate-x-0'"
                      >
                        <RefreshCw 
                          v-if="togglingIds.has(c.id)" 
                          class="w-3 h-3 text-slate-600 animate-spin m-0.5" 
                        />
                      </span>
                    </div>
                  </div>
                </div>

                <!-- Connector Info -->
                <div>
                  <h3 class="font-bold text-sm text-white group-hover:text-blue-400 transition flex items-center justify-between">
                    <span>{{ c.name }}</span>
                    <!-- MCP Protocol Badge -->
                    <span 
                      v-if="c.mcp" 
                      class="px-1.5 py-0.2 rounded text-[9px] font-mono bg-purple-950/80 text-purple-300 border border-purple-800/50 flex items-center gap-0.5"
                      title="Supports Model Context Protocol"
                    >
                      <Cpu class="w-2.5 h-2.5" /> MCP
                    </span>
                  </h3>
                  <p class="text-xs text-slate-400 line-clamp-2 leading-relaxed mt-1">
                    {{ c.description }}
                  </p>
                </div>

                <!-- Capabilities Badges -->
                <div v-if="c.capabilities && c.capabilities.length > 0" class="flex flex-wrap items-center gap-1.5 pt-1">
                  <span
                    v-for="(cap, idx) in c.capabilities.slice(0, 3)"
                    :key="idx"
                    class="px-2 py-0.5 rounded text-[10px] font-mono bg-slate-950/70 text-slate-300 border border-slate-800"
                  >
                    {{ cap }}
                  </span>
                </div>
              </div>

              <!-- Card Bottom: Real Status, Live Ping, & Setup Prompt -->
              <div class="pt-4 mt-4 border-t border-slate-800/80 flex items-center justify-between text-xs relative z-10">
                <!-- Status Badge (REAL INFORMATION ONLY) -->
                <div class="flex items-center gap-2 font-mono text-[11px]">
                  <template v-if="!c.enabled">
                    <span class="w-2 h-2 rounded-full bg-slate-600"></span>
                    <span class="text-slate-500 font-semibold uppercase">Disabled</span>
                  </template>
                  <template v-else-if="c.status === 'connected'">
                    <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
                    <span class="text-emerald-400 font-semibold uppercase">Connected</span>
                    <span v-if="c.latency_ms && c.latency_ms > 0" class="text-[10px] text-slate-400">({{ c.latency_ms }}ms)</span>
                  </template>
                  <template v-else-if="c.status === 'configured'">
                    <span class="w-2 h-2 rounded-full bg-sky-400"></span>
                    <span class="text-sky-400 font-semibold uppercase">Configured</span>
                  </template>
                  <template v-else-if="c.status === 'error'">
                    <span class="w-2 h-2 rounded-full bg-rose-400"></span>
                    <span class="text-rose-400 font-semibold uppercase">Error</span>
                    <span v-if="c.latency_ms && c.latency_ms > 0" class="text-[10px] text-rose-400/80">({{ c.latency_ms }}ms)</span>
                  </template>
                  <template v-else>
                    <span class="w-2 h-2 rounded-full bg-slate-500"></span>
                    <span class="text-slate-400 font-semibold uppercase">Unconfigured</span>
                  </template>

                  <!-- Live Ping Flash Badge -->
                  <span 
                    v-if="recentlyPingedIds.has(c.id)"
                    class="px-1.5 py-0.5 rounded text-[9px] font-mono bg-emerald-950 text-emerald-300 border border-emerald-700/60 flex items-center gap-1 animate-pulse"
                  >
                    <Wifi class="w-2.5 h-2.5" /> Pinged
                  </span>
                </div>

                <!-- Last tested timestamp & Setup CTA -->
                <div class="flex items-center gap-2">
                  <span 
                    v-if="c.last_tested_at && formatRelativeTime(c.last_tested_at)" 
                    class="text-[10px] text-slate-500 font-mono hidden sm:inline"
                    :title="`Last tested: ${new Date(c.last_tested_at).toLocaleString()}`"
                  >
                    {{ formatRelativeTime(c.last_tested_at) }}
                  </span>
                  <div class="flex items-center gap-1 text-[11px] font-medium text-slate-400 group-hover:text-blue-400 transition">
                    <span>Setup</span>
                    <ArrowRight class="w-3.5 h-3.5 transform group-hover:translate-x-0.5 transition-transform" />
                  </div>
                </div>
              </div>

            </div>
          </div>

        </div>
      </div>
    </template>

    <!-- ==================== VIEW 2: SETUP / EDIT MODE ==================== -->
    <template v-else-if="currentView === 'edit' && selectedConnector">
      <!-- Breadcrumb Bar & Header -->
      <div class="px-8 py-4 border-b border-slate-800 bg-slate-900/60 flex flex-wrap items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <button
            @click="returnToCatalog"
            class="px-3 py-1.5 rounded-lg border border-slate-800 bg-slate-900 hover:bg-slate-800 text-slate-300 hover:text-white text-xs font-medium flex items-center gap-1.5 transition"
          >
            <ArrowLeft class="w-3.5 h-3.5" />
            <span>All Connectors</span>
          </button>

          <div class="h-4 w-px bg-slate-800"></div>

          <div class="flex items-center gap-2.5">
            <div 
              class="w-7 h-7 rounded-lg border flex items-center justify-center text-xs"
              :class="[getBrandColor(selectedConnector).bg, getBrandColor(selectedConnector).border, getBrandColor(selectedConnector).text]"
            >
              <CheckSquare v-if="selectedConnector.id === 'jira'" class="w-4 h-4" />
              <BookOpen v-else-if="selectedConnector.id === 'confluence'" class="w-4 h-4" />
              <GitBranch v-else-if="selectedConnector.id === 'github' || selectedConnector.id === 'gitlab' || selectedConnector.id === 'bitbucket'" class="w-4 h-4" />
              <MessageSquare v-else-if="selectedConnector.id === 'slack'" class="w-4 h-4" />
              <Layers v-else-if="selectedConnector.id === 'linear'" class="w-4 h-4" />
              <FileText v-else-if="selectedConnector.id === 'notion'" class="w-4 h-4" />
              <Zap v-else class="w-4 h-4" />
            </div>
            <div>
              <h2 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
                {{ selectedConnector.name }}
                <span 
                  class="px-2 py-0.5 rounded text-[10px] font-mono border"
                  :class="getBrandColor(selectedConnector).badgeBg"
                >
                  {{ selectedConnector.category_label }}
                </span>
                <!-- Live Status Badge -->
                <span 
                  class="px-2 py-0.5 rounded-full text-[10px] font-mono uppercase font-bold tracking-wider inline-flex items-center gap-1.5 border"
                  :class="[
                    !selectedConnector.enabled ? 'bg-slate-900 border-slate-700 text-slate-400' :
                    selectedConnector.status === 'connected' ? 'bg-emerald-950/80 border-emerald-500/50 text-emerald-400' :
                    selectedConnector.status === 'error' ? 'bg-rose-950/80 border-rose-500/50 text-rose-400' :
                    selectedConnector.status === 'configured' ? 'bg-sky-950/80 border-sky-500/50 text-sky-400' :
                    'bg-slate-900 border-slate-700 text-slate-400'
                  ]"
                >
                  <span 
                    class="w-1.5 h-1.5 rounded-full"
                    :class="[
                      !selectedConnector.enabled ? 'bg-slate-500' :
                      selectedConnector.status === 'connected' ? 'bg-emerald-400 animate-pulse' :
                      selectedConnector.status === 'error' ? 'bg-rose-400' :
                      selectedConnector.status === 'configured' ? 'bg-sky-400' :
                      'bg-slate-500'
                    ]"
                  />
                  {{ !selectedConnector.enabled ? 'DISABLED' : (selectedConnector.status || 'UNCONFIGURED') }}
                </span>
              </h2>
            </div>
          </div>
        </div>

        <!-- Quick Top Actions: Enable Switch, Test Connection, Save -->
        <div class="flex items-center gap-3">
          <!-- Live Master Enable Toggle -->
          <div 
            @click="selectedConnector.enabled = !selectedConnector.enabled"
            class="flex items-center gap-2.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 cursor-pointer text-xs hover:border-slate-700 transition select-none"
            :title="selectedConnector.enabled ? 'Connector is active. Click to disable.' : 'Connector is disabled. Click to enable.'"
          >
            <span class="text-slate-400 font-mono text-[11px]">Enabled:</span>
            <div 
              class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
              :class="selectedConnector.enabled ? 'bg-emerald-500 shadow-sm shadow-emerald-950' : 'bg-slate-700'"
            >
              <span 
                class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                :class="selectedConnector.enabled ? 'translate-x-4' : 'translate-x-0'"
              />
            </div>
            <span :class="selectedConnector.enabled ? 'text-emerald-400 font-bold' : 'text-slate-500'">
              {{ selectedConnector.enabled ? 'ON' : 'OFF' }}
            </span>
          </div>

          <!-- Test Connection Button -->
          <button
            @click="testConnection"
            :disabled="isTesting"
            type="button"
            class="h-8 px-3 rounded-lg bg-slate-900 hover:bg-slate-850 border border-slate-750 text-xs font-medium text-slate-200 flex items-center gap-1.5 transition"
            title="Perform real live HTTP probe against target endpoint"
          >
            <Wifi class="w-3.5 h-3.5 text-blue-400" :class="{ 'animate-pulse text-emerald-400': isTesting }" />
            <span>{{ isTesting ? 'Pinging Real Host...' : 'Ping REST' }}</span>
          </button>

          <!-- Test MCP Button -->
          <button
            v-if="selectedConnector.mcp"
            @click="testMCP"
            :disabled="isTestingMCP"
            type="button"
            class="h-8 px-3 rounded-lg bg-purple-950/60 hover:bg-purple-900/60 border border-purple-800/60 text-xs font-medium text-purple-300 flex items-center gap-1.5 transition"
            title="Verify MCP server executable in local machine PATH"
          >
            <Cpu class="w-3.5 h-3.5 text-purple-400" :class="{ 'animate-spin': isTestingMCP }" />
            <span>{{ isTestingMCP ? 'Checking MCP...' : 'Test MCP' }}</span>
          </button>

          <!-- Save Button -->
          <button
            @click="saveConnector"
            :disabled="isSaving"
            type="button"
            class="h-8 px-4 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-semibold text-white flex items-center gap-1.5 transition shadow-md shadow-blue-950"
          >
            <Save class="w-3.5 h-3.5" :class="{ 'animate-spin': isSaving }" />
            <span>{{ isSaving ? 'Saving...' : 'Save Settings' }}</span>
          </button>
        </div>
      </div>

      <!-- Navigation Tabs: REST API vs MCP Server vs Backlog Explorer -->
      <div class="px-8 border-b border-slate-800 bg-slate-900/30 flex items-center gap-6">
        <button
          @click="activeEditTab = 'rest'"
          class="py-3 text-xs font-medium border-b-2 transition-colors flex items-center gap-2"
          :class="activeEditTab === 'rest' ? 'border-blue-500 text-blue-400 font-semibold' : 'border-transparent text-slate-400 hover:text-slate-200'"
        >
          <Settings class="w-3.5 h-3.5" />
          <span>REST API & Credentials</span>
        </button>

        <button
          v-if="selectedConnector.mcp"
          @click="activeEditTab = 'mcp'"
          class="py-3 text-xs font-medium border-b-2 transition-colors flex items-center gap-2"
          :class="activeEditTab === 'mcp' ? 'border-purple-500 text-purple-400 font-semibold' : 'border-transparent text-slate-400 hover:text-slate-200'"
        >
          <Cpu class="w-3.5 h-3.5 text-purple-400" />
          <span>Model Context Protocol (MCP) Config</span>
          <span 
            class="px-1.5 py-0.2 rounded text-[10px] font-mono"
            :class="selectedConnector.mcp.enabled ? 'bg-purple-900 text-purple-200' : 'bg-slate-800 text-slate-400'"
          >
            {{ selectedConnector.mcp.enabled ? 'ACTIVE' : 'READY' }}
          </span>
        </button>

        <button
          v-if="selectedConnector.id === 'jira'"
          @click="activeEditTab = 'explorer'"
          class="py-3 text-xs font-medium border-b-2 transition-colors flex items-center gap-2"
          :class="activeEditTab === 'explorer' ? 'border-emerald-500 text-emerald-400 font-semibold' : 'border-transparent text-slate-400 hover:text-slate-200'"
        >
          <Download class="w-3.5 h-3.5 text-emerald-400" />
          <span>JIRA Backlog Explorer & Importer</span>
        </button>
      </div>

      <!-- Main Edit View Scroll Area -->
      <div class="flex-1 overflow-y-auto p-8">
        <div class="max-w-4xl mx-auto space-y-6">

          <!-- Real REST Diagnostic Result Banner -->
          <div
            v-if="testResult"
            class="p-4 rounded-xl border text-xs font-mono flex items-start gap-3 transition-all animate-fade-in"
            :class="testResult.success ? 'bg-emerald-950/40 border-emerald-800/70 text-emerald-300' : 'bg-rose-950/40 border-rose-800/70 text-rose-300'"
          >
            <CheckCircle2 v-if="testResult.success" class="w-4 h-4 text-emerald-400 flex-shrink-0 mt-0.5" />
            <AlertCircle v-else class="w-4 h-4 text-rose-400 flex-shrink-0 mt-0.5" />
            <div class="space-y-1 flex-1">
              <div class="font-semibold">{{ testResult.message }}</div>
              <div class="text-[11px] text-slate-400 flex flex-wrap gap-x-4">
                <span>Real Latency: <strong class="text-slate-200">{{ testResult.latency_ms }}ms</strong></span>
                <span v-if="testResult.connected_as">Authenticated User: <strong class="text-slate-200">{{ testResult.connected_as }}</strong></span>
                <span v-if="testResult.server_info">Host/Server Info: <strong class="text-slate-200">{{ testResult.server_info }}</strong></span>
                <span v-if="testResult.target_entity">Target: <strong class="text-slate-200">{{ testResult.target_entity }}</strong></span>
              </div>
            </div>
          </div>

          <!-- Real MCP Diagnostic Result Banner -->
          <div
            v-if="mcpTestResult"
            class="p-4 rounded-xl border text-xs font-mono flex items-start gap-3 transition-all animate-fade-in"
            :class="mcpTestResult.success ? 'bg-purple-950/40 border-purple-800/70 text-purple-300' : 'bg-rose-950/40 border-rose-800/70 text-rose-300'"
          >
            <Cpu v-if="mcpTestResult.success" class="w-4 h-4 text-purple-400 flex-shrink-0 mt-0.5" />
            <AlertCircle v-else class="w-4 h-4 text-rose-400 flex-shrink-0 mt-0.5" />
            <div class="space-y-1 flex-1">
              <div class="font-semibold">{{ mcpTestResult.message }}</div>
              <div class="text-[11px] text-slate-400 flex flex-wrap gap-x-4">
                <span>Check Time: <strong class="text-slate-200">{{ mcpTestResult.latency_ms }}ms</strong></span>
                <span v-if="mcpTestResult.connected_as">Host Binary: <strong class="text-slate-200">{{ mcpTestResult.connected_as }}</strong></span>
                <span v-if="mcpTestResult.server_info">Command: <strong class="text-slate-200">{{ mcpTestResult.server_info }}</strong></span>
                <span v-if="mcpTestResult.target_entity">Details: <strong class="text-slate-200">{{ mcpTestResult.target_entity }}</strong></span>
              </div>
            </div>
          </div>

          <!-- ================= TAB 1: REST API & CREDENTIALS ================= -->
          <div v-if="activeEditTab === 'rest'" class="space-y-6">
            <!-- Form Card -->
            <div class="p-6 rounded-2xl bg-slate-900 border border-slate-800 space-y-6">
              
              <!-- Section 1: Endpoints & Credentials -->
              <div class="space-y-4">
                <div class="flex items-center justify-between border-b border-slate-800 pb-3">
                  <div>
                    <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
                      Authentication & Live Endpoints
                    </h3>
                    <p class="text-xs text-slate-400 mt-0.5">
                      Configure base URLs and service account authentication credentials.
                    </p>
                  </div>
                  <div class="text-[11px] font-mono text-slate-500">
                    ID: <code class="text-blue-400">{{ selectedConnector.id }}</code>
                  </div>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <!-- Base URL -->
                  <div class="md:col-span-2">
                    <label class="block text-xs font-medium text-slate-300 mb-1.5">
                      Base Endpoint / Host URL
                    </label>
                    <input
                      v-model="selectedConnector.base_url"
                      type="text"
                      :placeholder="selectedConnector.id === 'bitbucket' ? 'https://api.bitbucket.org/2.0' : selectedConnector.id === 'slack' ? 'https://hooks.slack.com/services/...' : 'https://api.example.com'"
                      class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                    />
                  </div>

                  <!-- Username / Email -->
                  <div>
                    <label class="block text-xs font-medium text-slate-300 mb-1.5">
                      Username / Email / Account ID
                    </label>
                    <input
                      v-model="selectedConnector.username"
                      type="text"
                      placeholder="devops@company.com or service-account"
                      class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                    />
                  </div>

                  <!-- API Token / PAT / Secret -->
                  <div>
                    <label class="block text-xs font-medium text-slate-300 mb-1.5">
                      API Token / Personal Access Token / App Password
                    </label>
                    <div class="relative">
                      <input
                        v-model="selectedConnector.api_token"
                        :type="showToken ? 'text' : 'password'"
                        placeholder="••••••••••••••••••••••••"
                        class="w-full h-9 pl-3 pr-9 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                      />
                      <button
                        @click="showToken = !showToken"
                        type="button"
                        class="absolute right-2.5 top-2.5 text-slate-500 hover:text-slate-300"
                      >
                        <EyeOff v-if="showToken" class="w-4 h-4" />
                        <Eye v-else class="w-4 h-4" />
                      </button>
                    </div>
                  </div>

                  <!-- Target Entity -->
                  <div class="md:col-span-2">
                    <label class="block text-xs font-medium text-slate-300 mb-1.5">
                      {{ selectedConnector.target_label || 'Default Target Entity (e.g. Space, Channel, Project)' }}
                    </label>
                    <input
                      v-model="selectedConnector.target_entity"
                      type="text"
                      :placeholder="selectedConnector.id === 'bitbucket' ? 'workspace-name/repo-slug' : selectedConnector.id === 'slack' ? '#alerts' : 'Target Key'"
                      class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                    />
                  </div>
                </div>
              </div>

              <!-- Section 2: Automation Rules & Custom Toggles -->
              <div class="space-y-4 pt-4 border-t border-slate-800">
                <div class="border-b border-slate-800 pb-3">
                  <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
                    Automation Features & Integration Hooks
                  </h3>
                  <p class="text-xs text-slate-400 mt-0.5">
                    Tailored behavior and synchronization settings for {{ selectedConnector.name }}.
                  </p>
                </div>

                <!-- JIRA SPECIFIC TOGGLES -->
                <template v-if="selectedConnector.id === 'jira'">
                  <div class="space-y-3">
                    <div 
                      @click="selectedConnector.extra_settings!.auto_detect_keys = !selectedConnector.extra_settings!.auto_detect_keys"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Automatically Detect JIRA Keys on Kanban Cards</div>
                        <div class="text-[11px] text-slate-400">Scans task titles and descriptions for ticket keys (e.g. [PAY-1042]) and adds deep-link badges.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.auto_detect_keys ? 'bg-emerald-500 shadow-sm shadow-emerald-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.auto_detect_keys ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>

                    <div 
                      @click="selectedConnector.extra_settings!.auto_sync_status = !selectedConnector.extra_settings!.auto_sync_status"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Auto-Sync Stage Transitions to JIRA Status</div>
                        <div class="text-[11px] text-slate-400">Advances JIRA issue workflow as pipeline stages complete.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.auto_sync_status ? 'bg-emerald-500 shadow-sm shadow-emerald-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.auto_sync_status ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>

                    <div>
                      <label class="block text-xs font-medium text-slate-300 mb-1.5">Default JQL Query Filter</label>
                      <input
                        v-model="selectedConnector.extra_settings!.jql_filter"
                        type="text"
                        placeholder="project = PAY AND status != Done ORDER BY created DESC"
                        class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                      />
                    </div>
                  </div>
                </template>

                <!-- CONFLUENCE SPECIFIC TOGGLES -->
                <template v-else-if="selectedConnector.id === 'confluence'">
                  <div class="space-y-3">
                    <div 
                      @click="selectedConnector.extra_settings!.auto_publish_tech_docs = !selectedConnector.extra_settings!.auto_publish_tech_docs"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Automatically Publish Tech Doc RFCs</div>
                        <div class="text-[11px] text-slate-400">Pushes Technical Architecture Design documents (TECH_DOC_RFC.md) to Confluence upon reaching the RFC design stage.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.auto_publish_tech_docs ? 'bg-emerald-500 shadow-sm shadow-emerald-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.auto_publish_tech_docs ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>

                    <div 
                      @click="selectedConnector.extra_settings!.auto_publish_prd = !selectedConnector.extra_settings!.auto_publish_prd"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Automatically Publish PRD Discovery Documents</div>
                        <div class="text-[11px] text-slate-400">Exports synthesized product requirement specifications into Confluence team spaces.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.auto_publish_prd ? 'bg-emerald-500 shadow-sm shadow-emerald-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.auto_publish_prd ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>
                  </div>
                </template>

                <!-- BITBUCKET SPECIFIC TOGGLES -->
                <template v-else-if="selectedConnector.id === 'bitbucket'">
                  <div class="space-y-3">
                    <div 
                      @click="selectedConnector.extra_settings!.auto_sync_prs = !selectedConnector.extra_settings!.auto_sync_prs"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Auto-Create Bitbucket Pull Request upon Code Generation</div>
                        <div class="text-[11px] text-slate-400">Generates pull request with diff summary and ATDD checklist directly into Bitbucket repo.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.auto_sync_prs ? 'bg-cyan-500 shadow-sm shadow-cyan-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.auto_sync_prs ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>

                    <div 
                      @click="selectedConnector.extra_settings!.sync_pipelines = !selectedConnector.extra_settings!.sync_pipelines"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Verify Bitbucket Pipelines Status Before Marking ATDD Done</div>
                        <div class="text-[11px] text-slate-400">Checks commit status build results from Bitbucket Pipelines CI/CD.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.sync_pipelines ? 'bg-cyan-500 shadow-sm shadow-cyan-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.sync_pipelines ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>
                  </div>
                </template>

                <!-- SLACK SPECIFIC TOGGLES -->
                <template v-else-if="selectedConnector.id === 'slack'">
                  <div class="space-y-3">
                    <div 
                      @click="selectedConnector.extra_settings!.post_channel_notifications = !selectedConnector.extra_settings!.post_channel_notifications"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Real-Time Human-In-The-Loop (HITL) Gate Alerts</div>
                        <div class="text-[11px] text-slate-400">Sends instant notifications to the channel when a stage requires human review or gate confirmation.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.post_channel_notifications ? 'bg-purple-600 shadow-sm shadow-purple-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.post_channel_notifications ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>
                  </div>
                </template>

                <!-- GITHUB / GITLAB TOGGLES -->
                <template v-else-if="selectedConnector.id === 'github' || selectedConnector.id === 'gitlab'">
                  <div class="space-y-3">
                    <div 
                      @click="selectedConnector.extra_settings!.sync_pull_requests = !selectedConnector.extra_settings!.sync_pull_requests"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Auto-Create Pull/Merge Request on Code Signoff</div>
                        <div class="text-[11px] text-slate-400">Automatically creates an upstream branch and draft Pull Request once code generation completes.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.sync_pull_requests ? 'bg-blue-600 shadow-sm shadow-blue-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.sync_pull_requests ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>
                  </div>
                </template>

                <!-- GENERIC / WEBHOOK TOGGLES -->
                <template v-else>
                  <div class="space-y-3">
                    <div 
                      @click="selectedConnector.extra_settings!.sign_payloads = !selectedConnector.extra_settings!.sign_payloads"
                      class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
                    >
                      <div>
                        <div class="text-xs font-medium text-slate-200">Sign Payloads with HMAC SHA-256 Header (X-Hub-Signature-256)</div>
                        <div class="text-[11px] text-slate-400">Ensures message integrity and security for receiver endpoints.</div>
                      </div>
                      <div 
                        class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                        :class="selectedConnector.extra_settings?.sign_payloads ? 'bg-emerald-500 shadow-sm shadow-emerald-950' : 'bg-slate-700'"
                      >
                        <span 
                          class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                          :class="selectedConnector.extra_settings?.sign_payloads ? 'translate-x-4' : 'translate-x-0'"
                        />
                      </div>
                    </div>
                  </div>
                </template>
              </div>

              <!-- Save Actions Bar -->
              <div class="pt-4 border-t border-slate-800 flex items-center justify-between">
                <button
                  @click="testConnection"
                  :disabled="isTesting"
                  type="button"
                  class="h-9 px-4 rounded-lg bg-slate-800 hover:bg-slate-750 text-xs font-medium text-slate-200 flex items-center gap-1.5 transition"
                >
                  <Wifi class="w-3.5 h-3.5 text-blue-400" />
                  <span>{{ isTesting ? 'Testing Real Host...' : 'Ping REST Endpoint' }}</span>
                </button>

                <button
                  @click="saveConnector"
                  :disabled="isSaving"
                  type="button"
                  class="h-9 px-6 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-semibold text-white flex items-center gap-1.5 transition shadow-md shadow-blue-950"
                >
                  <Save class="w-3.5 h-3.5" />
                  <span>{{ isSaving ? 'Saving...' : 'Save Configuration' }}</span>
                </button>
              </div>

            </div>
          </div>

          <!-- ================= TAB 2: MODEL CONTEXT PROTOCOL (MCP) CONFIG ================= -->
          <div v-else-if="activeEditTab === 'mcp' && selectedConnector.mcp" class="space-y-6">
            <div class="p-6 rounded-2xl bg-slate-900 border border-purple-900/40 space-y-6">
              
              <!-- Explainer Card -->
              <div class="p-4 rounded-xl bg-purple-950/30 border border-purple-800/40 text-xs space-y-1.5">
                <div class="flex items-center gap-2 text-purple-300 font-semibold text-sm">
                  <Cpu class="w-4 h-4" />
                  <span>Model Context Protocol (MCP) Integration</span>
                </div>
                <p class="text-slate-300 text-xs leading-relaxed">
                  Enables Meta-Orchestrator and AI agents to invoke {{ selectedConnector.name }} tools directly using standard Model Context Protocol servers.
                  Compatible with Claude Desktop, Antigravity, Cursor, and CLI agent drivers.
                </p>
              </div>

              <!-- MCP Enable Toggle -->
              <div 
                @click="selectedConnector.mcp.enabled = !selectedConnector.mcp.enabled"
                class="flex items-center justify-between p-3.5 rounded-xl bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 transition select-none"
              >
                <div>
                  <div class="text-xs font-medium text-slate-200">Enable MCP Server for {{ selectedConnector.name }}</div>
                  <div class="text-[11px] text-slate-400">Includes this connector in generated MCP server manifests and agent toolsets.</div>
                </div>
                <div 
                  class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
                  :class="selectedConnector.mcp?.enabled ? 'bg-purple-600 shadow-sm shadow-purple-950' : 'bg-slate-700'"
                >
                  <span 
                    class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out"
                    :class="selectedConnector.mcp?.enabled ? 'translate-x-4' : 'translate-x-0'"
                  />
                </div>
              </div>

              <!-- MCP Server Configuration -->
              <div class="space-y-4">
                <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <!-- Command -->
                  <div>
                    <label class="block text-xs font-medium text-slate-300 mb-1.5">
                      MCP Server Command / Executable
                    </label>
                    <div class="relative">
                      <Terminal class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
                      <input
                        v-model="selectedConnector.mcp.command"
                        type="text"
                        placeholder="npx, uvx, docker, or binary path"
                        class="w-full h-9 pl-9 pr-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-purple-500"
                      />
                    </div>
                    <span class="text-[10px] text-slate-500 mt-1 block">Usually <code class="text-slate-400 font-mono">npx</code> for Node packages or <code class="text-slate-400 font-mono">uvx</code> for Python packages.</span>
                  </div>

                  <!-- Transport -->
                  <div>
                    <label class="block text-xs font-medium text-slate-300 mb-1.5">
                      Transport Type
                    </label>
                    <select
                      v-model="selectedConnector.mcp.transport"
                      class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-purple-500"
                    >
                      <option value="stdio">stdio (Standard I/O Subprocess)</option>
                      <option value="sse">sse (Server-Sent Events HTTP)</option>
                      <option value="streamable_http">streamable_http (HTTP Streaming)</option>
                    </select>
                  </div>

                  <!-- Arguments -->
                  <div class="md:col-span-2">
                    <label class="block text-xs font-medium text-slate-300 mb-1.5">
                      Server Arguments (Command Line Args)
                    </label>
                    <input
                      :value="selectedConnector.mcp.args.join(' ')"
                      @input="(e: any) => selectedConnector!.mcp!.args = e.target.value.split(' ').filter(Boolean)"
                      type="text"
                      placeholder="-y @modelcontextprotocol/server-..."
                      class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-purple-500"
                    />
                    <span class="text-[10px] text-slate-500 mt-1 block">Space-separated arguments passed to the executable.</span>
                  </div>
                </div>

                <!-- Environment Variables -->
                <div class="space-y-3 pt-2">
                  <div class="flex items-center justify-between">
                    <label class="block text-xs font-medium text-slate-300">
                      Environment Variables & Secrets (<code class="text-purple-400 font-mono">env</code>)
                    </label>
                    <span class="text-[11px] text-slate-500 font-mono">
                      {{ Object.keys(selectedConnector.mcp.env || {}).length }} Variables
                    </span>
                  </div>

                  <!-- Existing Env Vars Table -->
                  <div v-if="selectedConnector.mcp.env && Object.keys(selectedConnector.mcp.env).length > 0" class="space-y-2">
                    <div
                      v-for="(val, key) in selectedConnector.mcp.env"
                      :key="key"
                      class="p-2.5 rounded-lg bg-slate-950 border border-slate-800 flex items-center justify-between gap-3 text-xs font-mono"
                    >
                      <span class="font-bold text-purple-300 text-xs w-1/3 truncate">{{ key }}</span>
                      <input
                        v-model="selectedConnector.mcp.env[key]"
                        type="password"
                        placeholder="Variable value"
                        class="flex-1 h-7 px-2.5 bg-slate-900 border border-slate-750 rounded text-slate-200 font-mono text-[11px] focus:outline-none focus:border-purple-500"
                      />
                      <button
                        @click="promptRemoveEnvVar(key as string)"
                        class="p-1 rounded hover:bg-slate-800 text-slate-500 hover:text-rose-400 transition"
                        title="Delete variable"
                      >
                        <Trash2 class="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  <!-- Add New Env Var Row -->
                  <div class="flex items-center gap-2 pt-1">
                    <input
                      v-model="newEnvKey"
                      type="text"
                      placeholder="KEY_NAME (e.g. API_TOKEN)"
                      class="w-1/3 h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-purple-500 uppercase"
                    />
                    <input
                      v-model="newEnvVal"
                      type="text"
                      placeholder="Secret or Configuration Value"
                      class="flex-1 h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-purple-500"
                    />
                    <button
                      @click="addEnvVar"
                      type="button"
                      class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-750 text-xs font-medium text-slate-200 flex items-center gap-1 transition"
                    >
                      <Plus class="w-3.5 h-3.5" />
                      <span>Add</span>
                    </button>
                  </div>
                </div>
              </div>

              <!-- MCP Action Bar -->
              <div class="pt-4 border-t border-slate-800 flex items-center justify-between">
                <button
                  @click="testMCP"
                  :disabled="isTestingMCP"
                  type="button"
                  class="h-9 px-4 rounded-lg bg-purple-950/70 hover:bg-purple-900 border border-purple-800/80 text-xs font-medium text-purple-200 flex items-center gap-1.5 transition"
                >
                  <Cpu class="w-3.5 h-3.5 text-purple-400" :class="{ 'animate-spin': isTestingMCP }" />
                  <span>{{ isTestingMCP ? 'Testing Runtime...' : 'Verify Host MCP Runtime' }}</span>
                </button>

                <button
                  @click="saveConnector"
                  :disabled="isSaving"
                  type="button"
                  class="h-9 px-6 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-semibold text-white flex items-center gap-1.5 transition shadow-md shadow-blue-950"
                >
                  <Save class="w-3.5 h-3.5" />
                  <span>{{ isSaving ? 'Saving...' : 'Save MCP Settings' }}</span>
                </button>
              </div>

            </div>
          </div>

          <!-- ================= TAB 3: JIRA BACKLOG EXPLORER ================= -->
          <div v-else-if="activeEditTab === 'explorer' && selectedConnector.id === 'jira'" class="space-y-4 animate-fade-in">
            <div class="p-6 rounded-2xl bg-slate-900 border border-slate-800 space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
                    JIRA Backlog Issues
                    <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-blue-950 text-blue-300 border border-blue-800/60">
                      Live Feed
                    </span>
                  </h3>
                  <p class="text-xs text-slate-400 mt-0.5">
                    Browse and import open JIRA issues directly into your Kanban pipeline with pre-configured method routing and PRD synthesis.
                  </p>
                </div>

                <button
                  @click="showImportModal = true"
                  type="button"
                  class="h-8 px-3 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-semibold text-white flex items-center gap-1.5 transition shadow-md shadow-blue-950"
                >
                  <Download class="w-3.5 h-3.5" />
                  <span>Import Issue</span>
                </button>
              </div>

              <!-- Search Filter -->
              <div class="flex items-center gap-2">
                <div class="relative flex-1">
                  <Search class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
                  <input
                    v-model="explorerQuery"
                    @keydown.enter="searchExplorer"
                    type="text"
                    placeholder="Filter by issue key (e.g. PAY-1044), label, or text..."
                    class="w-full h-9 pl-9 pr-4 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-blue-500"
                  />
                </div>
                <button
                  @click="searchExplorer"
                  type="button"
                  class="h-9 px-3.5 rounded-lg bg-slate-800 hover:bg-slate-750 text-xs text-slate-200 flex items-center gap-1.5 transition"
                >
                  <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isSearchingExplorer }" />
                  <span>Search</span>
                </button>
              </div>

              <!-- Issues List -->
              <div v-if="isSearchingExplorer" class="p-8 text-center text-xs text-slate-500 animate-pulse">
                Querying JIRA API for open backlog issues...
              </div>
              <div v-else-if="explorerIssues.length === 0" class="p-8 text-center text-xs text-slate-500 bg-slate-950 rounded-lg border border-slate-800">
                No JIRA issues found matching current query.
              </div>
              <div v-else class="space-y-2.5">
                <div
                  v-for="issue in explorerIssues"
                  :key="issue.key || (issue as any).Key"
                  class="p-4 rounded-xl bg-slate-950 border border-slate-800 hover:border-slate-700 transition flex items-center justify-between gap-4"
                >
                  <div class="space-y-1.5 min-w-0 flex-1">
                    <div class="flex items-center gap-2">
                      <span class="px-2 py-0.5 rounded font-mono text-[11px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60">
                        {{ issue.key || (issue as any).Key }}
                      </span>
                      <span class="text-xs font-semibold text-slate-200 truncate">
                        {{ issue.summary || (issue as any).Summary }}
                      </span>
                    </div>

                    <p class="text-xs text-slate-400 line-clamp-1 leading-relaxed">
                      {{ issue.description || (issue as any).Description }}
                    </p>

                    <div class="flex items-center gap-3 text-[11px] font-mono text-slate-500">
                      <span>Status: <strong class="text-slate-300">{{ issue.status || (issue as any).Status }}</strong></span>
                      <span>•</span>
                      <span>Priority: <strong class="text-amber-400">{{ issue.priority || (issue as any).Priority }}</strong></span>
                      <span>•</span>
                      <span>Assignee: <strong class="text-slate-300">{{ issue.assignee || (issue as any).Assignee || 'Unassigned' }}</strong></span>
                    </div>
                  </div>

                  <div class="flex items-center gap-2 flex-shrink-0">
                    <a
                      :href="issue.url || (issue as any).URL"
                      target="_blank"
                      class="p-2 rounded-lg bg-slate-900 border border-slate-800 text-slate-400 hover:text-blue-400 hover:border-slate-700 transition"
                      title="View in JIRA"
                    >
                      <ExternalLink class="w-4 h-4" />
                    </a>

                    <button
                      @click="showImportModal = true"
                      type="button"
                      class="h-8 px-3 rounded-lg bg-blue-950/80 hover:bg-blue-900 border border-blue-800/80 text-xs font-medium text-blue-300 hover:text-blue-100 flex items-center gap-1.5 transition"
                    >
                      <Download class="w-3.5 h-3.5" />
                      <span>Import</span>
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

        </div>
      </div>
    </template>

    <!-- ==================== MCP EXPORT MODAL ==================== -->
    <div
      v-if="showMCPExportModal"
      class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4"
    >
      <div class="w-full max-w-2xl bg-slate-900 border border-purple-800/60 rounded-2xl shadow-2xl p-6 space-y-4">
        <div class="flex items-center justify-between border-b border-slate-800 pb-3">
          <div class="flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-purple-950 border border-purple-800/60 flex items-center justify-center text-purple-400">
              <Cpu class="w-4 h-4" />
            </div>
            <div>
              <h3 class="font-bold text-sm text-slate-100">Standard MCP Configuration</h3>
              <p class="text-[11px] text-slate-400 font-mono">mcp_config.json export format</p>
            </div>
          </div>
          <button
            @click="showMCPExportModal = false"
            class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
          >
            ✕
          </button>
        </div>

        <p class="text-xs text-slate-300 leading-relaxed">
          Copy this configuration block directly into your <code class="text-purple-400 font-mono">mcp_config.json</code> (for Antigravity, Claude Desktop, or Cursor) to register all configured connectors as agent tools:
        </p>

        <!-- Code Block -->
        <div class="relative rounded-xl bg-slate-950 border border-slate-800 p-4 font-mono text-xs text-slate-200 overflow-x-auto max-h-80">
          <pre>{{ mcpExportJSON }}</pre>
          <button
            @click="copyMCPConfig"
            class="absolute top-3 right-3 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-750 text-xs font-sans text-slate-200 flex items-center gap-1.5 transition shadow"
          >
            <Check v-if="copiedMCP" class="w-3.5 h-3.5 text-emerald-400" />
            <Copy v-else class="w-3.5 h-3.5" />
            <span>{{ copiedMCP ? 'Copied!' : 'Copy JSON' }}</span>
          </button>
        </div>

        <div class="flex items-center justify-between pt-2">
          <span class="text-[11px] text-slate-500 font-mono">
            Default Location: ~/.gemini/config/mcp_config.json
          </span>
          <button
            @click="showMCPExportModal = false"
            class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-750 text-xs font-semibold text-slate-200 transition"
          >
            Done
          </button>
        </div>
      </div>
    </div>

    <!-- JIRA Import Modal Dialog -->
    <JiraImportModal
      v-if="showImportModal"
      @close="showImportModal = false"
      @imported="searchExplorer"
    />

    <!-- Confirmation Modal -->
    <ConfirmDeleteModal
      :is-open="isConfirmDeleteOpen"
      :title="confirmDeleteTitle"
      :message="confirmDeleteMessage"
      :item-name="confirmDeleteItemName"
      :note="confirmDeleteNote"
      :confirm-text="confirmDeleteBtnText"
      :loading="isConfirmDeleteLoading"
      @confirm="handleExecuteConfirmDelete"
      @close="isConfirmDeleteOpen = false"
    />
  </div>
</template>

