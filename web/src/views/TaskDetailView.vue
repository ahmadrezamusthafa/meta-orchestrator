<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTaskStore } from '../stores/tasks'
import { useTerminalStore } from '../stores/terminal'
import { useToastStore } from '../stores/toast'
import AgentConsole from '../components/console/AgentConsole.vue'
import ThoughtFeed from '../components/thought/ThoughtFeed.vue'
import WorkspaceGraph from '../components/workspace/WorkspaceGraph.vue'
import FrustrationBanner from '../components/hitl/FrustrationBanner.vue'
import ContextInput from '../components/hitl/ContextInput.vue'
import ResetWorkspaceModal from '../components/hitl/ResetWorkspaceModal.vue'
import GateApprovalBar from '../components/hitl/GateApprovalBar.vue'
import PhaseStatusBadge from '../components/common/PhaseStatusBadge.vue'
import RoutingExplainerPill from '../components/common/RoutingExplainerPill.vue'
import OperatorGuidanceCard from '../components/common/OperatorGuidanceCard.vue'
import type { TaskProcessDTO, TaskWorktreeDTO, TaskDependencyInfoDTO } from '../types'
import { api } from '../services/api'
import { marked } from 'marked'
import {
  ChevronLeft,
  Terminal,
  BrainCircuit,
  Network,
  FileText,
  Play,
  Pause,
  RotateCcw,
  Sparkles,
  GitBranch,
  Link2,
  ShieldCheck,
  Lock,
  Copy,
  Check,
  Folder,
  Layers,
  CheckCircle2,
  Clock,
  Cpu,
  HardDrive,
  Info,
  ChevronDown,
  ChevronUp
} from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const taskId = computed(() => route.params.id as string)

const taskStore = useTaskStore()
const terminalStore = useTerminalStore()
const toastStore = useToastStore()

// Focused top-level tabs (Live Terminal is default)
const activeTab = ref<'terminal' | 'thoughts' | 'specs' | 'workspace'>('terminal')
const showResetModal = ref(false)
const showGuidanceCard = ref(true)
const taskProcess = ref<TaskProcessDTO | null>(null)
const worktreeInfo = ref<TaskWorktreeDTO | null>(null)
const dependencyInfo = ref<TaskDependencyInfoDTO | null>(null)
const isExecutingAI = ref(false)
const isResuming = ref(false)
const isPausing = ref(false)
const copiedTaskId = ref(false)

let unsubscribeWS: (() => void) | null = null

const currentTask = computed(() => taskStore.tasks.find(t => t.id === taskId.value) || taskStore.activeTask)
const isFrustrated = computed(() => currentTask.value?.state === 'BLOCKED_FRUSTRATION')
const isWaitingGate = computed(() => currentTask.value?.state === 'WAITING_GATE_APPROVAL')

const parsedDescription = computed(() => {
  if (!currentTask.value?.description) return ''
  try {
    return marked.parse(currentTask.value.description) as string
  } catch {
    return currentTask.value.description
  }
})

async function refreshTaskData() {
  await taskStore.fetchTask(taskId.value)
  try {
    taskProcess.value = await api.getTaskProcess(taskId.value)
  } catch (err) {
    console.debug('Failed to load process info:', err)
  }
  try {
    worktreeInfo.value = await api.getTaskWorktree(taskId.value)
  } catch (err) {
    console.debug('Failed to load worktree info:', err)
  }
  try {
    dependencyInfo.value = await api.getTaskDependencies(taskId.value)
  } catch (err) {
    console.debug('Failed to load dependency info:', err)
  }
}

onMounted(async () => {
  await refreshTaskData()
  unsubscribeWS = terminalStore.initTaskListeners(taskId.value)
})

onUnmounted(() => {
  if (unsubscribeWS) unsubscribeWS()
})

async function copyTaskId() {
  try {
    await navigator.clipboard.writeText(taskId.value)
    copiedTaskId.value = true
    setTimeout(() => { copiedTaskId.value = false }, 1800)
    toastStore.info('Copied', `Task ID ${taskId.value} copied to clipboard`)
  } catch {}
}

async function handleInjectGuidance(instruction: string) {
  try {
    await taskStore.injectContext(taskId.value, instruction)
    toastStore.success('Steering Dispatched', 'Instruction injected into active agent loop')
    await refreshTaskData()
  } catch (err: any) {
    toastStore.error('Steering Failed', err?.message || 'Server error')
  }
}

async function handleBannerInject() {
  await handleInjectGuidance('Operator reviewed failure trace and authorized retry from checkpoint.')
}

async function handleResetWorkspace() {
  try {
    await taskStore.resetWorkspace(taskId.value)
    showResetModal.value = false
    toastStore.warning('Workspace Reset', 'Container purged and volume state refreshed')
    await refreshTaskData()
  } catch (err: any) {
    toastStore.error('Reset Failed', err?.message || 'Server error')
  }
}

async function handleGateApprove() {
  try {
    await taskStore.updateGate(taskId.value, true)
    toastStore.success('Gate Approved', 'Advancing stage in zero-trust pipeline')
    await refreshTaskData()
  } catch (err: any) {
    toastStore.error('Gate Approval Failed', err?.message || 'Server error')
  }
}

async function handleGateReject() {
  try {
    await taskStore.updateGate(taskId.value, false, 'Revision requested by operator')
    toastStore.info('Gate Rejected', 'Task halted for operator revision')
    await refreshTaskData()
  } catch (err: any) {
    toastStore.error('Gate Rejection Failed', err?.message || 'Server error')
  }
}

async function handleExecuteTask() {
  if (isExecutingAI.value) return
  isExecutingAI.value = true
  try {
    await taskStore.executeTask(taskId.value)
    activeTab.value = 'terminal'
    setTimeout(refreshTaskData, 1000)
  } catch (err: any) {
    console.error('Execute error:', err)
  } finally {
    setTimeout(() => {
      isExecutingAI.value = false
    }, 2000)
  }
}

async function handleResume() {
  if (isResuming.value) return
  isResuming.value = true
  try {
    await taskStore.resumeTask(taskId.value)
    activeTab.value = 'terminal'
    await refreshTaskData()
  } catch (err: any) {
    console.error('Resume error:', err)
  } finally {
    setTimeout(() => {
      isResuming.value = false
    }, 1200)
  }
}

async function handlePause() {
  if (isPausing.value) return
  isPausing.value = true
  try {
    await taskStore.pauseTask(taskId.value)
    await refreshTaskData()
  } catch (err: any) {
    console.error('Pause error:', err)
  } finally {
    setTimeout(() => {
      isPausing.value = false
    }, 1000)
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden select-text">
    <!-- 1. Top Sub-Header / Breadcrumb & Action Toolbar -->
    <div class="h-12 px-4 bg-slate-900/80 border-b border-slate-800 flex items-center justify-between flex-shrink-0 z-20">
      <div class="flex items-center gap-3 min-w-0">
        <button
          @click="router.push('/')"
          class="p-1 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition-colors flex-shrink-0"
          title="Back to Kanban Mission Control"
        >
          <ChevronLeft class="w-5 h-5" />
        </button>

        <div class="flex items-center gap-2 min-w-0">
          <button
            @click="copyTaskId"
            class="font-mono text-xs font-bold text-slate-100 bg-slate-800/80 hover:bg-slate-700/80 px-2 py-0.5 rounded border border-slate-700 transition-colors flex items-center gap-1 flex-shrink-0"
            title="Click to copy Task ID"
          >
            <span>{{ taskId }}</span>
            <Check v-if="copiedTaskId" class="w-3 h-3 text-emerald-400" />
            <Copy v-else class="w-3 h-3 text-slate-400" />
          </button>
          <span class="text-slate-600 select-none">/</span>
          <h2 class="text-xs font-medium text-slate-200 truncate max-w-md" :title="currentTask?.title">
            {{ currentTask?.title || 'Loading task details...' }}
          </h2>
        </div>
      </div>

      <!-- Right Metadata Indicators & Execution Actions -->
      <div v-if="currentTask" class="flex items-center gap-2 flex-shrink-0">
        <RoutingExplainerPill
          :source="currentTask.metadata?.router_source"
          :rationale="currentTask.metadata?.router_rationale"
        />

        <span v-if="currentTask.metadata?.active_model" class="px-2 py-0.5 rounded bg-purple-950/80 border border-purple-800 text-[10px] font-mono text-purple-300 flex items-center gap-1">
          <BrainCircuit class="w-3 h-3 text-purple-400" />
          <span>{{ currentTask.metadata.active_model }}</span>
        </span>

        <span v-if="currentTask.token_usage?.total_tokens" class="px-2 py-0.5 rounded bg-slate-800 text-[10px] font-mono text-amber-300 flex items-center gap-1">
          <Sparkles class="w-2.5 h-2.5 text-amber-400" />
          <span>{{ currentTask.token_usage.total_tokens.toLocaleString() }} tokens</span>
        </span>

        <PhaseStatusBadge :state="currentTask.state" />

        <!-- Dynamic Action Controls: Pause / Resume / Run with 9Router -->
        <div class="flex items-center gap-1.5 ml-1">
          <!-- Pause Button (if RUNNING) -->
          <button
            v-if="currentTask.state === 'RUNNING' || taskProcess?.status === 'RUNNING'"
            @click="handlePause"
            :disabled="isPausing"
            class="h-7 px-2.5 rounded bg-amber-950/80 hover:bg-amber-900 border border-amber-700/80 text-amber-200 text-[11px] font-mono font-medium flex items-center gap-1.5 shadow-sm transition-all active:scale-95"
            title="Pause execution session while preserving all terminal logs"
          >
            <Pause class="w-3 h-3 text-amber-400" />
            <span>{{ isPausing ? 'Pausing...' : 'Pause' }}</span>
          </button>

          <!-- Resume Button (if SUSPENDED / PAUSED / BLOCKED) -->
          <button
            v-else-if="currentTask.state === 'SUSPENDED' || taskProcess?.status === 'PAUSED' || currentTask.state === 'BLOCKED_FRUSTRATION'"
            @click="handleResume"
            :disabled="isResuming"
            class="h-7 px-3 rounded bg-emerald-600 hover:bg-emerald-500 text-white text-[11px] font-mono font-medium flex items-center gap-1.5 shadow-sm transition-all active:scale-95"
            title="Resume session execution with full context preserved"
          >
            <Play v-if="!isResuming" class="w-3 h-3 fill-current" />
            <span v-else class="animate-spin text-white">⟳</span>
            <span>{{ isResuming ? 'Resuming...' : 'Resume Session' }}</span>
          </button>

          <!-- Run with 9Router (if PENDING, READY, or IDLE) -->
          <button
            v-else
            @click="handleExecuteTask"
            :disabled="isExecutingAI || currentTask.state === 'COMPLETED' || currentTask.state === 'WAITING_DEPENDENCY'"
            class="h-7 px-3 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-[11px] font-mono font-medium flex items-center gap-1.5 shadow-sm transition-all active:scale-95"
            title="Route and execute task through 9Router AI priority chain"
          >
            <Play class="w-3 h-3 fill-current" :class="{ 'animate-spin': isExecutingAI }" />
            <span>{{ isExecutingAI ? 'Routing AI...' : 'Run with 9Router' }}</span>
          </button>

          <!-- Reset Workspace Trigger -->
          <button
            @click="showResetModal = true"
            class="h-7 w-7 rounded bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-rose-400 border border-slate-700/80 flex items-center justify-center transition-colors"
            title="Purge container and reset workspace"
          >
            <RotateCcw class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>

    <!-- 2. Sticky HITL Alert Banners -->
    <!-- Frustration Alert Banner -->
    <FrustrationBanner
      v-if="isFrustrated"
      :trace="currentTask?.metadata?.failing_trace"
      @inject-guidance="handleBannerInject"
      @reset-workspace="showResetModal = true"
    />

    <!-- Gate Approval Bar -->
    <GateApprovalBar
      v-if="isWaitingGate"
      :stage-id="currentTask?.current_stage_id || ''"
      :stage-name="currentTask?.current_stage_id ? currentTask.current_stage_id.replace(/_/g, ' ').toUpperCase() : 'Gate Approval'"
      @approve="handleGateApprove"
      @reject="handleGateReject"
    />

    <!-- 3. Collapsible Operator Guidance Card -->
    <div v-if="currentTask" class="px-4 py-2 bg-slate-950 border-b border-slate-800/80 flex-shrink-0">
      <div class="flex items-center justify-between mb-1.5">
        <button
          @click="showGuidanceCard = !showGuidanceCard"
          class="text-[11px] font-mono font-medium text-slate-400 hover:text-slate-200 flex items-center gap-1.5 transition-colors"
        >
          <Info class="w-3.5 h-3.5 text-emerald-400" />
          <span>Stage Guidance & Next Action</span>
          <ChevronUp v-if="showGuidanceCard" class="w-3 h-3 text-slate-500" />
          <ChevronDown v-else class="w-3 h-3 text-slate-500" />
        </button>
      </div>

      <OperatorGuidanceCard
        v-if="showGuidanceCard"
        :task="currentTask"
        @approve-gate="handleGateApprove"
        @reject-gate="handleGateReject"
        @steer-click="activeTab = 'specs'"
      />
    </div>

    <!-- 4. Git Worktree & Dependency DAG Telemetry Bar -->
    <div v-if="currentTask" class="px-4 py-1.5 bg-slate-900/40 border-b border-slate-800/90 flex items-center justify-between gap-4 text-xs font-mono flex-shrink-0">
      <div class="flex items-center gap-3 truncate">
        <!-- Worktree Telemetry -->
        <div class="flex items-center gap-1.5 text-teal-300">
          <GitBranch class="w-3.5 h-3.5 text-teal-400" />
          <span class="text-slate-400">Branch:</span>
          <span class="font-semibold">{{ worktreeInfo?.branch || currentTask.metadata?.worktree_branch || `feat/${currentTask.id.toLowerCase()}-worktree` }}</span>
          <span class="text-[10px] px-1.5 py-0.2 rounded bg-teal-950/80 border border-teal-700/80 text-teal-300">Isolated Worktree</span>
        </div>

        <span class="text-slate-700">|</span>

        <!-- Dependency DAG Status -->
        <div class="flex items-center gap-1.5 truncate">
          <Link2 class="w-3.5 h-3.5 flex-shrink-0" :class="dependencyInfo?.all_satisfied !== false ? 'text-emerald-400' : 'text-orange-400'" />
          <span class="text-slate-400 flex-shrink-0">Prerequisites:</span>
          <template v-if="currentTask.dependencies && currentTask.dependencies.length > 0">
            <span
              v-for="dep in dependencyInfo?.details || currentTask.dependencies.map(d => ({ id: d, title: d, state: 'RUNNING', current_stage_id: 'task_implementation' }))"
              :key="dep.id"
              class="px-1.5 py-0.5 rounded text-[10px] flex items-center gap-1 border flex-shrink-0 font-mono"
              :class="dep.state === 'COMPLETED' ? 'bg-emerald-950/70 border-emerald-700 text-emerald-300' : 'bg-orange-950/70 border-orange-700 text-orange-300'"
            >
              <span>{{ dep.id }}</span>
              <span class="text-[9px] opacity-80">({{ dep.state }})</span>
            </span>
          </template>
          <template v-else>
            <span class="text-slate-400 text-[11px]">None (Root Task — Ready for Immediate Execution)</span>
          </template>
        </div>
      </div>

      <div class="flex items-center gap-2 flex-shrink-0">
        <span
          v-if="currentTask.state === 'WAITING_DEPENDENCY'"
          class="px-2 py-0.5 rounded bg-orange-950/90 border border-orange-600/80 text-orange-300 text-[10px] flex items-center gap-1"
        >
          <Lock class="w-3 h-3 text-orange-400" />
          <span>Held: Waiting for Prerequisite</span>
        </span>
        <span
          v-else-if="currentTask.dependencies && currentTask.dependencies.length > 0"
          class="px-2 py-0.5 rounded bg-emerald-950/90 border border-emerald-600/80 text-emerald-300 text-[10px] flex items-center gap-1"
        >
          <ShieldCheck class="w-3 h-3 text-emerald-400" />
          <span>Prerequisites Satisfied</span>
        </span>
      </div>
    </div>

    <!-- 5. Focused Main View Navigation Tabs Bar -->
    <div class="h-10 px-4 bg-slate-900 border-b border-slate-800 flex items-center justify-between flex-shrink-0">
      <div class="flex items-center gap-1">
        <!-- Tab 1: Agent Console (structured activity transcript) -->
        <button
          @click="activeTab = 'terminal'"
          class="h-8 px-3.5 rounded text-xs font-mono transition-all flex items-center gap-2 border"
          :class="activeTab === 'terminal' ? 'bg-slate-800 border-slate-700 text-emerald-400 font-bold shadow-sm' : 'border-transparent text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'"
        >
          <Terminal class="w-4 h-4" />
          <span>Agent Console</span>
          <span
            v-if="taskProcess?.status === 'RUNNING'"
            class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"
          ></span>
        </button>

        <!-- Tab 2: Thought Stream -->
        <button
          @click="activeTab = 'thoughts'"
          class="h-8 px-3.5 rounded text-xs font-mono transition-all flex items-center gap-2 border"
          :class="activeTab === 'thoughts' ? 'bg-slate-800 border-slate-700 text-sky-400 font-bold shadow-sm' : 'border-transparent text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'"
        >
          <BrainCircuit class="w-4 h-4" />
          <span>AI Thought Stream</span>
        </button>

        <!-- Tab 3: Task Specification & Scope -->
        <button
          @click="activeTab = 'specs'"
          class="h-8 px-3.5 rounded text-xs font-mono transition-all flex items-center gap-2 border"
          :class="activeTab === 'specs' ? 'bg-slate-800 border-slate-700 text-purple-400 font-bold shadow-sm' : 'border-transparent text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'"
        >
          <FileText class="w-4 h-4" />
          <span>Task Scope & Requirements</span>
        </button>

        <!-- Tab 4: Workspace Graph -->
        <button
          @click="activeTab = 'workspace'"
          class="h-8 px-3.5 rounded text-xs font-mono transition-all flex items-center gap-2 border"
          :class="activeTab === 'workspace' ? 'bg-slate-800 border-slate-700 text-teal-400 font-bold shadow-sm' : 'border-transparent text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'"
        >
          <Network class="w-4 h-4" />
          <span>Workspace & Repos</span>
        </button>
      </div>

      <div class="text-[11px] font-mono text-slate-400 flex items-center gap-2">
        <span class="text-slate-500">Container:</span>
        <code class="px-1.5 py-0.5 bg-slate-950 rounded border border-slate-800 text-slate-300">/workspaces/{{ taskId }}</code>
      </div>
    </div>

    <!-- 6. Full-Width Focused Tab Content -->
    <main class="flex-1 overflow-hidden flex flex-col min-h-0 bg-slate-950">
      <!-- TAB 1: Agent Console -->
      <div v-if="activeTab === 'terminal'" class="h-full flex flex-col min-h-0">
        <AgentConsole
          :task-id="taskId"
          :show-header="true"
          @task-updated="refreshTaskData"
        />
      </div>

      <!-- TAB 2: AI Thought Stream -->
      <div v-else-if="activeTab === 'thoughts'" class="h-full overflow-y-auto p-4 max-w-5xl mx-auto w-full">
        <ThoughtFeed
          :thoughts="terminalStore.thoughts"
          :task="currentTask || undefined"
        />
      </div>

      <!-- TAB 3: Task Specification & Scope -->
      <div v-else-if="activeTab === 'specs'" class="h-full overflow-y-auto flex flex-col justify-between">
        <div class="p-6 max-w-5xl mx-auto w-full space-y-6">
          <!-- Overview Spec Card -->
          <div class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-4 shadow-lg">
            <div class="flex items-start justify-between gap-4">
              <div class="space-y-1">
                <div class="flex items-center gap-2">
                  <span class="px-2 py-0.5 rounded bg-emerald-950/80 border border-emerald-700/80 text-emerald-300 text-xs font-mono font-bold">
                    {{ currentTask?.id }}
                  </span>
                  <span class="text-xs font-mono text-slate-400">Stage:</span>
                  <span class="px-2 py-0.5 rounded bg-slate-800 text-xs font-mono text-slate-200">
                    {{ currentTask?.current_stage_id || 'prd_discovery' }}
                  </span>
                </div>
                <h3 class="text-base font-semibold text-slate-100 pt-1">
                  {{ currentTask?.title }}
                </h3>
              </div>

              <PhaseStatusBadge v-if="currentTask" :state="currentTask.state" />
            </div>

            <!-- Task Metadata Grid -->
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 pt-3 border-t border-slate-800 text-xs font-mono">
              <div class="p-2.5 rounded bg-slate-950 border border-slate-800">
                <div class="text-[10px] text-slate-500 mb-1">AI ROUTING METHOD</div>
                <div class="text-slate-200 font-semibold">{{ currentTask?.selected_method || 'BMAD' }}</div>
              </div>

              <div class="p-2.5 rounded bg-slate-950 border border-slate-800">
                <div class="text-[10px] text-slate-500 mb-1">ACTIVE MODEL</div>
                <div class="text-purple-300 font-semibold truncate">{{ currentTask?.metadata?.active_model || '9router-auto' }}</div>
              </div>

              <div class="p-2.5 rounded bg-slate-950 border border-slate-800">
                <div class="text-[10px] text-slate-500 mb-1">TOKEN BURN</div>
                <div class="text-amber-300 font-semibold">{{ currentTask?.token_usage?.total_tokens ? currentTask.token_usage.total_tokens.toLocaleString() + ' tokens' : '0 tokens' }}</div>
              </div>

              <div class="p-2.5 rounded bg-slate-950 border border-slate-800">
                <div class="text-[10px] text-slate-500 mb-1">PARALLEL WORKTREE</div>
                <div class="text-teal-300 font-semibold truncate">{{ worktreeInfo?.branch || `feat/${currentTask?.id.toLowerCase()}-worktree` }}</div>
              </div>
            </div>
          </div>

          <!-- Description & Scope Markdown Card -->
          <div class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-3 shadow-lg">
            <h4 class="text-xs font-bold font-mono uppercase tracking-wider text-slate-400 flex items-center gap-2">
              <FileText class="w-4 h-4 text-purple-400" />
              <span>Specification & Problem Statement</span>
            </h4>

            <div
              v-if="parsedDescription"
              class="prose prose-invert prose-sm max-w-none text-slate-300 text-sm leading-relaxed"
              v-html="parsedDescription"
            ></div>
            <div v-else class="text-xs text-slate-500 italic py-4">
              No detailed description provided for this task. Use the guidance dock below to supply implementation context.
            </div>
          </div>

          <!-- Assigned Repositories Card -->
          <div class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-3 shadow-lg">
            <h4 class="text-xs font-bold font-mono uppercase tracking-wider text-slate-400 flex items-center gap-2">
              <Folder class="w-4 h-4 text-teal-400" />
              <span>Assigned Codebase Repositories</span>
            </h4>

            <div class="flex flex-wrap gap-2 pt-1">
              <template v-if="currentTask?.assigned_repos && currentTask.assigned_repos.length > 0">
                <div
                  v-for="repo in currentTask.assigned_repos"
                  :key="repo"
                  class="px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-200 flex items-center gap-2"
                >
                  <span class="w-2 h-2 rounded-full bg-teal-400"></span>
                  <span class="font-semibold">{{ repo }}</span>
                  <span class="text-slate-500 text-[10px]">(/workspaces/{{ taskId }}/{{ repo }})</span>
                </div>
              </template>
              <div v-else class="text-xs text-slate-500 italic">
                Inherited from global root repository.
              </div>
            </div>
          </div>

          <!-- Dependencies & Prerequisites DAG Card -->
          <div class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-3 shadow-lg">
            <h4 class="text-xs font-bold font-mono uppercase tracking-wider text-slate-400 flex items-center gap-2">
              <Link2 class="w-4 h-4 text-emerald-400" />
              <span>Execution Prerequisites & DAG Dependencies</span>
            </h4>

            <div v-if="currentTask?.dependencies && currentTask.dependencies.length > 0" class="space-y-2 pt-1">
              <div
                v-for="dep in dependencyInfo?.details || currentTask.dependencies.map(d => ({ id: d, title: d, state: 'RUNNING', current_stage_id: 'task_implementation' }))"
                :key="dep.id"
                class="p-3 rounded-lg bg-slate-950 border border-slate-800 flex items-center justify-between text-xs font-mono"
              >
                <div class="flex items-center gap-3">
                  <span
                    class="w-2.5 h-2.5 rounded-full"
                    :class="dep.state === 'COMPLETED' ? 'bg-emerald-400' : 'bg-orange-400 animate-pulse'"
                  ></span>
                  <div>
                    <span class="font-bold text-slate-200">{{ dep.id }}</span>
                    <span class="text-slate-400 ml-2">{{ dep.title }}</span>
                  </div>
                </div>

                <div class="flex items-center gap-2">
                  <span class="text-[10px] text-slate-500">Stage: {{ dep.current_stage_id }}</span>
                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-semibold border"
                    :class="dep.state === 'COMPLETED' ? 'bg-emerald-950/80 border-emerald-700 text-emerald-300' : 'bg-orange-950/80 border-orange-700 text-orange-300'"
                  >
                    {{ dep.state }}
                  </span>
                </div>
              </div>
            </div>
            <div v-else class="p-3 rounded-lg bg-slate-950/60 border border-slate-800/80 text-xs text-slate-400 flex items-center gap-2">
              <CheckCircle2 class="w-4 h-4 text-emerald-400" />
              <span>Root task with zero prerequisite blockers. Can run concurrently with other independent tasks.</span>
            </div>
          </div>
        </div>

        <!-- Sticky Operator Steering Guidance Dock -->
        <div class="max-w-5xl mx-auto w-full p-4 border-t border-slate-800/80 bg-slate-900/60">
          <ContextInput
            @send="handleInjectGuidance"
            @reset="showResetModal = true"
          />
        </div>
      </div>

      <!-- TAB 4: Workspace & Repos Graph -->
      <div v-else-if="activeTab === 'workspace'" class="h-full flex flex-col min-h-0">
        <WorkspaceGraph
          :task-id="taskId"
          :assigned-repos="currentTask?.assigned_repos"
          :current-stage-id="currentTask?.current_stage_id"
          :is-write-locked="currentTask?.current_stage_id === 'atdd_creation' || currentTask?.metadata?.write_lock === 'ACTIVE'"
        />
      </div>
    </main>

    <!-- Reset Workspace Confirmation Modal -->
    <ResetWorkspaceModal
      v-if="showResetModal"
      :task-id="taskId"
      @close="showResetModal = false"
      @confirm="handleResetWorkspace"
    />
  </div>
</template>
