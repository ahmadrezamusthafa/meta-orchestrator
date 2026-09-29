<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { Task, WorkflowStage } from '../../types'
import { useTaskStore } from '../../stores/tasks'
import { useToastStore } from '../../stores/toast'
import { getStageGuidance } from '../../composables/useStageGuidance'
import RoutingExplainerPill from '../common/RoutingExplainerPill.vue'
import { 
  Layers, Flame, Clock, Lock, AlertTriangle, ArrowRight, 
  ExternalLink, FileText, User, MoreVertical, Check, Copy, Terminal,
  GitBranch, Link2, Play, Trash2
} from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  task: Task
  density?: 'comfortable' | 'compact'
  allStages?: WorkflowStage[]
}>(), {
  density: 'comfortable',
  allStages: () => []
})

const emit = defineEmits<{
  (e: 'open-console', taskId: string): void
  (e: 'request-delete', taskId: string): void
}>()

function requestDelete(e: Event) {
  e.stopPropagation()
  showStageMenu.value = false
  emit('request-delete', props.task.id)
}

const router = useRouter()
const taskStore = useTaskStore()
const toastStore = useToastStore()

const isDragging = ref(false)
const copiedId = ref(false)
const showStageMenu = ref(false)
const isExecutingAI = ref(false)

async function handleRunAI() {
  if (isExecutingAI.value) return
  isExecutingAI.value = true
  try {
    // 1. Immediately open console drawer so user sees live terminal & AI thoughts
    emit('open-console', props.task.id)
    
    // 2. If suspended or blocked, resume session; otherwise dispatch execution
    if (props.task.state === 'SUSPENDED' || props.task.state === 'BLOCKED_FRUSTRATION') {
      await taskStore.resumeTask(props.task.id)
    } else {
      await taskStore.executeTask(props.task.id)
    }
  } catch (err: any) {
    console.error('Run/Resume AI error:', err)
  } finally {
    setTimeout(() => {
      isExecutingAI.value = false
    }, 1500)
  }
}

const guidance = computed(() => getStageGuidance(props.task, props.allStages))
const isBlocked = computed(() => props.task.state === 'BLOCKED_FRUSTRATION')
const isWaitingDependency = computed(() => props.task.state === 'WAITING_DEPENDENCY')
const isWorktree = computed(() => props.task.metadata?.worktree_enabled === 'true')
const worktreeBranch = computed(() => props.task.metadata?.worktree_branch || `feat/${props.task.id.toLowerCase()}-worktree`)
const unmetDependencies = computed(() => {
  if (props.task.metadata?.unmet_dependencies) {
    return props.task.metadata.unmet_dependencies.split(',').filter(Boolean)
  }
  return props.task.dependencies || []
})

const isWriteLocked = computed(() => {
  return props.task.current_stage_id === 'atdd_creation' || props.task.metadata?.write_lock === 'ACTIVE'
})

const currentStageIndex = computed(() => {
  if (!props.allStages || props.allStages.length === 0) return -1
  return props.allStages.findIndex(s => s.id === props.task.current_stage_id)
})

const nextStage = computed<WorkflowStage | null>(() => {
  if (currentStageIndex.value === -1 || !props.allStages || props.allStages.length === 0) return null
  if (currentStageIndex.value < props.allStages.length - 1) {
    return props.allStages[currentStageIndex.value + 1]
  }
  return null
})

const jiraKey = computed(() => {
  if (props.task.metadata?.jira_key) return props.task.metadata.jira_key
  const match = props.task.title.match(/\b([A-Z]{2,10}-\d+)\b/)
  return match ? match[1] : null
})

const jiraUrl = computed(() => {
  if (props.task.metadata?.jira_url) return props.task.metadata.jira_url
  if (jiraKey.value) return `https://jira.atlassian.net/browse/${jiraKey.value}`
  return '#'
})

const epicLabel = computed(() => {
  const key = props.task.metadata?.jira_epic_key
  if (!key || key === props.task.metadata?.jira_key) return null
  return props.task.metadata?.jira_epic_name || key
})

const confluenceUrl = computed(() => {
  return props.task.metadata?.confluence_page_url || null
})

const timeElapsed = computed(() => {
  const created = new Date(props.task.created_at).getTime()
  const now = new Date().getTime()
  const diffMins = Math.floor((now - created) / 60000)
  if (diffMins < 60) return `${diffMins}m`
  const hours = Math.floor(diffMins / 60)
  return `${hours}h ${diffMins % 60}m`
})

function navigateToTask() {
  router.push(`/tasks/${props.task.id}`)
}

function handleDragStart(e: DragEvent) {
  isDragging.value = true
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', props.task.id)
  }
}

function handleDragEnd() {
  isDragging.value = false
}

async function copyTaskId(e: Event) {
  e.stopPropagation()
  try {
    await navigator.clipboard.writeText(props.task.id)
    copiedId.value = true
    setTimeout(() => {
      copiedId.value = false
    }, 1500)
  } catch (err) {
    console.debug('Failed to copy ID', err)
  }
}

function moveToNextStage(e: Event) {
  e.stopPropagation()
  if (!nextStage.value) return
  taskStore.moveTaskToStage(props.task.id, nextStage.value.id)
  toastStore.success('Card Moved', `Moved ${props.task.id} to "${nextStage.value.name}" column`)
}
const advanceStage = moveToNextStage

function moveToStage(stageId: string, e: Event) {
  e.stopPropagation()
  const target = props.allStages?.find(s => s.id === stageId)
  taskStore.moveTaskToStage(props.task.id, stageId)
  toastStore.info('Stage Updated', `Moved ${props.task.id} to "${target?.name || stageId}" column`)
  showStageMenu.value = false
}
</script>

<template>
  <div
    draggable="true"
    @dragstart="handleDragStart"
    @dragend="handleDragEnd"
    @click="navigateToTask"
    class="rounded-lg bg-slate-900/90 border border-slate-800 hover:border-slate-700 hover:-translate-y-0.5 transition-all duration-150 cursor-grab active:cursor-grabbing shadow-sm relative group flex flex-col justify-between select-none"
    :class="[
      density === 'compact' ? 'p-2.5' : 'p-3.5',
      {
        'opacity-40 scale-95 border-emerald-500/80': isDragging,
        'border-l-4 !border-l-rose-500 shadow-rose-950/20 animate-pulse-subtle': isBlocked,
        'border-l-4 !border-l-orange-500 shadow-orange-950/20': isWaitingDependency,
        'border-l-4 !border-l-amber-500': isWriteLocked && !isBlocked && !isWaitingDependency,
        'border-l-4 !border-l-emerald-500': !isWriteLocked && !isBlocked && !isWaitingDependency,
      }
    ]"
  >
    <!-- ================= COMPACT VIEW ================= -->
    <div v-if="density === 'compact'" class="space-y-1.5">
      <!-- Row 1: ID, Title & Quick Actions -->
      <div class="flex items-center justify-between gap-1.5">
        <div class="flex items-center gap-1.5 truncate flex-1 min-w-0">
          <!-- Status dot -->
          <span
            class="w-2 h-2 rounded-full flex-shrink-0"
            :class="{
              'bg-rose-500 animate-pulse': isBlocked,
              'bg-orange-500 animate-pulse': isWaitingDependency,
              'bg-amber-400': isWriteLocked && !isBlocked && !isWaitingDependency,
              'bg-emerald-400': !isWriteLocked && !isBlocked && !isWaitingDependency
            }"
          ></span>

          <!-- Task ID -->
          <button
            @click="copyTaskId"
            type="button"
            title="Click to copy task ID"
            class="font-mono text-[11px] font-semibold text-slate-400 hover:text-emerald-400 transition-colors flex-shrink-0 cursor-copy"
          >
            {{ copiedId ? '✓ Copied' : task.id }}
          </button>

          <h4 class="text-xs font-medium text-slate-200 truncate group-hover:text-white transition-colors" :title="task.title">
            {{ task.title }}
          </h4>
        </div>

        <!-- Row 1 Right: Single Console Button, Action & Menu -->
        <div class="flex items-center gap-1 flex-shrink-0" @click.stop>
          <!-- Compact Run AI Button -->
          <button
            @click.stop="handleRunAI"
            :disabled="isExecutingAI || isWaitingDependency || task.state === 'COMPLETED'"
            type="button"
            :title="isWaitingDependency ? `Blocked: waiting on ${unmetDependencies.join(', ')}` : 'Trigger 9Router AI execution for current stage and open live console'"
            class="h-6 px-1.5 rounded text-[10px] font-mono flex items-center gap-1 transition-all"
            :class="task.state === 'RUNNING' || isExecutingAI
              ? 'bg-emerald-950/80 border border-emerald-500/80 text-emerald-300'
              : isWaitingDependency
                ? 'bg-slate-950 border border-slate-800 text-slate-500 cursor-not-allowed'
                : 'bg-emerald-950/70 hover:bg-emerald-900 border border-emerald-700/80 text-emerald-300 hover:text-white'"
          >
            <span v-if="isExecutingAI" class="animate-spin text-emerald-400">⟳</span>
            <Play v-else class="w-2.5 h-2.5 fill-current text-emerald-400" />
            <span class="hidden sm:inline">{{ isExecutingAI ? '...' : task.state === 'RUNNING' ? 'Running' : (task.state === 'SUSPENDED' || task.state === 'BLOCKED_FRUSTRATION') ? 'Resume' : 'Run' }}</span>
          </button>

          <!-- Single Canonical Console Button -->
          <button
            @click="emit('open-console', task.id)"
            type="button"
            title="View background process & console terminal"
            class="h-6 px-1.5 rounded bg-slate-950 hover:bg-slate-800 border border-slate-800 hover:border-emerald-500/80 text-slate-300 hover:text-emerald-300 text-[10px] font-mono flex items-center gap-1 transition-all"
          >
            <Terminal class="w-3 h-3 text-emerald-400" />
            <span v-if="task.state === 'RUNNING'" class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
          </button>

          <!-- Contextual Action Button (Single non-duplicate) -->
          <button
            v-if="guidance.actionType === 'gate_approval'"
            @click.stop="navigateToTask"
            type="button"
            title="Pipeline paused: Review gate approval"
            class="h-6 px-2 rounded bg-amber-950 hover:bg-amber-800 border border-amber-600/80 text-amber-200 text-[10px] font-mono font-medium flex items-center gap-1 transition-all shadow-sm"
          >
            <span>Gate</span>
            <ArrowRight class="w-2.5 h-2.5" />
          </button>

          <button
            v-else-if="guidance.actionType === 'blocked_steer'"
            @click.stop="navigateToTask"
            type="button"
            title="Execution blocked: click to steer task"
            class="h-6 px-2 rounded bg-rose-950 hover:bg-rose-800 border border-rose-600/80 text-rose-200 text-[10px] font-mono font-medium flex items-center gap-1 transition-all shadow-sm"
          >
            <AlertTriangle class="w-2.5 h-2.5" />
            <span>Steer</span>
          </button>

          <span
            v-else-if="isWaitingDependency"
            :title="`Held in queue: Awaiting prerequisite (${unmetDependencies.join(', ')}) to complete`"
            class="h-6 px-2 rounded bg-orange-950/90 border border-orange-600/80 text-orange-300 text-[10px] font-mono font-medium flex items-center gap-1 shadow-sm"
          >
            <Lock class="w-2.5 h-2.5 text-orange-400" />
            <span>Dep</span>
          </span>

          <button
            v-else-if="nextStage"
            @click="moveToNextStage"
            type="button"
            :title="`Move card to next column: ${nextStage.name}`"
            class="h-6 px-1.5 rounded bg-emerald-950/80 hover:bg-emerald-800 border border-emerald-700/80 text-emerald-300 text-[10px] font-mono flex items-center gap-1 transition-all shadow-sm"
          >
            <span>Move</span>
            <ArrowRight class="w-2.5 h-2.5" />
          </button>

          <div class="relative">
            <button
              @click="showStageMenu = !showStageMenu"
              type="button"
              title="Move card to any column..."
              class="h-6 w-6 rounded flex items-center justify-center text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
            >
              <MoreVertical class="w-3 h-3" />
            </button>

            <div
              v-if="showStageMenu"
              class="absolute right-0 top-full mt-1 w-44 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl z-30 py-1 text-xs"
            >
              <div class="px-2.5 py-1 text-[10px] font-mono text-slate-400 uppercase tracking-wider border-b border-slate-800">
                Move to column
              </div>
              <div class="max-h-44 overflow-y-auto">
                <button
                  v-for="s in allStages"
                  :key="s.id"
                  @click="moveToStage(s.id, $event)"
                  type="button"
                  class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 flex items-center justify-between text-[11px]"
                  :class="s.id === task.current_stage_id ? 'text-emerald-400 font-semibold' : 'text-slate-300'"
                >
                  <span class="truncate">{{ s.name }}</span>
                  <Check v-if="s.id === task.current_stage_id" class="w-3 h-3 text-emerald-400 flex-shrink-0" />
                </button>
              </div>
              <button
                @click="requestDelete"
                type="button"
                class="w-full text-left px-2.5 py-1.5 border-t border-slate-800 hover:bg-rose-950/60 text-rose-300 flex items-center gap-1.5 text-[11px]"
              >
                <Trash2 class="w-3 h-3" />
                <span>Remove from board</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Row 2: Metadata chips -->
      <div class="flex items-center justify-between text-[10px] font-mono text-slate-400 pt-1 border-t border-slate-800/60">
        <div class="flex items-center gap-1.5 truncate">
          <span class="px-1.5 py-0.2 rounded bg-slate-950 border border-slate-800 text-slate-300">{{ task.selected_method }}</span>
          <span v-if="jiraKey" class="text-blue-400 truncate">{{ jiraKey }}</span>
          <span v-if="isWorktree" class="text-teal-400 flex items-center gap-0.5" :title="`Git Worktree: ${worktreeBranch}`">
            <GitBranch class="w-2.5 h-2.5" />
            <span class="text-[9px]">wt</span>
          </span>
          <span v-if="task.dependencies && task.dependencies.length > 0" class="flex items-center gap-0.5 truncate" :class="isWaitingDependency ? 'text-orange-400 font-semibold' : 'text-slate-400'" :title="`Prerequisites: ${task.dependencies.join(', ')}`">
            <Link2 class="w-2.5 h-2.5" />
            <span>{{ task.dependencies.length }}</span>
          </span>
        </div>
        <span class="text-slate-500">{{ timeElapsed }}</span>
      </div>
    </div>

    <!-- ================= COMFORTABLE VIEW ================= -->
    <div v-else>
      <div>
        <!-- Card Header: Task ID, Tag & Single Canonical Console + Move Menu -->
        <div class="flex items-center justify-between gap-2 mb-2">
          <!-- Left: Task ID & Primary status badge -->
          <div class="flex items-center gap-1.5 min-w-0 flex-wrap">
            <button
              @click="copyTaskId"
              type="button"
              title="Click to copy task ID"
              class="font-mono text-xs font-semibold text-slate-400 group-hover:text-emerald-400 transition-colors flex items-center gap-1 cursor-copy"
            >
              <span>{{ copiedId ? '✓ Copied!' : task.id }}</span>
            </button>

            <!-- Key status badge (Dep Held, Worktree, Blocked, or Locked) -->
            <span
              v-if="isWaitingDependency"
              :title="`Held in DAG Queue: Waiting for prerequisite (${unmetDependencies.join(', ')}) to finish`"
              class="px-1.5 py-0.5 rounded bg-orange-950/80 border border-orange-700/80 text-orange-300 text-[10px] font-mono flex items-center gap-1"
            >
              <Lock class="w-2.5 h-2.5 text-orange-400" />
              <span>Dep Held</span>
            </span>

            <span
              v-else-if="isWorktree"
              :title="`Isolated Git Worktree: ${worktreeBranch}`"
              class="px-1.5 py-0.5 rounded bg-teal-950/80 border border-teal-700/80 text-teal-300 text-[10px] font-mono flex items-center gap-1"
            >
              <GitBranch class="w-2.5 h-2.5 text-teal-400" />
              <span>Worktree</span>
            </span>

            <span
              v-else-if="isBlocked"
              title="Blocked in circuit breaker frustration loop"
              class="px-1.5 py-0.5 rounded bg-rose-950/80 border border-rose-700/80 text-rose-300 text-[10px] font-mono flex items-center gap-1"
            >
              <AlertTriangle class="w-2.5 h-2.5 text-rose-400" />
              <span>Blocked</span>
            </span>

            <span
              v-else-if="isWriteLocked"
              title="Source write-locking active (Red Phase)"
              class="px-1.5 py-0.5 rounded bg-amber-950/80 border border-amber-800/80 text-amber-300 text-[10px] font-mono flex items-center gap-1"
            >
              <Lock class="w-2.5 h-2.5 text-amber-400" />
              <span>Locked</span>
            </span>
          </div>

          <!-- Right: Canonical Run AI & Console Buttons & Quick Move Menu -->
          <div class="flex items-center gap-1.5 flex-shrink-0" @click.stop>
            <!-- 1-Click Run AI Button with Automatic Console Auto-Open -->
            <button
              @click="handleRunAI"
              :disabled="isExecutingAI || isWaitingDependency || task.state === 'COMPLETED'"
              type="button"
              :title="isWaitingDependency ? `Blocked: waiting on ${unmetDependencies.join(', ')}` : 'Trigger 9Router AI execution for current stage and open live console'"
              class="h-6 px-2 rounded text-[10px] font-mono flex items-center gap-1.5 transition-all shadow-sm"
              :class="task.state === 'RUNNING' || isExecutingAI
                ? 'bg-emerald-950/80 border border-emerald-500/80 text-emerald-300'
                : isWaitingDependency
                  ? 'bg-slate-950 border border-slate-800 text-slate-500 cursor-not-allowed'
                  : 'bg-emerald-950/80 hover:bg-emerald-900 border border-emerald-700/80 text-emerald-300 hover:text-white'"
            >
              <span v-if="isExecutingAI" class="animate-spin text-emerald-400">⟳</span>
              <Play v-else class="w-2.5 h-2.5 fill-current text-emerald-400" />
              <span>{{ isExecutingAI ? 'Routing...' : task.state === 'RUNNING' ? 'Running' : (task.state === 'SUSPENDED' || task.state === 'BLOCKED_FRUSTRATION') ? 'Resume' : 'Run AI' }}</span>
            </button>

            <!-- Single, Canonical 1-Click Console Terminal Button -->
            <button
              @click="emit('open-console', task.id)"
              type="button"
              title="View background process & console terminal"
              class="h-6 px-2 rounded bg-slate-950 border border-slate-800 hover:border-emerald-500/80 hover:bg-slate-900 text-slate-300 hover:text-emerald-300 text-[10px] font-mono flex items-center gap-1.5 transition-all shadow-sm group/console"
            >
              <Terminal class="w-3 h-3 text-emerald-400 group-hover/console:text-emerald-300" />
              <span>Console</span>
              <span v-if="task.state === 'RUNNING'" class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
            </button>

            <!-- Quick Stage Move Dropdown Menu -->
            <div class="relative">
              <button
                @click="showStageMenu = !showStageMenu"
                type="button"
                title="Move card to column..."
                class="h-6 w-6 rounded flex items-center justify-center text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
              >
                <MoreVertical class="w-3.5 h-3.5" />
              </button>

              <div
                v-if="showStageMenu"
                class="absolute right-0 top-full mt-1 w-48 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl z-30 py-1 text-xs"
              >
                <div class="px-3 py-1 text-[10px] font-mono text-slate-400 uppercase tracking-wider border-b border-slate-800">
                  Move to column
                </div>
                <div class="max-h-48 overflow-y-auto">
                  <button
                    v-for="s in allStages"
                    :key="s.id"
                    @click="moveToStage(s.id, $event)"
                    type="button"
                    class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center justify-between text-[11px]"
                    :class="s.id === task.current_stage_id ? 'text-emerald-400 font-semibold' : 'text-slate-300'"
                  >
                    <span class="truncate">{{ s.name }}</span>
                    <Check v-if="s.id === task.current_stage_id" class="w-3 h-3 text-emerald-400 flex-shrink-0" />
                  </button>
                </div>
                <button
                  @click="requestDelete"
                  type="button"
                  class="w-full text-left px-3 py-1.5 border-t border-slate-800 hover:bg-rose-950/60 text-rose-300 flex items-center gap-1.5 text-[11px]"
                >
                  <Trash2 class="w-3 h-3" />
                  <span>Remove from board</span>
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Task Title -->
        <h4 class="text-xs font-medium text-slate-200 line-clamp-2 leading-relaxed mb-2.5 group-hover:text-white transition-colors">
          {{ task.title }}
        </h4>

        <!-- External Connector Badges & Router Info (Wrapped without clipping) -->
        <div class="flex flex-wrap items-center gap-1.5 mb-2.5">
          <a
            v-if="jiraKey"
            :href="jiraUrl"
            target="_blank"
            @click.stop
            title="Open linked JIRA ticket in new tab"
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-blue-950/80 text-blue-300 border border-blue-800/80 hover:bg-blue-900 hover:text-blue-100 hover:border-blue-600 transition-colors text-[10px] font-mono font-medium shadow-sm group/jira"
          >
            <span class="w-1.5 h-1.5 rounded-full bg-blue-400"></span>
            <span>{{ jiraKey }}</span>
            <ExternalLink class="w-2.5 h-2.5 opacity-70 group-hover/jira:opacity-100" />
          </a>

          <span
            v-if="epicLabel && taskStore.groupBy !== 'epic'"
            :title="`Epic ${task.metadata?.jira_epic_key}`"
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-violet-950/70 text-violet-300 border border-violet-800/70 text-[10px] font-medium max-w-[160px]"
          >
            <Layers class="w-2.5 h-2.5 flex-shrink-0" />
            <span class="truncate">{{ epicLabel }}</span>
          </span>

          <a
            v-if="confluenceUrl"
            :href="confluenceUrl"
            target="_blank"
            @click.stop
            title="Open published Technical RFC on Confluence"
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-indigo-950/80 text-indigo-300 border border-indigo-800/80 hover:bg-indigo-900 hover:text-indigo-100 hover:border-indigo-600 transition-colors text-[10px] font-mono font-medium shadow-sm group/conf"
          >
            <FileText class="w-2.5 h-2.5 text-indigo-400" />
            <span>Confluence RFC</span>
            <ExternalLink class="w-2.5 h-2.5 opacity-70 group-hover/conf:opacity-100" />
          </a>

          <span
            v-if="task.metadata?.assignee"
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-slate-950 text-slate-300 border border-slate-800 text-[10px] font-mono"
            title="Assigned to"
          >
            <User class="w-2.5 h-2.5 text-blue-400" />
            <span>{{ task.metadata.assignee }}</span>
          </span>

          <RoutingExplainerPill
            :source="task.metadata?.router_source"
            :rationale="task.metadata?.router_rationale"
          />

          <!-- Worktree branch chip if both worktree & dependency hold are active -->
          <span
            v-if="isWorktree && isWaitingDependency"
            :title="`Isolated Git Worktree: ${worktreeBranch}`"
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-teal-950/60 border border-teal-800/60 text-teal-300 text-[10px] font-mono"
          >
            <GitBranch class="w-2.5 h-2.5 text-teal-400" />
            <span class="truncate max-w-[120px]">{{ worktreeBranch }}</span>
          </span>
        </div>

        <!-- Single Unified Execution & Progress Status Bar -->
        <div class="mb-3 p-2.5 rounded-lg bg-slate-950/90 border border-slate-800/80 space-y-2 text-[10px] font-mono">
          <!-- Top Row: Stage Step & State Indicator -->
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-1.5 truncate flex-1 min-w-0">
              <span
                class="w-2 h-2 rounded-full flex-shrink-0"
                :class="{
                  'bg-emerald-400 animate-pulse': task.state === 'RUNNING',
                  'bg-rose-500 animate-pulse': isBlocked,
                  'bg-amber-400': task.state === 'WAITING_GATE_APPROVAL',
                  'bg-orange-500 animate-pulse': isWaitingDependency,
                  'bg-emerald-400': task.state === 'COMPLETED',
                  'bg-slate-500': task.state !== 'RUNNING' && !isBlocked && task.state !== 'WAITING_GATE_APPROVAL' && !isWaitingDependency && task.state !== 'COMPLETED'
                }"
              ></span>
              <span class="text-slate-400 font-medium">Step {{ guidance.stageNumber }}/{{ guidance.totalStages }}:</span>
              <span class="text-slate-200 font-semibold truncate">{{ guidance.stageName }}</span>
            </div>

            <span
              v-if="isWaitingDependency"
              class="text-orange-400 font-medium flex-shrink-0"
            >
              ● Dep Waiting
            </span>
            <span
              v-else-if="guidance.actionType === 'blocked_steer'"
              class="text-rose-400 font-medium flex-shrink-0"
            >
              ● Steer Needed
            </span>
            <span
              v-else-if="guidance.actionType === 'gate_approval'"
              class="text-amber-400 font-medium flex-shrink-0"
            >
              ● Gate Review
            </span>
            <span
              v-else-if="guidance.actionType === 'completed_review'"
              class="text-emerald-400 font-medium flex-shrink-0"
            >
              ● Done
            </span>
            <span
              v-else
              class="text-emerald-400 font-medium flex-shrink-0"
            >
              ● {{ guidance.isWriteLocked ? 'Locked (Red)' : 'Active' }}
            </span>
          </div>

          <!-- Bottom Row: Context Hint & Single Contextual Action Button -->
          <div class="flex items-center justify-between gap-2 pt-1.5 border-t border-slate-800/60">
            <div class="text-slate-400 truncate flex items-center gap-1 flex-1 min-w-0">
              <template v-if="isWaitingDependency">
                <span class="text-orange-400 font-medium">Prereq:</span>
                <span class="text-slate-300 truncate" :title="`Awaiting prerequisite (${unmetDependencies.join(', ')}) to complete`">Waiting on {{ unmetDependencies.join(', ') }}</span>
              </template>
              <template v-else-if="guidance.actionType === 'gate_approval'">
                <span class="text-amber-400 font-medium">Gate:</span>
                <span class="text-slate-300 truncate">Requires human review</span>
              </template>
              <template v-else-if="guidance.actionType === 'blocked_steer'">
                <span class="text-rose-400 font-medium">Status:</span>
                <span class="text-slate-300 truncate">Circuit breaker halted</span>
              </template>
              <template v-else-if="guidance.actionType === 'completed_review'">
                <span class="text-emerald-400 font-medium">Status:</span>
                <span class="text-slate-300 truncate">All stages verified</span>
              </template>
              <template v-else-if="nextStage">
                <span class="text-slate-500">Next Col:</span>
                <span class="text-slate-300 truncate" :title="nextStage.name">{{ nextStage.name }}</span>
              </template>
              <template v-else>
                <span class="text-slate-500">Process:</span>
                <span class="text-slate-300 truncate">{{ task.state === 'RUNNING' ? 'Background active' : 'Ready' }}</span>
              </template>
            </div>

            <!-- Single Action Button (Never duplicated with Console) -->
            <!-- 1. Dependency Held -->
            <button
              v-if="isWaitingDependency"
              type="button"
              :title="`Task held until prerequisite ${unmetDependencies.join(', ')} is COMPLETED`"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-orange-950/90 border border-orange-600/80 text-orange-200 text-[10px] font-mono font-medium flex-shrink-0 cursor-not-allowed"
            >
              <Lock class="w-3 h-3 text-orange-400" />
              <span>Held (Dep)</span>
            </button>

            <!-- 2. Review Gate -->
            <button
              v-else-if="guidance.actionType === 'gate_approval'"
              @click.stop="navigateToTask"
              type="button"
              title="Pipeline paused: Review gate decision and authorize next stage"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-amber-950/90 hover:bg-amber-800 border border-amber-600/80 text-amber-200 hover:text-white text-[10px] font-mono font-medium transition-all shadow-sm flex-shrink-0 group/gate"
            >
              <span>Review Gate</span>
              <ArrowRight class="w-3 h-3 text-amber-400 group-hover/gate:translate-x-0.5 transition-transform" />
            </button>

            <!-- 3. Steer Task (when blocked) -->
            <button
              v-else-if="guidance.actionType === 'blocked_steer'"
              @click.stop="navigateToTask"
              type="button"
              title="Click to view task details, logs and inject operator instructions"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-rose-950/90 hover:bg-rose-800 border border-rose-600/80 text-rose-200 hover:text-white text-[10px] font-mono font-medium transition-all shadow-sm flex-shrink-0 group/steer"
            >
              <AlertTriangle class="w-3 h-3 text-rose-400" />
              <span>Steer Task</span>
              <ArrowRight class="w-3 h-3 text-rose-400 group-hover/steer:translate-x-0.5 transition-transform" />
            </button>

            <!-- 4. Done -->
            <span
              v-else-if="guidance.actionType === 'completed_review'"
              class="inline-flex items-center gap-1 text-[10px] font-mono text-emerald-400 font-medium px-1.5 py-0.5 rounded bg-emerald-950/40 border border-emerald-800/40 flex-shrink-0"
            >
              <Check class="w-3 h-3 text-emerald-400" />
              <span>Done</span>
            </span>

            <!-- 5. 1-Click Move Column -->
            <button
              v-else-if="nextStage"
              @click="moveToNextStage"
              type="button"
              :title="`Move card to next Kanban column: ${nextStage.name} (1-click alternative to dragging)`"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-emerald-950/90 hover:bg-emerald-800 border border-emerald-700/80 text-emerald-300 hover:text-white text-[10px] font-mono font-medium transition-all shadow-sm flex-shrink-0 group/move"
            >
              <span>Move Column</span>
              <ArrowRight class="w-3 h-3 text-emerald-400 group-hover/move:translate-x-0.5 transition-transform" />
            </button>
          </div>
        </div>
      </div>

      <!-- Card Footer -->
      <div class="pt-2.5 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-400">
        <div class="flex items-center gap-2">
          <span
            class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium"
            :class="{
              'bg-emerald-950/60 text-emerald-300 border border-emerald-800/60': task.selected_method === 'BMAD',
              'bg-sky-950/60 text-sky-300 border border-sky-800/60': task.selected_method === 'Supervisor',
              'bg-purple-950/60 text-purple-300 border border-purple-800/60': task.selected_method === 'ReAct',
              'bg-amber-950/60 text-amber-300 border border-amber-800/60': task.selected_method === 'Superpower',
            }"
          >
            {{ task.selected_method }}
          </span>

          <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400" title="Assigned repositories">
            <Layers class="w-3 h-3" />
            <span>{{ task.assigned_repos?.length || 1 }}</span>
          </div>

          <div
            v-if="task.dependencies && task.dependencies.length > 0"
            class="flex items-center gap-1 text-[10px] font-mono"
            :class="isWaitingDependency ? 'text-orange-400 font-medium' : 'text-slate-400'"
            :title="`Prerequisites: ${task.dependencies.join(', ')}`"
          >
            <Link2 class="w-3 h-3 text-orange-400" />
            <span>{{ task.dependencies.length }}</span>
          </div>
        </div>

        <div class="flex items-center gap-2.5">
          <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400" title="Token consumption">
            <Flame class="w-3 h-3 text-amber-500" />
            <span>{{ Math.round((task.token_usage?.total_tokens || 0) / 1000) }}k</span>
          </div>

          <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400" title="Time elapsed">
            <Clock class="w-3 h-3" />
            <span>{{ timeElapsed }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
