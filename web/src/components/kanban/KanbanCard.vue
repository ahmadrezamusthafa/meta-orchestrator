<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import type { Task, WorkflowStage } from '../../types'
import { useTaskStore } from '../../stores/tasks'
import { taskLifecycle, stagePosition, TONE_BADGE, ACTION_BUTTON } from '../../composables/taskLifecycle'
import {
  MoreVertical, Check, Terminal, GitBranch, Link2, Play, Pause, RotateCw, RefreshCcw, Eye,
  ExternalLink, FileText, Layers, Trash2, ArrowRight, Loader2,
} from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  task: Task
  density?: 'comfortable' | 'compact'
  allStages?: WorkflowStage[]
}>(), {
  density: 'comfortable',
  allStages: () => [],
})

const emit = defineEmits<{
  (e: 'open-console', taskId: string): void
  (e: 'request-delete', taskId: string): void
}>()

const router = useRouter()
const taskStore = useTaskStore()

const isDragging = ref(false)
const copiedId = ref(false)
const showMenu = ref(false)

const life = computed(() => taskLifecycle(props.task)!)
const pending = computed(() => !!taskStore.pendingActions[props.task.id])
const pos = computed(() => stagePosition(props.task.current_stage_id))
const meta = computed(() => props.task.metadata || {})

const jiraKey = computed(() => meta.value.jira_key || null)
const epicLabel = computed(() => {
  const key = meta.value.jira_epic_key
  if (!key || key === meta.value.jira_key) return null
  return meta.value.jira_epic_name || key
})
const branch = computed(() => meta.value.worktree_branch || '')
const repos = computed(() => props.task.assigned_repos || [])
const unmetDeps = computed(() => (meta.value.unmet_dependencies || '').split(',').filter(Boolean))

const nextStage = computed<WorkflowStage | null>(() => {
  const i = props.allStages.findIndex((s) => s.id === props.task.current_stage_id)
  return i >= 0 && i < props.allStages.length - 1 ? props.allStages[i + 1] : null
})

const updatedAgo = computed(() => {
  const mins = Math.max(0, Math.floor((Date.now() - new Date(props.task.updated_at || props.task.created_at).getTime()) / 60000))
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  if (mins < 1440) return `${Math.floor(mins / 60)}h ago`
  return `${Math.floor(mins / 1440)}d ago`
})

const ACTION_ICON = { run: Play, resume: RotateCw, pause: Pause, reset: RefreshCcw, review: Eye }

async function runPrimary() {
  const a = life.value.primary
  if (!a || pending.value) return
  if (a.kind === 'review') {
    emit('open-console', props.task.id) // the console hosts approve / request-changes
    return
  }
  if (a.kind !== 'pause') emit('open-console', props.task.id) // watch the run live
  try {
    await taskStore.performTaskAction(props.task.id, a.kind)
  } catch {
    /* the store already reported it */
  }
}

function open() {
  router.push(`/tasks/${props.task.id}`)
}

async function copyId() {
  try {
    await navigator.clipboard.writeText(props.task.id)
    copiedId.value = true
    setTimeout(() => (copiedId.value = false), 1500)
  } catch {
    /* clipboard unavailable */
  }
}

function moveTo(stageId: string) {
  showMenu.value = false
  taskStore.moveTaskToStage(props.task.id, stageId)
}

function requestDelete() {
  showMenu.value = false
  emit('request-delete', props.task.id)
}

function onDocClick() {
  showMenu.value = false
}
function toggleMenu() {
  showMenu.value = !showMenu.value
  if (showMenu.value) setTimeout(() => document.addEventListener('click', onDocClick, { once: true }))
}
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))

function onDragStart(e: DragEvent) {
  isDragging.value = true
  e.dataTransfer?.setData('text/plain', props.task.id)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}

const ACCENT: Record<string, string> = {
  active: 'border-l-sky-500', idle: 'border-l-slate-600', warn: 'border-l-amber-500', error: 'border-l-rose-500',
  done: 'border-l-emerald-500', review: 'border-l-violet-500', waiting: 'border-l-orange-500',
}
</script>

<template>
  <article
    draggable="true"
    @dragstart="onDragStart"
    @dragend="isDragging = false"
    @click="open"
    @keydown.enter.self="open"
    tabindex="0"
    :aria-label="`${task.id}: ${task.title}. ${life.status}.`"
    class="group relative rounded-lg bg-slate-900/90 border border-slate-800 border-l-[3px] hover:border-slate-700 transition-all duration-150 cursor-pointer shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
    :class="[ACCENT[life.tone], density === 'compact' ? 'p-2.5 space-y-1.5' : 'p-3 space-y-2.5', { 'opacity-40': isDragging }]"
  >
    <!-- Header: ID, JIRA key, menu -->
    <div class="flex items-center gap-1.5 min-w-0">
      <button type="button" @click.stop="copyId" title="Copy task ID"
        class="font-mono text-[11px] font-semibold text-slate-400 hover:text-slate-200 flex-shrink-0">
        {{ copiedId ? 'Copied' : task.id }}
      </button>
      <a v-if="jiraKey" :href="meta.jira_url" target="_blank" rel="noopener" @click.stop
        :title="`Open ${jiraKey} in JIRA${meta.jira_status ? ` (${meta.jira_status})` : ''}`"
        class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-blue-950/70 border border-blue-800/70 text-blue-300 hover:text-blue-100 text-[10px] font-mono">
        {{ jiraKey }}<ExternalLink class="w-2.5 h-2.5 opacity-70" />
      </a>
      <span v-if="epicLabel && taskStore.groupBy !== 'epic'" :title="`Epic ${meta.jira_epic_key}`"
        class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-violet-950/60 border border-violet-800/60 text-violet-300 text-[10px] min-w-0">
        <Layers class="w-2.5 h-2.5 flex-shrink-0" /><span class="truncate max-w-[110px]">{{ epicLabel }}</span>
      </span>

      <div class="relative ml-auto" @click.stop>
        <button type="button" @click="toggleMenu" aria-label="Card actions" :aria-expanded="showMenu"
          class="h-6 w-6 rounded flex items-center justify-center text-slate-500 hover:text-slate-200 hover:bg-slate-800">
          <MoreVertical class="w-3.5 h-3.5" />
        </button>
        <div v-if="showMenu" role="menu"
          class="absolute right-0 top-full mt-1 w-52 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl z-30 py-1 text-[11px]">
          <button type="button" role="menuitem" @click="emit('open-console', task.id); showMenu = false"
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 text-slate-200 flex items-center gap-2">
            <Terminal class="w-3 h-3" /> Open console
          </button>
          <button v-if="nextStage" type="button" role="menuitem" @click="moveTo(nextStage.id)"
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 text-slate-200 flex items-center gap-2">
            <ArrowRight class="w-3 h-3" /> Move to {{ nextStage.name }}
          </button>
          <div class="px-3 pt-1.5 pb-1 mt-1 border-t border-slate-800 text-[10px] uppercase tracking-wider text-slate-500">Move to stage</div>
          <div class="max-h-44 overflow-y-auto">
            <button v-for="s in allStages" :key="s.id" type="button" role="menuitem" @click="moveTo(s.id)"
              class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center justify-between"
              :class="s.id === task.current_stage_id ? 'text-emerald-400 font-semibold' : 'text-slate-300'">
              <span class="truncate">{{ s.name }}</span>
              <Check v-if="s.id === task.current_stage_id" class="w-3 h-3 flex-shrink-0" />
            </button>
          </div>
          <button type="button" role="menuitem" @click="requestDelete"
            class="w-full text-left px-3 py-1.5 mt-1 border-t border-slate-800 hover:bg-rose-950/60 text-rose-300 flex items-center gap-2">
            <Trash2 class="w-3 h-3" /> Remove from board
          </button>
        </div>
      </div>
    </div>

    <!-- Title -->
    <h4 class="text-xs font-medium text-slate-100 leading-relaxed" :class="density === 'compact' ? 'line-clamp-1' : 'line-clamp-2'" :title="task.title">
      {{ task.title }}
    </h4>

    <!-- Status: one honest line -->
    <div class="flex items-center gap-2 min-w-0">
      <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded border text-[10px] font-semibold flex-shrink-0" :class="TONE_BADGE[life.tone]">
        <span v-if="life.isRunning" class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-pulse"></span>
        {{ life.status }}
      </span>
      <span class="text-[11px] text-slate-400 truncate" :title="life.detail">
        <template v-if="unmetDeps.length">Needs {{ unmetDeps.join(', ') }}</template>
        <template v-else-if="task.state === 'FAILED' && meta.last_error">{{ meta.last_error }}</template>
        <template v-else-if="pos.index >= 0">Stage {{ pos.index + 1 }} of {{ pos.total }}</template>
      </span>
    </div>

    <!-- Context (comfortable only) -->
    <div v-if="density === 'comfortable'" class="flex items-center gap-2 text-[10px] text-slate-500 min-w-0">
      <span v-if="repos.length" class="font-mono truncate" :title="repos.join(', ')">
        {{ repos[0] }}<template v-if="repos.length > 1"> +{{ repos.length - 1 }}</template>
      </span>
      <span v-if="branch" class="flex items-center gap-0.5 text-teal-400/80 flex-shrink-0" :title="`Branch ${branch}`">
        <GitBranch class="w-3 h-3" />
      </span>
      <span v-if="task.dependencies?.length" class="flex items-center gap-0.5 flex-shrink-0" :title="`Prerequisites: ${task.dependencies.join(', ')}`">
        <Link2 class="w-3 h-3" />{{ task.dependencies.length }}
      </span>
      <a v-if="meta.confluence_page_url" :href="meta.confluence_page_url" target="_blank" rel="noopener" @click.stop
        class="flex items-center gap-0.5 text-indigo-300 hover:text-indigo-100 flex-shrink-0" title="Published on Confluence">
        <FileText class="w-3 h-3" />
      </a>
      <span class="ml-auto flex-shrink-0" :title="`Updated ${new Date(task.updated_at).toLocaleString()}`">{{ updatedAgo }}</span>
    </div>

    <!-- Actions: exactly one primary action + console -->
    <div class="flex items-center gap-1.5" @click.stop>
      <button v-if="life.primary" type="button" @click="runPrimary" :disabled="pending" :title="life.primary.hint"
        class="h-7 px-2.5 rounded border text-[11px] font-semibold flex items-center gap-1.5 transition-colors disabled:opacity-60 disabled:cursor-wait"
        :class="ACTION_BUTTON[life.primary.kind]">
        <Loader2 v-if="pending" class="w-3 h-3 animate-spin" />
        <component v-else :is="ACTION_ICON[life.primary.kind]" class="w-3 h-3" />
        {{ life.primary.label }}
      </button>
      <span v-else class="text-[11px] text-slate-500 italic">{{ task.state === 'COMPLETED' ? 'Done' : 'Starts automatically' }}</span>
      <button type="button" @click="emit('open-console', task.id)" title="Open console" aria-label="Open console"
        class="ml-auto h-7 w-7 rounded border border-slate-800 bg-slate-950 text-slate-400 hover:text-slate-100 hover:border-slate-700 flex items-center justify-center">
        <Terminal class="w-3.5 h-3.5" />
      </button>
    </div>
  </article>
</template>
