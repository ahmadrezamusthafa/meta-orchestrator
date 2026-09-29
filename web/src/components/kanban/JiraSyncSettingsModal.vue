<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useTaskStore } from '../../stores/tasks'
import { useWorkflowStore } from '../../stores/workflows'
import { useProjectStore } from '../../stores/projects'
import { useToastStore } from '../../stores/toast'
import type { JiraSyncConfig } from '../../types'
import { X, RefreshCw, Plus, Trash2, AlertCircle, CheckCircle2, Settings2, RotateCcw } from 'lucide-vue-next'

const emit = defineEmits<{ (e: 'close'): void }>()

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()
const projectStore = useProjectStore()
const toastStore = useToastStore()

const FALLBACK_STAGES = [
  { id: 'prd_discovery', name: 'PRD Discovery' },
  { id: 'atdd_creation', name: 'ATDD Creation' },
  { id: 'techdoc_rfc', name: 'Technical RFC' },
  { id: 'task_breakdown', name: 'Task Breakdown' },
  { id: 'task_implementation', name: 'Implementation' },
  { id: 'e2e_validation', name: 'E2E Validation' },
  { id: 'uat_verification', name: 'UAT Verification' },
  { id: 'signoff_merge', name: 'Signoff & Merge' },
]

const JQL_PRESETS = [
  { label: 'My open issues', jql: 'assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC' },
  { label: 'My current sprint', jql: 'assignee = currentUser() AND sprint in openSprints() ORDER BY rank ASC' },
  { label: 'My in-progress', jql: 'assignee = currentUser() AND statusCategory = "In Progress" ORDER BY updated DESC' },
]

const INTERVALS = [
  { value: 60, label: 'Every minute' },
  { value: 300, label: 'Every 5 minutes' },
  { value: 900, label: 'Every 15 minutes' },
  { value: 1800, label: 'Every 30 minutes' },
  { value: 3600, label: 'Every hour' },
]

const form = ref<JiraSyncConfig | null>(null)
const rules = ref<{ status: string; stage: string }[]>([])
const newExcluded = ref('')
const isSaving = ref(false)
const isLoading = ref(true)

const stages = computed(() => {
  const cols = workflowStore.projectedColumns
  return cols.length > 0 ? cols.map((c) => ({ id: c.id, name: c.name })) : FALLBACK_STAGES
})
const repoOptions = computed(() => {
  const names = new Set<string>(projectStore.activeRepos.map((r: any) => r.name))
  ;(form.value?.assigned_repos || []).forEach((r) => names.add(r))
  return Array.from(names)
})
const status = computed(() => taskStore.jiraSync?.status || null)

function hydrate() {
  const cfg = taskStore.jiraSync?.config
  if (!cfg) return
  form.value = { ...cfg, assigned_repos: [...(cfg.assigned_repos || [])], exclude_statuses: [...(cfg.exclude_statuses || [])] }
  rules.value = Object.entries(cfg.status_stage_map || {}).map(([status, stage]) => ({ status, stage }))
}

function toggleRepo(repo: string) {
  if (!form.value) return
  const repos = form.value.assigned_repos || []
  form.value.assigned_repos = repos.includes(repo) ? repos.filter((r) => r !== repo) : [...repos, repo]
}

function addExcluded() {
  const st = newExcluded.value.trim()
  if (!form.value || !st) return
  const list = form.value.exclude_statuses || []
  if (!list.some((x) => x.toLowerCase() === st.toLowerCase())) form.value.exclude_statuses = [...list, st]
  newExcluded.value = ''
}

function addRule() {
  rules.value.push({ status: '', stage: stages.value[0]?.id || 'prd_discovery' })
}

async function save(syncNow: boolean) {
  if (!form.value) return
  isSaving.value = true
  try {
    const map: Record<string, string> = {}
    rules.value.forEach((r) => {
      if (r.status.trim()) map[r.status.trim()] = r.stage
    })
    await taskStore.saveJiraSync({ ...form.value, status_stage_map: map })
    toastStore.success('Sync rules saved', form.value.enabled ? 'The board picks up matching JIRA issues automatically.' : 'Automatic sync is off.')
    if (syncNow) await taskStore.runJiraSync()
    emit('close')
  } catch (err: any) {
    toastStore.error('Could not save rules', err?.message || 'Unknown error')
  } finally {
    isSaving.value = false
  }
}

async function restoreDismissed() {
  try {
    await taskStore.restoreDismissedJira()
    toastStore.success('Removed issues restored', 'They return to the board on the next sync.')
  } catch (err: any) {
    toastStore.error('Restore failed', err?.message || 'Unknown error')
  }
}

function formatTime(iso?: string) {
  if (!iso || iso.startsWith('0001')) return 'never'
  return new Date(iso).toLocaleString()
}

onMounted(async () => {
  await Promise.all([
    taskStore.fetchJiraSync(),
    projectStore.projects.length ? Promise.resolve() : projectStore.fetchProjects(),
  ])
  hydrate()
  isLoading.value = false
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm" @click.self="emit('close')">
    <div class="w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="px-6 py-4 border-b border-slate-800 flex items-center justify-between">
        <div class="flex items-center gap-2.5">
          <div class="w-8 h-8 rounded-lg bg-blue-950/80 border border-blue-800/80 flex items-center justify-center text-blue-400">
            <Settings2 class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-sm font-semibold text-slate-100">JIRA board sync</h3>
            <p class="text-xs text-slate-400">Choose which issues appear on the board and where they start</p>
          </div>
        </div>
        <button @click="emit('close')" type="button" aria-label="Close"
          class="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div v-if="isLoading || !form" class="p-10 text-center text-xs text-slate-400 animate-pulse">Loading sync rules…</div>

      <div v-else class="flex-1 overflow-y-auto p-6 space-y-5">
        <!-- Connection / last run -->
        <div v-if="status && !status.connected"
          class="p-3 rounded-lg border border-amber-800/60 bg-amber-950/30 text-xs text-amber-200 flex items-start gap-2">
          <AlertCircle class="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
          <span>JIRA isn't connected yet. Add your base URL, email and API token in
            <router-link to="/connectors" class="underline hover:text-white" @click="emit('close')">Connectors</router-link>
            — the rules below apply as soon as it is.</span>
        </div>
        <div v-else-if="status"
          class="p-3 rounded-lg border text-xs flex items-start gap-2"
          :class="status.last_error ? 'border-rose-800/60 bg-rose-950/30 text-rose-200' : 'border-slate-800 bg-slate-950/60 text-slate-300'">
          <AlertCircle v-if="status.last_error" class="w-4 h-4 text-rose-400 flex-shrink-0 mt-0.5" />
          <CheckCircle2 v-else class="w-4 h-4 text-emerald-400 flex-shrink-0 mt-0.5" />
          <div class="space-y-0.5">
            <div>Last sync: <strong>{{ formatTime(status.last_run_at) }}</strong>
              <template v-if="!status.last_error && status.last_run_at"> · {{ status.fetched }} matched, {{ status.created }} new, {{ status.updated }} updated</template>
            </div>
            <div v-if="status.last_error">{{ status.last_error }}</div>
            <div class="text-slate-500">{{ status.linked_tasks }} tasks on the board are linked to JIRA</div>
          </div>
        </div>

        <!-- Enable -->
        <label class="flex items-start gap-3 cursor-pointer">
          <input v-model="form.enabled" type="checkbox" class="mt-0.5 accent-blue-500" />
          <span>
            <span class="block text-xs font-medium text-slate-200">Load matching issues onto the board automatically</span>
            <span class="block text-[11px] text-slate-500">New issues become tasks waiting to run. Sync never moves or runs existing tasks.</span>
          </span>
        </label>

        <!-- JQL -->
        <div>
          <label for="sync-jql" class="block text-xs font-medium text-slate-300 mb-1.5">Which issues (JQL)</label>
          <textarea id="sync-jql" v-model="form.jql" rows="2" spellcheck="false"
            class="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-blue-500" />
          <div class="flex flex-wrap gap-1.5 mt-1.5">
            <button v-for="p in JQL_PRESETS" :key="p.label" type="button" @click="form.jql = p.jql"
              class="px-2 py-0.5 rounded border text-[11px] transition-colors"
              :class="form.jql === p.jql ? 'border-blue-600 bg-blue-950/60 text-blue-300' : 'border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700'">
              {{ p.label }}
            </button>
          </div>
        </div>

        <!-- Finished work never syncs -->
        <div>
          <span class="block text-xs font-medium text-slate-300 mb-1">Never sync finished issues</span>
          <p class="text-[11px] text-slate-500 mb-1.5">
            Issues in JIRA's <span class="font-mono">Done</span> category are always excluded. Also skip these status names:
          </p>
          <div class="flex flex-wrap items-center gap-1.5">
            <span v-for="(st, i) in form.exclude_statuses || []" :key="st"
              class="inline-flex items-center gap-1 pl-2 pr-1 py-0.5 rounded bg-slate-800 border border-slate-700 text-[11px] text-slate-200">
              {{ st }}
              <button type="button" :aria-label="`Stop excluding ${st}`" @click="form.exclude_statuses!.splice(i, 1)"
                class="p-0.5 rounded text-slate-400 hover:text-rose-300">
                <X class="w-3 h-3" />
              </button>
            </span>
            <input v-model="newExcluded" @keydown.enter.prevent="addExcluded" type="text" placeholder="Add status…"
              aria-label="Add a finished status"
              class="h-7 w-32 px-2 bg-slate-950 border border-slate-800 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500" />
          </div>
        </div>

        <!-- Cadence -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label for="sync-interval" class="block text-xs font-medium text-slate-300 mb-1.5">Check for changes</label>
            <select id="sync-interval" v-model.number="form.interval_seconds"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500">
              <option v-for="i in INTERVALS" :key="i.value" :value="i.value">{{ i.label }}</option>
            </select>
          </div>
          <div>
            <label for="sync-max" class="block text-xs font-medium text-slate-300 mb-1.5">Max issues per sync</label>
            <input id="sync-max" v-model.number="form.max_issues" type="number" min="1" max="100"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500" />
          </div>
        </div>

        <!-- Placement rules -->
        <div class="pt-4 border-t border-slate-800 space-y-3">
          <div>
            <label for="sync-stage" class="block text-xs font-medium text-slate-300 mb-1.5">New issues start in</label>
            <select id="sync-stage" v-model="form.default_stage_id"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500">
              <option v-for="s in stages" :key="s.id" :value="s.id">{{ s.name }}</option>
            </select>
          </div>

          <div>
            <div class="flex items-center justify-between mb-1.5">
              <span class="text-xs font-medium text-slate-300">Unless the JIRA status is…</span>
              <button type="button" @click="addRule"
                class="text-[11px] text-blue-400 hover:text-blue-300 flex items-center gap-1">
                <Plus class="w-3 h-3" /> Add rule
              </button>
            </div>
            <p v-if="rules.length === 0" class="text-[11px] text-slate-500">No status rules — every new issue starts in the stage above.</p>
            <div v-for="(rule, i) in rules" :key="i" class="flex items-center gap-2 mb-1.5">
              <input v-model="rule.status" type="text" placeholder="JIRA status, e.g. In Review" :aria-label="`Rule ${i + 1} JIRA status`"
                class="flex-1 h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500" />
              <span class="text-[11px] text-slate-500">→</span>
              <select v-model="rule.stage" :aria-label="`Rule ${i + 1} stage`"
                class="flex-1 h-8 px-2 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500">
                <option v-for="s in stages" :key="s.id" :value="s.id">{{ s.name }}</option>
              </select>
              <button type="button" @click="rules.splice(i, 1)" :aria-label="`Remove rule ${i + 1}`"
                class="p-1.5 rounded text-slate-500 hover:text-rose-400 hover:bg-slate-800">
                <Trash2 class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </div>

        <!-- Execution defaults -->
        <div class="pt-4 border-t border-slate-800 grid grid-cols-2 gap-4">
          <div>
            <label for="sync-workflow" class="block text-xs font-medium text-slate-300 mb-1.5">Workflow</label>
            <select id="sync-workflow" v-model="form.workflow_id"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500">
              <option v-if="!workflowStore.workflows.some(w => w.id === 'general_ai_sdlc')" value="general_ai_sdlc">General AI SDLC</option>
              <option v-for="w in workflowStore.workflows" :key="w.id" :value="w.id">{{ w.name }}</option>
            </select>
          </div>
          <div>
            <label for="sync-method" class="block text-xs font-medium text-slate-300 mb-1.5">Method</label>
            <select id="sync-method" v-model="form.selected_method"
              class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500">
              <option value="BMAD">BMAD</option>
              <option value="Supervisor">Supervisor</option>
              <option value="ReAct">ReAct</option>
              <option value="Superpower">Superpower</option>
            </select>
          </div>
          <div class="col-span-2">
            <span class="block text-xs font-medium text-slate-300 mb-1.5">Repositories</span>
            <p v-if="repoOptions.length === 0" class="text-[11px] text-slate-500">
              No repositories registered. Add a project in <router-link to="/projects" class="underline" @click="emit('close')">Projects</router-link>, or assign repos per task later.
            </p>
            <div v-else class="flex flex-wrap gap-1.5">
              <button v-for="repo in repoOptions" :key="repo" type="button" @click="toggleRepo(repo)"
                class="px-2.5 py-1 rounded text-[11px] font-mono border transition-all"
                :class="(form.assigned_repos || []).includes(repo) ? 'bg-blue-950/80 border-blue-700 text-blue-300' : 'bg-slate-950 border-slate-800 text-slate-400 hover:border-slate-700'">
                {{ repo }}
              </button>
            </div>
          </div>
        </div>

        <label class="flex items-start gap-3 cursor-pointer">
          <input v-model="form.update_existing" type="checkbox" class="mt-0.5 accent-blue-500" />
          <span>
            <span class="block text-xs font-medium text-slate-200">Keep linked tasks up to date</span>
            <span class="block text-[11px] text-slate-500">Refresh title, description, status and priority from JIRA on every sync.</span>
          </span>
        </label>

        <div v-if="status && status.dismissed > 0" class="flex items-center justify-between text-[11px] text-slate-400 p-2.5 rounded-lg bg-slate-950/60 border border-slate-800">
          <span>{{ status.dismissed }} issue{{ status.dismissed === 1 ? '' : 's' }} you removed from the board {{ status.dismissed === 1 ? 'is' : 'are' }} skipped by sync.</span>
          <button type="button" @click="restoreDismissed" class="flex items-center gap-1 text-blue-400 hover:text-blue-300">
            <RotateCcw class="w-3 h-3" /> Restore
          </button>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-6 py-3.5 border-t border-slate-800 bg-slate-950/80 flex items-center justify-end gap-2">
        <button @click="emit('close')" type="button"
          class="h-9 px-4 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-300 transition-colors">Cancel</button>
        <button @click="save(false)" :disabled="!form || isSaving" type="button"
          class="h-9 px-4 rounded-lg bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-xs font-medium text-slate-200 transition-colors">Save</button>
        <button @click="save(true)" :disabled="!form || isSaving || !status?.connected" type="button"
          class="h-9 px-4 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-xs font-medium text-white flex items-center gap-1.5 transition-colors">
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isSaving }" />
          Save &amp; sync now
        </button>
      </div>
    </div>
  </div>
</template>
