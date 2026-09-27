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
  ExternalLink, FileText, User, MoreVertical, Check, Copy, Terminal 
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
}>()

const router = useRouter()
const taskStore = useTaskStore()
const toastStore = useToastStore()

const isDragging = ref(false)
const copiedId = ref(false)
const showStageMenu = ref(false)

const guidance = computed(() => getStageGuidance(props.task, props.allStages))
const isBlocked = computed(() => props.task.state === 'BLOCKED_FRUSTRATION')
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
        'border-l-4 !border-l-amber-500': isWriteLocked && !isBlocked,
        'border-l-4 !border-l-emerald-500': !isWriteLocked && !isBlocked,
      }
    ]"
  >
    <!-- ================= COMPACT VIEW ================= -->
    <div v-if="density === 'compact'" class="space-y-2">
      <div class="flex items-center justify-between gap-1.5">
        <div class="flex items-center gap-1.5 truncate flex-1 min-w-0">
          <!-- Status dot -->
          <span
            class="w-2 h-2 rounded-full flex-shrink-0"
            :class="{
              'bg-rose-500 animate-pulse': isBlocked,
              'bg-amber-400': isWriteLocked && !isBlocked,
              'bg-emerald-400': !isWriteLocked && !isBlocked
            }"
          ></span>

          <!-- Task ID with copy button -->
          <button
            @click="copyTaskId"
            type="button"
            title="Click to copy task ID"
            class="font-mono text-[11px] font-semibold text-slate-400 hover:text-emerald-400 transition-colors flex-shrink-0 cursor-copy"
          >
            {{ copiedId ? '✓ Copied' : task.id }}
          </button>

          <h4 class="text-xs font-medium text-slate-200 truncate group-hover:text-white transition-colors">
            {{ task.title }}
          </h4>
        </div>

        <!-- Column Actions: Console & Move / Gate Actions -->
        <div class="flex items-center gap-1 flex-shrink-0" @click.stop>
          <!-- Console Terminal Button -->
          <button
            @click="emit('open-console', task.id)"
            type="button"
            title="View background process & console terminal"
            class="h-6 px-1.5 rounded bg-slate-950 hover:bg-slate-800 border border-slate-800 hover:border-emerald-500/80 text-slate-300 hover:text-emerald-300 text-[10px] font-mono flex items-center gap-1 transition-all"
          >
            <Terminal class="w-3 h-3 text-emerald-400" />
            <span v-if="task.state === 'RUNNING'" class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
          </button>

          <!-- Contextual Gate Review Action -->
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

          <!-- Contextual Error Inspection Action -->
          <button
            v-else-if="guidance.actionType === 'blocked_steer'"
            @click.stop="emit('open-console', task.id)"
            type="button"
            title="Execution blocked: click to inspect terminal & steer"
            class="h-6 px-2 rounded bg-rose-950 hover:bg-rose-800 border border-rose-600/80 text-rose-200 text-[10px] font-mono font-medium flex items-center gap-1 transition-all shadow-sm"
          >
            <AlertTriangle class="w-2.5 h-2.5" />
            <span>Error</span>
          </button>

          <!-- 1-Click Move to Next Column (Alternative to Drag & Drop) -->
          <button
            v-else-if="nextStage"
            @click="moveToNextStage"
            type="button"
            :title="`Move card to next column: ${nextStage.name} (1-click move)`"
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
            </div>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-between text-[10px] font-mono text-slate-400 pt-1.5 border-t border-slate-800/60">
        <div class="flex items-center gap-1.5 truncate">
          <span class="px-1.5 py-0.2 rounded bg-slate-950 border border-slate-800 text-slate-300">{{ task.selected_method }}</span>
          <span v-if="jiraKey" class="text-blue-400 truncate">{{ jiraKey }}</span>
        </div>
        <span class="text-slate-500">{{ timeElapsed }}</span>
      </div>
    </div>

    <!-- ================= COMFORTABLE VIEW ================= -->
    <div v-else>
      <div>
        <div class="flex items-center justify-between gap-2 mb-2">
          <!-- Copy Task ID on Click -->
          <button
            @click="copyTaskId"
            type="button"
            title="Click to copy task ID"
            class="font-mono text-xs font-semibold text-slate-400 group-hover:text-emerald-400 transition-colors flex items-center gap-1 cursor-copy"
          >
            <span>{{ copiedId ? '✓ Copied!' : task.id }}</span>
          </button>

          <div class="flex items-center gap-1.5">
            <span
              v-if="isWriteLocked"
              title="Source write-locking active (Red Phase)"
              class="p-1 rounded bg-amber-950/60 border border-amber-800/80 text-amber-400"
            >
              <Lock class="w-3 h-3" />
            </span>

            <span
              v-if="isBlocked"
              title="Blocked in frustration loop"
              class="p-1 rounded bg-rose-950/60 border border-rose-800/80 text-rose-400"
            >
              <AlertTriangle class="w-3 h-3" />
            </span>

            <RoutingExplainerPill
              :source="task.metadata?.router_source"
              :rationale="task.metadata?.router_rationale"
            />

            <!-- Dedicated 1-Click Console Terminal Button -->
            <button
              @click.stop="emit('open-console', task.id)"
              type="button"
              title="View background process & console terminal"
              class="h-6 px-2 rounded bg-slate-950 border border-slate-800 hover:border-emerald-500/80 hover:bg-slate-900 text-slate-300 hover:text-emerald-300 text-[10px] font-mono flex items-center gap-1.5 transition-all shadow-sm group/console"
            >
              <Terminal class="w-3 h-3 text-emerald-400 group-hover/console:text-emerald-300" />
              <span>Console</span>
              <span v-if="task.state === 'RUNNING'" class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
            </button>

            <!-- Quick Stage Move Dropdown Menu -->
            <div class="relative" @click.stop>
              <button
                @click="showStageMenu = !showStageMenu"
                type="button"
                title="Move card to column..."
                class="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
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
              </div>
            </div>
          </div>
        </div>

        <h4 class="text-xs font-medium text-slate-200 line-clamp-2 leading-relaxed mb-2">
          {{ task.title }}
        </h4>

        <!-- External Connector Badges (JIRA, Confluence & Assignee) -->
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
        </div>

        <!-- Live Background Process Telemetry Pill -->
        <div
          @click.stop="emit('open-console', task.id)"
          class="mb-2.5 px-2.5 py-1.5 rounded-lg border transition-all cursor-pointer flex items-center justify-between text-[10px] font-mono group/proc shadow-sm"
          :class="{
            'bg-emerald-950/30 border-emerald-800/60 hover:border-emerald-500 text-emerald-300': task.state === 'RUNNING',
            'bg-rose-950/30 border-rose-800/60 hover:border-rose-500 text-rose-300': isBlocked,
            'bg-amber-950/30 border-amber-800/60 hover:border-amber-500 text-amber-300': task.state === 'WAITING_GATE_APPROVAL',
            'bg-slate-950/60 border-slate-800 hover:border-slate-700 text-slate-400': task.state !== 'RUNNING' && !isBlocked && task.state !== 'WAITING_GATE_APPROVAL'
          }"
          title="Click to view live background process terminal"
        >
          <div class="flex items-center gap-2 truncate flex-1 min-w-0">
            <span
              class="w-2 h-2 rounded-full flex-shrink-0"
              :class="{
                'bg-emerald-400 animate-pulse': task.state === 'RUNNING',
                'bg-rose-500 animate-pulse': isBlocked,
                'bg-amber-400': task.state === 'WAITING_GATE_APPROVAL',
                'bg-slate-500': task.state !== 'RUNNING' && !isBlocked && task.state !== 'WAITING_GATE_APPROVAL'
              }"
            ></span>
            <span class="text-slate-400">Process:</span>
            <span class="font-medium text-slate-200 truncate group-hover/proc:text-white">
              {{ task.state === 'RUNNING' ? 'Background Execution Active' : isBlocked ? 'Execution Blocked (Trace Ready)' : task.state === 'WAITING_GATE_APPROVAL' ? 'Process Paused at Gate' : 'Process Idle / Completed' }}
            </span>
          </div>

          <span class="flex items-center gap-1 text-[10px] font-semibold text-emerald-400 group-hover/proc:underline flex-shrink-0 ml-2">
            <span>&gt;_ Console</span>
          </span>
        </div>

        <!-- Real Status & Step Information Pill -->
        <div class="mb-3 p-2 rounded bg-slate-950/90 border border-slate-800/80 space-y-1.5 text-[10px] font-mono">
          <div class="flex items-center justify-between">
            <span class="text-slate-400 font-medium">Step {{ guidance.stageNumber }}/{{ guidance.totalStages }}</span>
            <span
              v-if="guidance.actionType === 'blocked_steer'"
              class="text-rose-400 font-semibold"
            >
              ● Steer Needed
            </span>
            <span
              v-else-if="guidance.actionType === 'gate_approval'"
              class="text-amber-400 font-semibold"
            >
              ● Gate Review
            </span>
            <span
              v-else
              class="text-emerald-400"
            >
              ● {{ guidance.isWriteLocked ? 'Locked (Red)' : 'Active' }}
            </span>
          </div>

          <div class="flex items-center justify-between gap-2 pt-1 border-t border-slate-800/60">
            <!-- Contextual Stage / Column Target Description -->
            <div class="text-slate-400 truncate flex items-center gap-1 flex-1 min-w-0">
              <template v-if="guidance.actionType === 'gate_approval'">
                <span class="text-amber-400 font-medium">Gate:</span>
                <span class="text-slate-300 truncate" title="Human approval required before advancing">Requires Approval</span>
              </template>
              <template v-else-if="guidance.actionType === 'blocked_steer'">
                <span class="text-rose-400 font-medium">Status:</span>
                <span class="text-slate-300 truncate" title="Execution halted due to error loop">Circuit Breaker Halted</span>
              </template>
              <template v-else-if="guidance.actionType === 'completed_review'">
                <span class="text-emerald-400 font-medium">Status:</span>
                <span class="text-slate-300 truncate">All Stages Verified</span>
              </template>
              <template v-else-if="nextStage">
                <span class="text-slate-500">Next Col:</span>
                <span class="text-slate-300 truncate" :title="nextStage.name">{{ nextStage.name }}</span>
              </template>
              <template v-else>
                <span class="text-slate-500">Current:</span>
                <span class="text-slate-300 truncate">{{ guidance.stageName }}</span>
              </template>
            </div>

            <!-- Contextual Quick Action Buttons -->
            <!-- 1. Review Gate when approval is pending -->
            <button
              v-if="guidance.actionType === 'gate_approval'"
              @click.stop="navigateToTask"
              type="button"
              title="Pipeline paused: Review gate decision and authorize next stage"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-amber-950/90 hover:bg-amber-800 border border-amber-600/80 text-amber-200 hover:text-white text-[10px] font-mono font-medium transition-all shadow-sm flex-shrink-0 group/gate"
            >
              <span>Review Gate</span>
              <ArrowRight class="w-3 h-3 text-amber-400 group-hover/gate:translate-x-0.5 transition-transform" />
            </button>

            <!-- 2. Inspect Error when blocked -->
            <button
              v-else-if="guidance.actionType === 'blocked_steer'"
              @click.stop="emit('open-console', task.id)"
              type="button"
              title="Click to inspect console terminal error logs and steer the agent"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-rose-950/90 hover:bg-rose-800 border border-rose-600/80 text-rose-200 hover:text-white text-[10px] font-mono font-medium transition-all shadow-sm flex-shrink-0 group/steer"
            >
              <AlertTriangle class="w-3 h-3 text-rose-400" />
              <span>Inspect Error</span>
            </button>

            <!-- 3. Completed State -->
            <span
              v-else-if="guidance.actionType === 'completed_review'"
              class="inline-flex items-center gap-1 text-[10px] font-mono text-emerald-400 font-medium px-1.5 py-0.5 rounded bg-emerald-950/40 border border-emerald-800/40"
            >
              <Check class="w-3 h-3 text-emerald-400" />
              <span>Done</span>
            </span>

            <!-- 4. 1-Click Move Column (Drag & Drop Alternative) -->
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

          <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400">
            <Layers class="w-3 h-3" />
            <span>{{ task.assigned_repos?.length || 1 }}</span>
          </div>
        </div>

        <div class="flex items-center gap-2.5">
          <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400">
            <Flame class="w-3 h-3 text-amber-500" />
            <span>{{ Math.round((task.token_usage?.total_tokens || 0) / 1000) }}k</span>
          </div>

          <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400">
            <Clock class="w-3 h-3" />
            <span>{{ timeElapsed }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
