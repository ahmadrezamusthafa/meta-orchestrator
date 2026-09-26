<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../services/api'
import { useToastStore } from '../stores/toast'
import type { ConnectorsConfig, JiraConfig, ConfluenceConfig, TestConnectorResponse, JiraIssueDTO } from '../types'
import JiraImportModal from '../components/kanban/JiraImportModal.vue'
import { 
  Plug, CheckCircle2, AlertCircle, RefreshCw, Wifi, Save, ExternalLink, 
  Eye, EyeOff, Search, Download, FileText, Check, ShieldCheck, ArrowRight, Layers
} from 'lucide-vue-next'

const toastStore = useToastStore()

const activeTab = ref<'jira' | 'confluence' | 'explorer'>('jira')
const isLoading = ref(true)
const isSaving = ref(false)
const isTestingJira = ref(false)
const isTestingConf = ref(false)

const showJiraToken = ref(false)
const showConfToken = ref(false)
const showImportModal = ref(false)

const jiraTestResult = ref<TestConnectorResponse | null>(null)
const confTestResult = ref<TestConnectorResponse | null>(null)

const jiraForm = ref<JiraConfig>({
  enabled: true,
  base_url: 'https://jira.atlassian.net',
  username: 'devops@meta-orchestrator.io',
  api_token: '••••••••••••••••••••••••',
  project_key: 'PAY',
  jql_filter: 'project = PAY AND status != Done ORDER BY created DESC',
  auto_detect_keys: true,
  auto_sync_status: true,
  status: 'connected',
})

const confForm = ref<ConfluenceConfig>({
  enabled: true,
  base_url: 'https://wiki.atlassian.net',
  username: 'devops@meta-orchestrator.io',
  api_token: '••••••••••••••••••••••••',
  space_key: 'ARCH',
  parent_page_id: '',
  auto_publish_tech_docs: true,
  auto_publish_prd: true,
  status: 'connected',
})

// Explorer Tab State
const explorerQuery = ref('')
const explorerIssues = ref<JiraIssueDTO[]>([])
const isSearchingExplorer = ref(false)

async function loadConfig() {
  isLoading.value = true
  try {
    const cfg = await api.getConnectors()
    if (cfg.jira) jiraForm.value = { ...cfg.jira }
    if (cfg.confluence) confForm.value = { ...cfg.confluence }
  } catch (err: any) {
    toastStore.error('Connectors', 'Failed to load configuration')
  } finally {
    isLoading.value = false
  }
}

async function testJira() {
  isTestingJira.value = true
  jiraTestResult.value = null
  try {
    const res = await api.testConnector({
      type: 'jira',
      jira: jiraForm.value
    })
    jiraTestResult.value = res
    if (res.success) {
      jiraForm.value.status = 'connected'
      toastStore.success('JIRA Connected', `${res.message} (${res.latency_ms}ms)`)
    } else {
      jiraForm.value.status = 'error'
      toastStore.error('JIRA Test Failed', res.message)
    }
  } catch (err: any) {
    toastStore.error('Test Failed', err.message)
  } finally {
    isTestingJira.value = false
  }
}

async function testConfluence() {
  isTestingConf.value = true
  confTestResult.value = null
  try {
    const res = await api.testConnector({
      type: 'confluence',
      confluence: confForm.value
    })
    confTestResult.value = res
    if (res.success) {
      confForm.value.status = 'connected'
      toastStore.success('Confluence Connected', `${res.message} (${res.latency_ms}ms)`)
    } else {
      confForm.value.status = 'error'
      toastStore.error('Confluence Test Failed', res.message)
    }
  } catch (err: any) {
    toastStore.error('Test Failed', err.message)
  } finally {
    isTestingConf.value = false
  }
}

async function saveJira() {
  isSaving.value = true
  try {
    await api.updateJira(jiraForm.value)
    toastStore.success('Saved', 'JIRA configuration updated successfully')
  } catch (err: any) {
    toastStore.error('Save Failed', err.message)
  } finally {
    isSaving.value = false
  }
}

async function saveConfluence() {
  isSaving.value = true
  try {
    await api.updateConfluence(confForm.value)
    toastStore.success('Saved', 'Confluence configuration updated successfully')
  } catch (err: any) {
    toastStore.error('Save Failed', err.message)
  } finally {
    isSaving.value = false
  }
}

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

onMounted(async () => {
  await loadConfig()
  searchExplorer()
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- View Header -->
    <div class="px-8 py-5 border-b border-slate-800 bg-slate-900/60 flex items-center justify-between">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-xl bg-blue-950/80 border border-blue-800/80 flex items-center justify-center text-blue-400 shadow-md">
          <Plug class="w-5 h-5" />
        </div>
        <div>
          <div class="flex items-center gap-2.5">
            <h1 class="text-base font-semibold text-slate-100">Third-Party Connectors</h1>
            <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-blue-950 text-blue-400 border border-blue-800/50">
              Ecosystem Hub
            </span>
          </div>
          <p class="text-xs text-slate-400 mt-0.5">
            Connect JIRA for automatic ticket detection & two-way sync, and Confluence for zero-touch RFC tech doc publishing.
          </p>
        </div>
      </div>

      <!-- Quick Status Badges -->
      <div class="flex items-center gap-2">
        <div class="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 flex items-center gap-2 text-xs font-mono">
          <span class="w-2 h-2 rounded-full" :class="jiraForm.status === 'connected' ? 'bg-emerald-400' : 'bg-amber-400'"></span>
          <span class="text-slate-300">JIRA:</span>
          <span class="text-emerald-400 font-semibold uppercase">{{ jiraForm.status }}</span>
        </div>

        <div class="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 flex items-center gap-2 text-xs font-mono">
          <span class="w-2 h-2 rounded-full" :class="confForm.status === 'connected' ? 'bg-emerald-400' : 'bg-amber-400'"></span>
          <span class="text-slate-300">Confluence:</span>
          <span class="text-emerald-400 font-semibold uppercase">{{ confForm.status }}</span>
        </div>
      </div>
    </div>

    <!-- Navigation Tabs -->
    <div class="px-8 border-b border-slate-800 bg-slate-900/30 flex items-center justify-between">
      <div class="flex items-center gap-6">
        <button
          @click="activeTab = 'jira'"
          class="py-3 text-xs font-medium border-b-2 transition-colors flex items-center gap-2"
          :class="activeTab === 'jira' ? 'border-blue-500 text-blue-400 font-semibold' : 'border-transparent text-slate-400 hover:text-slate-200'"
        >
          <span class="w-2 h-2 rounded-full bg-blue-400"></span>
          <span>JIRA Settings & Auto-Detect</span>
        </button>

        <button
          @click="activeTab = 'confluence'"
          class="py-3 text-xs font-medium border-b-2 transition-colors flex items-center gap-2"
          :class="activeTab === 'confluence' ? 'border-indigo-500 text-indigo-400 font-semibold' : 'border-transparent text-slate-400 hover:text-slate-200'"
        >
          <span class="w-2 h-2 rounded-full bg-indigo-400"></span>
          <span>Confluence Tech Docs Publishing</span>
        </button>

        <button
          @click="activeTab = 'explorer'"
          class="py-3 text-xs font-medium border-b-2 transition-colors flex items-center gap-2"
          :class="activeTab === 'explorer' ? 'border-emerald-500 text-emerald-400 font-semibold' : 'border-transparent text-slate-400 hover:text-slate-200'"
        >
          <Download class="w-3.5 h-3.5 text-emerald-400" />
          <span>JIRA Backlog Explorer & Importer</span>
        </button>
      </div>

      <div class="text-[11px] font-mono text-slate-500">
        Active Environment: <span class="text-slate-300">Default Workspace</span>
      </div>
    </div>

    <!-- Tab Contents -->
    <div class="flex-1 overflow-y-auto p-8">
      <div class="max-w-4xl mx-auto space-y-6">

        <!-- =================== JIRA TAB =================== -->
        <div v-if="activeTab === 'jira'" class="space-y-6 animate-fade-in">
          <!-- Overview Card -->
          <div class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
                  Atlassian JIRA Integration
                  <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-emerald-950 text-emerald-400 border border-emerald-800/60">
                    Live Connector
                  </span>
                </h3>
                <p class="text-xs text-slate-400 mt-0.5">
                  Synchronizes Kanban tasks with JIRA issues. Automatically parses ticket keys like 
                  <code class="text-blue-400 bg-slate-950 px-1 py-0.5 rounded">[PAY-1042]</code> in task titles and creates clickable deep links.
                </p>
              </div>

              <div class="flex items-center gap-2">
                <button
                  @click="testJira"
                  :disabled="isTestingJira"
                  type="button"
                  class="h-8 px-3 rounded-lg bg-blue-950/70 hover:bg-blue-900 border border-blue-800/80 text-xs font-medium text-blue-300 hover:text-blue-100 flex items-center gap-1.5 transition-colors"
                >
                  <Wifi class="w-3.5 h-3.5" :class="{ 'animate-pulse text-blue-400': isTestingJira }" />
                  <span>{{ isTestingJira ? 'Pinging...' : 'Test Connection' }}</span>
                </button>
              </div>
            </div>

            <!-- Diagnostics Test Banner -->
            <div
              v-if="jiraTestResult"
              class="p-3.5 rounded-lg border text-xs font-mono flex items-start gap-2.5 transition-all"
              :class="jiraTestResult.success ? 'bg-emerald-950/40 border-emerald-800/70 text-emerald-300' : 'bg-rose-950/40 border-rose-800/70 text-rose-300'"
            >
              <CheckCircle2 v-if="jiraTestResult.success" class="w-4 h-4 text-emerald-400 flex-shrink-0 mt-0.5" />
              <AlertCircle v-else class="w-4 h-4 text-rose-400 flex-shrink-0 mt-0.5" />
              <div class="space-y-1">
                <div class="font-semibold">{{ jiraTestResult.message }}</div>
                <div class="text-[11px] text-slate-400 flex flex-wrap gap-x-4">
                  <span>Latency: <strong class="text-slate-200">{{ jiraTestResult.latency_ms }}ms</strong></span>
                  <span v-if="jiraTestResult.connected_as">Connected As: <strong class="text-slate-200">{{ jiraTestResult.connected_as }}</strong></span>
                  <span v-if="jiraTestResult.server_info">Server: <strong class="text-slate-200">{{ jiraTestResult.server_info }}</strong></span>
                </div>
              </div>
            </div>

            <!-- Form Fields -->
            <div class="grid grid-cols-2 gap-4 pt-2">
              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">JIRA Base URL</label>
                <input
                  v-model="jiraForm.base_url"
                  type="text"
                  placeholder="https://company.atlassian.net"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">Username / Service Account Email</label>
                <input
                  v-model="jiraForm.username"
                  type="text"
                  placeholder="devops@company.com"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">API Token / Personal Access Token</label>
                <div class="relative">
                  <input
                    v-model="jiraForm.api_token"
                    :type="showJiraToken ? 'text' : 'password'"
                    placeholder="Atlassian Cloud API Token"
                    class="w-full h-9 pl-3 pr-9 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                  />
                  <button
                    @click="showJiraToken = !showJiraToken"
                    type="button"
                    class="absolute right-2.5 top-2.5 text-slate-500 hover:text-slate-300"
                  >
                    <EyeOff v-if="showJiraToken" class="w-4 h-4" />
                    <Eye v-else class="w-4 h-4" />
                  </button>
                </div>
              </div>

              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">Default Project Key</label>
                <input
                  v-model="jiraForm.project_key"
                  type="text"
                  placeholder="e.g. PAY or PROJ"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500 uppercase"
                />
              </div>

              <div class="col-span-2">
                <label class="block text-xs font-medium text-slate-300 mb-1.5">Default JQL Query Filter</label>
                <input
                  v-model="jiraForm.jql_filter"
                  type="text"
                  placeholder="project = PAY AND status != Done ORDER BY created DESC"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-blue-500"
                />
              </div>
            </div>

            <!-- Toggles -->
            <div class="pt-4 border-t border-slate-800/80 space-y-3">
              <label class="flex items-center justify-between p-3 rounded-lg bg-slate-950 border border-slate-800/80 cursor-pointer hover:border-slate-700 transition-colors">
                <div>
                  <div class="text-xs font-medium text-slate-200">Automatically Detect JIRA Keys on Kanban Cards</div>
                  <div class="text-[11px] text-slate-400">Scans task titles and descriptions for ticket keys (e.g. PAY-1042) and adds deep-link badges.</div>
                </div>
                <input
                  v-model="jiraForm.auto_detect_keys"
                  type="checkbox"
                  class="w-4 h-4 rounded text-blue-600 focus:ring-0 focus:ring-offset-0 bg-slate-900 border-slate-700"
                />
              </label>

              <label class="flex items-center justify-between p-3 rounded-lg bg-slate-950 border border-slate-800/80 cursor-pointer hover:border-slate-700 transition-colors">
                <div>
                  <div class="text-xs font-medium text-slate-200">Auto-Sync Stage Transitions to JIRA Status</div>
                  <div class="text-[11px] text-slate-400">Advances JIRA issue workflow (e.g. In Progress → In Review → Done) as pipeline stages complete.</div>
                </div>
                <input
                  v-model="jiraForm.auto_sync_status"
                  type="checkbox"
                  class="w-4 h-4 rounded text-blue-600 focus:ring-0 focus:ring-offset-0 bg-slate-900 border-slate-700"
                />
              </label>
            </div>

            <!-- Save Action -->
            <div class="pt-3 flex justify-end">
              <button
                @click="saveJira"
                :disabled="isSaving"
                type="button"
                class="h-9 px-5 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-medium text-white flex items-center gap-1.5 transition-colors shadow-md shadow-blue-950"
              >
                <Save class="w-3.5 h-3.5" />
                <span>{{ isSaving ? 'Saving...' : 'Save JIRA Settings' }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- =================== CONFLUENCE TAB =================== -->
        <div v-if="activeTab === 'confluence'" class="space-y-6 animate-fade-in">
          <div class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
                  Atlassian Confluence Documentation
                  <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-indigo-950 text-indigo-400 border border-indigo-800/60">
                    Auto-Publishing
                  </span>
                </h3>
                <p class="text-xs text-slate-400 mt-0.5">
                  Publishes synthesized Technical Design RFCs (<code class="text-indigo-400 bg-slate-950 px-1 py-0.5 rounded">TECH_DOC_RFC.md</code>) 
                  and PRDs directly to your Confluence team spaces.
                </p>
              </div>

              <div class="flex items-center gap-2">
                <button
                  @click="testConfluence"
                  :disabled="isTestingConf"
                  type="button"
                  class="h-8 px-3 rounded-lg bg-indigo-950/70 hover:bg-indigo-900 border border-indigo-800/80 text-xs font-medium text-indigo-300 hover:text-indigo-100 flex items-center gap-1.5 transition-colors"
                >
                  <Wifi class="w-3.5 h-3.5" :class="{ 'animate-pulse text-indigo-400': isTestingConf }" />
                  <span>{{ isTestingConf ? 'Testing...' : 'Test Connection' }}</span>
                </button>
              </div>
            </div>

            <!-- Diagnostics Test Banner -->
            <div
              v-if="confTestResult"
              class="p-3.5 rounded-lg border text-xs font-mono flex items-start gap-2.5 transition-all"
              :class="confTestResult.success ? 'bg-emerald-950/40 border-emerald-800/70 text-emerald-300' : 'bg-rose-950/40 border-rose-800/70 text-rose-300'"
            >
              <CheckCircle2 v-if="confTestResult.success" class="w-4 h-4 text-emerald-400 flex-shrink-0 mt-0.5" />
              <AlertCircle v-else class="w-4 h-4 text-rose-400 flex-shrink-0 mt-0.5" />
              <div class="space-y-1">
                <div class="font-semibold">{{ confTestResult.message }}</div>
                <div class="text-[11px] text-slate-400 flex flex-wrap gap-x-4">
                  <span>Latency: <strong class="text-slate-200">{{ confTestResult.latency_ms }}ms</strong></span>
                  <span v-if="confTestResult.target_entity">Space: <strong class="text-slate-200">{{ confTestResult.target_entity }}</strong></span>
                  <span v-if="confTestResult.server_info">Server: <strong class="text-slate-200">{{ confTestResult.server_info }}</strong></span>
                </div>
              </div>
            </div>

            <!-- Form Fields -->
            <div class="grid grid-cols-2 gap-4 pt-2">
              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">Confluence Base URL</label>
                <input
                  v-model="confForm.base_url"
                  type="text"
                  placeholder="https://company.atlassian.net/wiki"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-indigo-500"
                />
              </div>

              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">Username / Email</label>
                <input
                  v-model="confForm.username"
                  type="text"
                  placeholder="devops@company.com"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-indigo-500"
                />
              </div>

              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">API Token</label>
                <div class="relative">
                  <input
                    v-model="confForm.api_token"
                    :type="showConfToken ? 'text' : 'password'"
                    placeholder="Confluence API Token"
                    class="w-full h-9 pl-3 pr-9 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-indigo-500"
                  />
                  <button
                    @click="showConfToken = !showConfToken"
                    type="button"
                    class="absolute right-2.5 top-2.5 text-slate-500 hover:text-slate-300"
                  >
                    <EyeOff v-if="showConfToken" class="w-4 h-4" />
                    <Eye v-else class="w-4 h-4" />
                  </button>
                </div>
              </div>

              <div>
                <label class="block text-xs font-medium text-slate-300 mb-1.5">Target Confluence Space Key</label>
                <input
                  v-model="confForm.space_key"
                  type="text"
                  placeholder="e.g. ARCH or ENG"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-indigo-500 uppercase"
                />
              </div>

              <div class="col-span-2">
                <label class="block text-xs font-medium text-slate-300 mb-1.5">Parent Page ID (Optional)</label>
                <input
                  v-model="confForm.parent_page_id"
                  type="text"
                  placeholder="e.g. 1048576 (Leave blank to publish under Space Root)"
                  class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 font-mono focus:outline-none focus:border-indigo-500"
                />
              </div>
            </div>

            <!-- Toggles -->
            <div class="pt-4 border-t border-slate-800/80 space-y-3">
              <label class="flex items-center justify-between p-3 rounded-lg bg-slate-950 border border-slate-800/80 cursor-pointer hover:border-slate-700 transition-colors">
                <div>
                  <div class="text-xs font-medium text-slate-200">Automatically Publish Tech Doc RFCs</div>
                  <div class="text-[11px] text-slate-400">Pushes Technical Architecture Design documents to Confluence upon reaching the RFC design stage.</div>
                </div>
                <input
                  v-model="confForm.auto_publish_tech_docs"
                  type="checkbox"
                  class="w-4 h-4 rounded text-indigo-600 focus:ring-0 focus:ring-offset-0 bg-slate-900 border-slate-700"
                />
              </label>

              <label class="flex items-center justify-between p-3 rounded-lg bg-slate-950 border border-slate-800/80 cursor-pointer hover:border-slate-700 transition-colors">
                <div>
                  <div class="text-xs font-medium text-slate-200">Automatically Publish PRD Discovery Documents</div>
                  <div class="text-[11px] text-slate-400">Exports product requirement specifications into Confluence requirements catalogs.</div>
                </div>
                <input
                  v-model="confForm.auto_publish_prd"
                  type="checkbox"
                  class="w-4 h-4 rounded text-indigo-600 focus:ring-0 focus:ring-offset-0 bg-slate-900 border-slate-700"
                />
              </label>
            </div>

            <!-- Save Action -->
            <div class="pt-3 flex justify-end">
              <button
                @click="saveConfluence"
                :disabled="isSaving"
                type="button"
                class="h-9 px-5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-xs font-medium text-white flex items-center gap-1.5 transition-colors shadow-md shadow-indigo-950"
              >
                <Save class="w-3.5 h-3.5" />
                <span>{{ isSaving ? 'Saving...' : 'Save Confluence Settings' }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- =================== EXPLORER & IMPORT TAB =================== -->
        <div v-if="activeTab === 'explorer'" class="space-y-4 animate-fade-in">
          <div class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-4">
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
                class="h-8 px-3 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-medium text-white flex items-center gap-1.5 transition-colors shadow-md shadow-blue-950"
              >
                <Download class="w-3.5 h-3.5" />
                <span>Import Modal</span>
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
                class="h-9 px-3.5 rounded-lg bg-slate-800 hover:bg-slate-750 text-xs text-slate-200 flex items-center gap-1.5 transition-colors"
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
                class="p-4 rounded-xl bg-slate-950 border border-slate-800 hover:border-slate-700 transition-all flex items-center justify-between gap-4"
              >
                <div class="space-y-1.5">
                  <div class="flex items-center gap-2">
                    <span class="px-2 py-0.5 rounded font-mono text-[11px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60">
                      {{ issue.key || (issue as any).Key }}
                    </span>
                    <span class="text-xs font-semibold text-slate-200">
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
                    class="p-2 rounded-lg bg-slate-900 border border-slate-800 text-slate-400 hover:text-blue-400 hover:border-slate-700 transition-colors"
                    title="View in JIRA"
                  >
                    <ExternalLink class="w-4 h-4" />
                  </a>

                  <button
                    @click="showImportModal = true"
                    type="button"
                    class="h-8 px-3 rounded-lg bg-blue-950/80 hover:bg-blue-900 border border-blue-800/80 text-xs font-medium text-blue-300 hover:text-blue-100 flex items-center gap-1.5 transition-colors"
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

    <!-- JIRA Import Modal Dialog -->
    <JiraImportModal
      v-if="showImportModal"
      @close="showImportModal = false"
      @imported="searchExplorer"
    />
  </div>
</template>
