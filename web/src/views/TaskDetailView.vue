<script setup lang="ts">
import { ref, computed, onMounted, watch, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTaskStore } from '../stores/tasks'
import AgentConsole from '../components/console/AgentConsole.vue'
import TaskReasoningPanel from '../components/task/TaskReasoningPanel.vue'
import TaskScopePanel from '../components/task/TaskScopePanel.vue'
import TaskDiffViewer from '../components/task/TaskDiffViewer.vue'
import TaskWorkspacePanel from '../components/task/TaskWorkspacePanel.vue'
import TaskPullRequestPanel from '../components/task/TaskPullRequestPanel.vue'
import TaskUATGuidePanel from '../components/task/TaskUATGuidePanel.vue'
import TaskRoutingPanel from '../components/task/TaskRoutingPanel.vue'
import { taskLifecycle, stagePosition, stageName, STAGES, TONE_BADGE, ACTION_BUTTON } from '../composables/taskLifecycle'
import type { TaskWorktreeDTO, TaskDependencyInfoDTO } from '../types'
import { api } from '../services/api'
import {
  ChevronLeft, Terminal, BrainCircuit, ClipboardList, FileDiff, FolderGit2, Copy, Check, GitBranch,
  ExternalLink, MoreHorizontal, Play, Pause, RotateCw, RefreshCcw, Eye, Loader2, Link2, ShieldAlert, GitPullRequest, ClipboardCheck, Route,
} from 'lucide-vue-next'

type Tab = 'console' | 'reasoning' | 'scope' | 'routing' | 'changes' | 'pr' | 'uat' | 'workspace'
const TABS: { id: Tab; label: string; icon: any }[] = [
  { id: 'console', label: 'Console', icon: Terminal },
  { id: 'reasoning', label: 'AI Reasoning', icon: BrainCircuit },
  { id: 'scope', label: 'Scope & Requirements', icon: ClipboardList },
  { id: 'routing', label: 'Method & Model', icon: Route },
  { id: 'changes', label: 'Changes', icon: FileDiff },
  { id: 'pr', label: 'Pull Request', icon: GitPullRequest },
  { id: 'uat', label: 'UAT Guide', icon: ClipboardCheck },
  { id: 'workspace', label: 'Workspace & Repos', icon: FolderGit2 },
]
const LEGACY_TAB: Record<string, Tab> = { terminal: 'console', thoughts: 'reasoning', specs: 'scope' }

const route = useRoute()
const router = useRouter()
const taskStore = useTaskStore()

const taskId = computed(() => route.params.id as string)
const activeTab = computed<Tab>({
  get: () => {
    const q = String(route.query.tab || 'console')
    return (TABS.some((t) => t.id === q) ? q : LEGACY_TAB[q] || 'console') as Tab
  },
  set: (tab) => router.replace({ query: { ...route.query, tab } }),
})

const worktree = ref<TaskWorktreeDTO | null>(null)
const dependencies = ref<TaskDependencyInfoDTO | null>(null)
const notFound = ref(false)
const copied = ref('')
const showMenu = ref(false)

const task = computed(() => taskStore.tasks.find((t) => t.id === taskId.value) || null)
const life = computed(() => taskLifecycle(task.value))
const pending = computed(() => !!taskStore.pendingActions[taskId.value])
const pos = computed(() => stagePosition(task.value?.current_stage_id))
const meta = computed(() => task.value?.metadata || {})
const ACTION_ICON = { run: Play, resume: RotateCw, pause: Pause, reset: RefreshCcw, review: Eye, assign: FolderGit2, approve: ShieldAlert }

async function loadContext() {
  const [wt, deps] = await Promise.allSettled([api.getTaskWorktree(taskId.value), api.getTaskDependencies(taskId.value)])
  worktree.value = wt.status === 'fulfilled' ? wt.value : null
  dependencies.value = deps.status === 'fulfilled' ? deps.value : null
}

async function load() {
  notFound.value = false
  const t = await taskStore.fetchTask(taskId.value)
  if (!t) {
    notFound.value = true
    return
  }
  await loadContext()
}

async function primary() {
  const a = life.value?.primary
  if (!a || pending.value) return
  if (a.kind === 'review' || a.kind === 'approve') {
    activeTab.value = 'console' // stage output and the approve / request-changes controls live there
    return
  }
  if (a.kind === 'assign') {
    router.replace({ query: { ...route.query, tab: 'workspace', edit: 'repos' } })
    return
  }
  if (a.kind !== 'pause') activeTab.value = 'console'
  try {
    await taskStore.performTaskAction(taskId.value, a.kind)
  } catch {
    /* reported by the store */
  }
}

async function resetAndRetry() {
  showMenu.value = false
  activeTab.value = 'console'
  try {
    await taskStore.performTaskAction(taskId.value, 'reset')
  } catch {
    /* reported by the store */
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = text
    setTimeout(() => (copied.value = ''), 1400)
  } catch {
    /* clipboard unavailable */
  }
}

function closeMenu() {
  showMenu.value = false
}
function toggleMenu() {
  showMenu.value = !showMenu.value
  if (showMenu.value) setTimeout(() => document.addEventListener('click', closeMenu, { once: true }))
}
onBeforeUnmount(() => document.removeEventListener('click', closeMenu))

// Worktree and prerequisites change as the task runs; refresh them on every state change.
watch(() => [task.value?.state, task.value?.current_stage_id, (task.value?.assigned_repos || []).join(',')], (_now, before) => {
  if (before && task.value) loadContext()
})
watch(taskId, load)
onMounted(() => {
  taskStore.initWebSocketSync()
  load()
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Deleted or unknown task -->
    <div v-if="notFound" class="flex-1 flex items-center justify-center p-6">
      <div class="text-center space-y-3">
        <p class="text-sm text-slate-200">{{ taskId }} is not on the board.</p>
        <p class="text-xs text-slate-500">It may have been removed. JIRA-linked tasks can be imported again.</p>
        <button type="button" @click="router.push('/')" class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs text-slate-200">Back to board</button>
      </div>
    </div>

    <template v-else>
      <!-- Header -->
      <header class="px-4 py-2.5 bg-slate-900/80 border-b border-slate-800 flex items-center gap-3 flex-shrink-0">
        <button type="button" @click="router.push('/')" aria-label="Back to board"
          class="p-1 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 flex-shrink-0">
          <ChevronLeft class="w-5 h-5" />
        </button>
        <button type="button" @click="copy(taskId)" title="Copy task ID"
          class="font-mono text-xs font-bold text-slate-100 bg-slate-800 hover:bg-slate-700 px-2 py-0.5 rounded border border-slate-700 flex items-center gap-1 flex-shrink-0">
          {{ taskId }}
          <Check v-if="copied === taskId" class="w-3 h-3 text-emerald-400" /><Copy v-else class="w-3 h-3 text-slate-400" />
        </button>
        <a v-if="meta.jira_key" :href="meta.jira_url" target="_blank" rel="noopener"
          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-blue-950/70 border border-blue-800/70 text-blue-300 hover:text-blue-100 text-[11px] font-mono flex-shrink-0">
          {{ meta.jira_key }} <ExternalLink class="w-2.5 h-2.5" />
        </a>
        <h1 class="text-sm font-medium text-slate-100 truncate min-w-0" :title="task?.title">{{ task?.title || 'Loading…' }}</h1>

        <div v-if="task && life" class="ml-auto flex items-center gap-2 flex-shrink-0">
          <span class="inline-flex items-center gap-1.5 px-2 py-1 rounded border text-[11px] font-semibold" :class="TONE_BADGE[life.tone]">
            <span v-if="life.isRunning" class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-pulse"></span>
            {{ life.status }}
          </span>
          <button v-if="life.primary" type="button" @click="primary" :disabled="pending" :title="life.primary.hint"
            class="h-8 px-3 rounded-lg border text-xs font-semibold flex items-center gap-1.5 disabled:opacity-60 disabled:cursor-wait"
            :class="ACTION_BUTTON[life.primary.kind]">
            <Loader2 v-if="pending" class="w-3.5 h-3.5 animate-spin" />
            <component v-else :is="ACTION_ICON[life.primary.kind]" class="w-3.5 h-3.5" />
            {{ life.primary.label }}
          </button>
          <div class="relative" @click.stop>
            <button type="button" @click="toggleMenu" aria-label="More actions" :aria-expanded="showMenu"
              class="h-8 w-8 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-slate-300 flex items-center justify-center">
              <MoreHorizontal class="w-4 h-4" />
            </button>
            <div v-if="showMenu" role="menu" class="absolute right-0 top-full mt-1 w-64 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl z-30 py-1 text-xs">
              <button type="button" role="menuitem" @click="resetAndRetry" :disabled="task.state === 'RUNNING' || task.state === 'COMPLETED'"
                class="w-full text-left px-3 py-2 hover:bg-slate-800 disabled:opacity-40 disabled:hover:bg-transparent text-slate-200">
                <div class="flex items-center gap-2"><RefreshCcw class="w-3.5 h-3.5" /> Reset failures & retry stage</div>
                <div class="text-[11px] text-slate-500 pl-5">Clears the failure counter and re-runs {{ stageName(task.current_stage_id) }}. Files are untouched.</div>
              </button>
              <button v-if="worktree?.branch" type="button" role="menuitem" @click="copy(worktree.branch); showMenu = false"
                class="w-full text-left px-3 py-2 hover:bg-slate-800 text-slate-200 flex items-center gap-2">
                <GitBranch class="w-3.5 h-3.5" /> Copy branch name
              </button>
              <a v-if="meta.jira_url" :href="meta.jira_url" target="_blank" rel="noopener" role="menuitem"
                class="block px-3 py-2 hover:bg-slate-800 text-slate-200"><span class="flex items-center gap-2"><ExternalLink class="w-3.5 h-3.5" /> Open in JIRA</span></a>
            </div>
          </div>
        </div>
      </header>

      <!-- What's happening + context -->
      <div v-if="task && life" class="px-4 py-2 border-b border-slate-800 bg-slate-950 flex flex-wrap items-center gap-x-5 gap-y-1.5 text-xs flex-shrink-0">
        <div class="min-w-0 flex-1">
          <span class="text-slate-100 font-medium">{{ life.title }}.</span>
          <span class="text-slate-400 ml-1">{{ life.detail }}</span>
        </div>
        <div class="flex items-center gap-1.5" :title="`Stage ${pos.index + 1} of ${STAGES.length}: ${stageName(task.current_stage_id)}`">
          <span v-for="(s, i) in STAGES" :key="s" class="h-1.5 w-4 rounded-full"
            :class="task.state === 'COMPLETED' || i < pos.index ? 'bg-emerald-500' : i === pos.index ? (life.isRunning ? 'bg-sky-400 animate-pulse' : 'bg-slate-300') : 'bg-slate-700'"></span>
          <span class="ml-1 text-slate-400">{{ pos.index + 1 }}/{{ STAGES.length }}</span>
        </div>
        <button v-if="worktree?.branch" type="button" @click="activeTab = 'workspace'" class="flex items-center gap-1.5 font-mono text-teal-300 hover:text-teal-100"
          :title="worktree.status === 'ACTIVE' ? 'Worktree active' : 'Branch is created when the task first runs'">
          <GitBranch class="w-3.5 h-3.5" /> {{ worktree.branch }}
          <span v-if="worktree.status !== 'ACTIVE'" class="font-sans text-[10px] text-slate-500">(planned)</span>
        </button>
        <span v-if="task.dependencies?.length" class="flex items-center gap-1" :class="dependencies?.all_satisfied ? 'text-emerald-400' : 'text-orange-300'">
          <Link2 class="w-3.5 h-3.5" />
          {{ dependencies?.all_satisfied ? 'Prerequisites done' : `Needs ${(dependencies?.unmet_dependencies || task.dependencies).join(', ')}` }}
        </span>
      </div>

      <!-- Tabs -->
      <nav class="min-h-10 px-4 py-1 bg-slate-900 border-b border-slate-800 flex flex-wrap items-center gap-1 flex-shrink-0" role="tablist">
        <button v-for="t in TABS" :key="t.id" type="button" role="tab" :aria-selected="activeTab === t.id" @click="activeTab = t.id"
          class="h-8 px-3 rounded text-xs transition-colors flex items-center gap-2 border whitespace-nowrap"
          :class="activeTab === t.id ? 'bg-slate-800 border-slate-700 text-slate-100 font-semibold' : 'border-transparent text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'">
          <component :is="t.icon" class="w-3.5 h-3.5" />
          {{ t.label }}
          <span v-if="t.id === 'pr' && task?.metadata?.pr_out_of_sync" class="w-1.5 h-1.5 rounded-full bg-amber-400"
            title="The pull request does not have the latest changes — update it" aria-label="pull request needs update" />
        </button>
      </nav>

      <main v-if="task" class="flex-1 min-h-0 flex flex-col">
        <AgentConsole v-if="activeTab === 'console'" :task-id="taskId" :show-header="true" @task-updated="loadContext" />
        <TaskReasoningPanel v-else-if="activeTab === 'reasoning'" :task-id="taskId" @open-console="activeTab = 'console'" />
        <TaskScopePanel v-else-if="activeTab === 'scope'" :task="task" :dependencies="dependencies" @updated="load" />
        <TaskRoutingPanel v-else-if="activeTab === 'routing'" :task="task" @updated="load" />
        <TaskDiffViewer v-else-if="activeTab === 'changes'" :task-id="taskId" :worktree="worktree" />
        <TaskPullRequestPanel v-else-if="activeTab === 'pr'" :task="task" @refresh="loadContext" />
        <TaskUATGuidePanel v-else-if="activeTab === 'uat'" :task="task" />
        <TaskWorkspacePanel v-else-if="activeTab === 'workspace'" :task="task" :worktree="worktree" :start-editing="route.query.edit === 'repos'"
          @open-changes="activeTab = 'changes'" @refresh="loadContext" />
      </main>
      <div v-else class="flex-1 flex items-center justify-center text-xs text-slate-500 animate-pulse">Loading task…</div>
    </template>
  </div>
</template>
