<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { api } from '../../services/api'
import { useTaskStore } from '../../stores/tasks'
import { useWorkflowStore } from '../../stores/workflows'
import { useToastStore } from '../../stores/toast'
import { useProjectStore } from '../../stores/projects'
import type { JiraIssueDTO } from '../../types'
import { X, Search, Check, Download, AlertCircle, ExternalLink, RefreshCw, User, ShieldCheck, Zap, Layers } from 'lucide-vue-next'

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'imported', taskId: string): void
}>()

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()
const toastStore = useToastStore()
const projectStore = useProjectStore()
const loadError = ref('')

const searchQuery = ref('')
const issues = ref<JiraIssueDTO[]>([])
const selectedIssue = ref<JiraIssueDTO | null>(null)
const isLoading = ref(false)
const isSubmitting = ref(false)
const importingKey = ref<string | null>(null)
const jiraUsername = ref('')

const selectedWorkflowId = ref('general_ai_sdlc')
const selectedMethod = ref('BMAD')
const startStageId = ref('prd_discovery')
const selectedRepos = ref<string[]>([])

const availableRepos = computed(() => {
  const names = new Set<string>(projectStore.activeRepos.map((r: any) => r.name))
  selectedRepos.value.forEach((r) => names.add(r))
  return Array.from(names)
})

function boardTaskFor(issue: JiraIssueDTO) {
  return taskStore.jiraKeysOnBoard.get(issue.key.toUpperCase()) || null
}

// Start from the sync rules so manual imports and synced issues land the same way.
async function loadDefaults() {
  if (!taskStore.jiraSync) await taskStore.fetchJiraSync()
  const cfg = taskStore.jiraSync?.config
  if (cfg) {
    selectedWorkflowId.value = cfg.workflow_id || selectedWorkflowId.value
    selectedMethod.value = cfg.selected_method || selectedMethod.value
    startStageId.value = cfg.default_stage_id || startStageId.value
    if (cfg.assigned_repos?.length) selectedRepos.value = [...cfg.assigned_repos]
  }
  if (!projectStore.projects.length) await projectStore.fetchProjects()
  // Left empty, the daemon assigns repositories from the sync rules (project / component / label).
}

async function loadJiraConfig() {
  try {
    const config = await api.getConnectors()
    if (config?.jira?.username) {
      jiraUsername.value = config.jira.username
    }
  } catch (e) {
    console.debug('Failed to get JIRA config', e)
  }
}

async function fetchIssues() {
  isLoading.value = true
  loadError.value = ''
  try {
    const list = await api.getJiraIssues(searchQuery.value)
    issues.value = list
    if (!selectedIssue.value || boardTaskFor(selectedIssue.value)) {
      selectedIssue.value = list.find((i) => !boardTaskFor(i)) || null
    }
  } catch (err: any) {
    issues.value = []
    selectedIssue.value = null
    loadError.value = err.message || 'Failed to fetch issues'
  } finally {
    isLoading.value = false
  }
}

function selectIssue(issue: JiraIssueDTO) {
  if (boardTaskFor(issue)) return
  selectedIssue.value = issue
}

function toggleRepo(repo: string) {
  if (selectedRepos.value.includes(repo)) {
    selectedRepos.value = selectedRepos.value.filter(r => r !== repo)
  } else {
    selectedRepos.value.push(repo)
  }
}

async function handleImport() {
  if (!selectedIssue.value) return
  isSubmitting.value = true
  try {
    const task = await api.importJiraIssue({
      issue_key: selectedIssue.value.key,
      workflow_id: selectedWorkflowId.value,
      selected_method: selectedMethod.value,
      assigned_repos: selectedRepos.value,
      start_stage_id: startStageId.value
    })
    toastStore.success('Imported from JIRA', `${selectedIssue.value.key} added to the board as ${task.id}`)
    await taskStore.fetchTasks()
    emit('imported', task.id)
    emit('close')
  } catch (err: any) {
    toastStore.error('Import Failed', err.message || 'Could not import ticket')
  } finally {
    isSubmitting.value = false
  }
}

async function quickImport(issue: JiraIssueDTO, e?: Event) {
  if (e) e.stopPropagation()
  importingKey.value = issue.key
  try {
    const task = await api.importJiraIssue({
      issue_key: issue.key,
      workflow_id: selectedWorkflowId.value,
      selected_method: selectedMethod.value,
      assigned_repos: selectedRepos.value,
      start_stage_id: startStageId.value
    })
    toastStore.success('Imported from JIRA', `${issue.key} added to the board as ${task.id}`)
    await taskStore.fetchTasks()
    emit('imported', task.id)
    emit('close')
  } catch (err: any) {
    toastStore.error('Import Failed', err.message || 'Could not import ticket')
  } finally {
    importingKey.value = null
  }
}

onMounted(async () => {
  loadJiraConfig()
  await loadDefaults()
  fetchIssues()
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm animate-fade-in">
    <div class="w-full max-w-3xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[88vh]">
      <!-- Header -->
      <div class="px-6 py-4 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center gap-2.5">
          <div class="w-8 h-8 rounded-lg bg-blue-950/80 border border-blue-800/80 flex items-center justify-center text-blue-400">
            <Download class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-sm font-semibold text-slate-100 flex flex-wrap items-center gap-2">
              <span>Import from JIRA</span>
              <span class="text-[10px] font-mono px-2 py-0.5 rounded bg-blue-950 text-blue-400 border border-blue-800/50 whitespace-nowrap">My Assigned Tickets</span>
            </h3>
            <p class="text-xs text-slate-400">Scoped exclusively to issues assigned to your active account credentials</p>
          </div>
        </div>
        <button
          @click="$emit('close')"
          type="button"
          class="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Credential & JQL Status Bar -->
      <div class="px-6 py-2.5 bg-blue-950/20 border-b border-blue-900/30 flex items-center justify-between text-xs flex-wrap gap-2">
        <div class="flex items-center gap-2 text-slate-300">
          <div class="w-2 h-2 rounded-full" :class="loadError ? 'bg-rose-400' : 'bg-emerald-400'"></div>
          <span class="text-slate-400">Assigned to:</span>
          <span class="font-mono font-semibold text-blue-300 px-2 py-0.5 rounded bg-blue-950/80 border border-blue-800/60 flex items-center gap-1.5">
            <User class="w-3 h-3 text-blue-400" />
            {{ jiraUsername || 'currentUser()' }}
          </span>
        </div>
        <div class="flex items-center gap-1.5 text-[11px] text-blue-400/90 font-mono">
          <ShieldCheck class="w-3.5 h-3.5 text-blue-400" />
          <span>JQL: assignee = currentUser()</span>
        </div>
      </div>

      <!-- Main Body -->
      <div class="flex-1 overflow-y-auto p-6 space-y-5">
        <!-- Search & Filter Bar -->
        <div class="flex items-center gap-2">
          <div class="relative flex-1">
            <Search class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
            <input
              v-model="searchQuery"
              @keydown.enter="fetchIssues"
              type="text"
              placeholder="Search your assigned JIRA issues by key (e.g. PAY-1044), summary or label..."
              class="w-full h-9 pl-9 pr-4 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-blue-500 transition-colors"
            />
          </div>
          <button
            @click="fetchIssues"
            type="button"
            class="h-9 px-3.5 rounded-lg bg-slate-800 hover:bg-slate-750 border border-slate-700 text-xs text-slate-200 flex items-center gap-1.5 transition-colors"
          >
            <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isLoading }" />
            <span>Refresh</span>
          </button>
        </div>

        <!-- Issue Picker -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <label class="text-xs font-medium text-slate-300">
              Assigned Tickets ({{ issues.length }})
            </label>
            <span class="text-[11px] text-slate-500">Click to configure, or 1-click Quick Import</span>
          </div>

          <div v-if="isLoading" class="p-8 text-center text-xs text-slate-400 animate-pulse bg-slate-950/40 rounded-lg border border-slate-800/80">
            Querying JIRA API for tickets assigned to {{ jiraUsername || 'currentUser()' }}...
          </div>
          <div v-else-if="loadError" class="p-6 text-center text-xs bg-rose-950/20 rounded-lg border border-rose-900/50 text-rose-200 space-y-2">
            <p>{{ loadError }}</p>
            <router-link to="/connectors" class="inline-block underline text-rose-100 hover:text-white" @click="$emit('close')">Open Connectors</router-link>
          </div>
          <div v-else-if="issues.length === 0" class="p-8 text-center text-xs text-slate-400 bg-slate-950/60 rounded-lg border border-slate-800">
            No assigned JIRA issues match this search.
          </div>
          <div v-else class="grid grid-cols-1 gap-2 max-h-56 overflow-y-auto pr-1">
            <div
              v-for="issue in issues"
              :key="issue.key"
              @click="selectIssue(issue)"
              class="p-3 rounded-lg border transition-all flex items-center justify-between group/row"
              :class="boardTaskFor(issue)
                ? 'bg-slate-950/40 border-slate-800/60 opacity-60 cursor-default'
                : selectedIssue?.key === issue.key
                  ? 'bg-blue-950/40 border-blue-600/80 shadow-sm ring-1 ring-blue-500/30 cursor-pointer'
                  : 'bg-slate-950 border-slate-800/80 hover:border-slate-700 hover:bg-slate-900/60 cursor-pointer'"
            >
              <div class="flex items-center gap-3 overflow-hidden mr-3">
                <span class="px-2 py-0.5 rounded font-mono text-[11px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60 flex-shrink-0">
                  {{ issue.key }}
                </span>
                <div class="truncate">
                  <div class="text-xs font-medium text-slate-200 truncate group-hover/row:text-white transition-colors">
                    {{ issue.summary }}
                  </div>
                  <div class="text-[11px] text-slate-400 font-mono flex items-center gap-2 mt-1 flex-wrap">
                    <span class="inline-flex items-center gap-1 text-blue-300 font-medium bg-blue-950/60 px-1.5 py-0.2 rounded border border-blue-800/40">
                      <User class="w-3 h-3 text-blue-400" />
                      {{ issue.assignee || jiraUsername || 'You' }}
                    </span>
                    <span>•</span>
                    <span class="px-1.5 py-0.2 rounded text-[10px]" :class="{
                      'bg-rose-950/60 text-rose-300 border border-rose-800/40': issue.priority === 'High' || issue.priority === 'Highest',
                      'bg-amber-950/60 text-amber-300 border border-amber-800/40': issue.priority === 'Medium',
                      'bg-slate-800 text-slate-300': issue.priority === 'Low' || issue.priority === 'Lowest'
                    }">{{ issue.priority }}</span>
                    <span>•</span>
                    <span>Status: <strong class="text-slate-300">{{ issue.status }}</strong></span>
                  </div>
                </div>
              </div>

              <div class="flex items-center gap-2 flex-shrink-0">
                <span
                  v-if="boardTaskFor(issue)"
                  class="h-7 px-2.5 rounded bg-slate-800 border border-slate-700 text-slate-300 text-[11px] font-mono flex items-center"
                  :title="`Already on the board as ${boardTaskFor(issue)}`"
                >
                  On board · {{ boardTaskFor(issue) }}
                </span>
                <!-- 1-Click Quick Import Button -->
                <button
                  v-else
                  @click="quickImport(issue, $event)"
                  :disabled="importingKey === issue.key"
                  type="button"
                  title="Quick Import directly with defaults"
                  class="h-7 px-2.5 rounded bg-blue-600/20 hover:bg-blue-600 border border-blue-500/40 hover:border-blue-400 text-blue-300 hover:text-white text-[11px] font-medium flex items-center gap-1 transition-all"
                >
                  <Zap class="w-3 h-3 text-blue-400 group-hover/row:text-white" :class="{ 'animate-spin': importingKey === issue.key }" />
                  <span>{{ importingKey === issue.key ? 'Importing...' : 'Quick Import' }}</span>
                </button>

                <a
                  :href="issue.url"
                  target="_blank"
                  @click.stop
                  title="Open ticket in JIRA"
                  class="p-1 rounded text-slate-400 hover:text-blue-400 transition-colors"
                >
                  <ExternalLink class="w-3.5 h-3.5" />
                </a>

                <div
                  v-if="selectedIssue?.key === issue.key"
                  class="w-5 h-5 rounded-full bg-blue-500 text-white flex items-center justify-center shadow"
                >
                  <Check class="w-3 h-3" />
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Task Routing Configuration Grid -->
        <div class="grid grid-cols-2 gap-4 pt-2 border-t border-slate-800">
          <div>
            <label class="block text-xs font-medium text-slate-300 mb-1.5">SDLC Workflow</label>
            <select
              v-model="selectedWorkflowId"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500"
            >
              <option value="general_ai_sdlc">General AI SDLC (8 Stages)</option>
              <option v-for="w in workflowStore.workflows" :key="w.id" :value="w.id">{{ w.name }}</option>
            </select>
          </div>

          <div>
            <label class="block text-xs font-medium text-slate-300 mb-1.5">Execution Methodology</label>
            <select
              v-model="selectedMethod"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500"
            >
              <option value="Auto">Auto (Smart AI Router)</option>
              <option value="BMAD">BMAD (Architecture First)</option>
              <option value="Supervisor">Supervisor (Strict TDD / ATDD)</option>
              <option value="ReAct">ReAct (Autonomous Discovery)</option>
              <option value="Superpower">Superpower (High Context Parallel)</option>
            </select>
          </div>
        </div>

        <!-- Start Stage & Repos -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-xs font-medium text-slate-300 mb-1.5">Starting Stage</label>
            <select
              v-model="startStageId"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500"
            >
              <option value="prd_discovery">1. PRD Discovery & Requirements</option>
              <option value="atdd_creation">2. Shift-Left ATDD Creation</option>
              <option value="techdoc_rfc">3. Technical RFC Design</option>
              <option value="task_breakdown">4. Atomic Task Breakdown</option>
              <option value="task_implementation">5. Code Implementation</option>
            </select>
          </div>

          <div>
            <label class="block text-xs font-medium text-slate-300 mb-1.5">Assigned Repositories</label>
            <p v-if="availableRepos.length === 0" class="text-[11px] text-slate-500">No repositories registered — assign them later from the task.</p>
            <p v-else-if="selectedRepos.length === 0" class="text-[11px] text-slate-500 mb-1.5">None selected: repositories are assigned by your JIRA sync rules.</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="repo in availableRepos"
                :key="repo"
                type="button"
                @click="toggleRepo(repo)"
                class="px-2.5 py-1 rounded text-[11px] font-mono border transition-all"
                :class="selectedRepos.includes(repo)
                  ? 'bg-blue-950/80 border-blue-700 text-blue-300 font-medium'
                  : 'bg-slate-950 border-slate-800 text-slate-400 hover:border-slate-700'"
              >
                {{ repo }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-6 py-3.5 border-t border-slate-800 bg-slate-950/80 flex items-center justify-between">
        <div class="text-[11px] text-slate-400 flex items-center gap-1.5">
          <AlertCircle class="w-3.5 h-3.5 text-blue-400" />
          <span>Will automatically inject ticket acceptance criteria into PRD.md</span>
        </div>

        <div class="flex items-center gap-2">
          <button
            @click="$emit('close')"
            type="button"
            class="h-9 px-4 rounded-lg bg-slate-800 hover:bg-slate-750 text-xs font-medium text-slate-300 transition-colors"
          >
            Cancel
          </button>
          <button
            @click="handleImport"
            :disabled="!selectedIssue || isSubmitting"
            type="button"
            class="h-9 px-4 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-xs font-medium text-white flex items-center gap-1.5 transition-colors shadow-md shadow-blue-900/30"
          >
            <Download class="w-3.5 h-3.5" />
            <span>{{ isSubmitting ? 'Importing...' : 'Import Selected' }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
