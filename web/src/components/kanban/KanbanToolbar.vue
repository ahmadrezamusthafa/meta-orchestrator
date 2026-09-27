<script setup lang="ts">
import { computed, ref } from 'vue'
import { useTaskStore } from '../../stores/tasks'
import { useWorkflowStore } from '../../stores/workflows'
import BtnPrimary from '../common/BtnPrimary.vue'
import { 
  Search, Plus, RotateCcw, RefreshCw, X, Download, User, 
  LayoutGrid, LayoutList, EyeOff, Eye, ShieldAlert, CheckCircle2, PlayCircle, AlertTriangle 
} from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  density?: 'comfortable' | 'compact'
  hasCollapsedEmpty?: boolean
}>(), {
  density: 'comfortable',
  hasCollapsedEmpty: false
})

const emit = defineEmits<{
  (e: 'open-new-task'): void
  (e: 'open-jira-import'): void
  (e: 'update:density', density: 'comfortable' | 'compact'): void
  (e: 'toggle-collapse-empty'): void
}>()

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()
const isRefreshing = ref(false)

const isFiltered = computed(() => {
  return (
    taskStore.searchQuery.trim() !== '' ||
    taskStore.selectedMethod !== 'All' ||
    taskStore.selectedRepo !== 'All' ||
    taskStore.selectedStatus !== 'all' ||
    taskStore.onlyMyTasks
  )
})

const visibleTaskCount = computed(() => {
  return Object.values(taskStore.tasksByStage).reduce((acc, list) => acc + list.length, 0)
})

function resetFilters() {
  taskStore.searchQuery = ''
  taskStore.selectedMethod = 'All'
  taskStore.selectedRepo = 'All'
  taskStore.selectedStatus = 'all'
  taskStore.onlyMyTasks = false
}

function toggleDensity() {
  const next = props.density === 'comfortable' ? 'compact' : 'comfortable'
  emit('update:density', next)
}

async function handleRefresh() {
  isRefreshing.value = true
  try {
    await taskStore.fetchTasks()
  } finally {
    setTimeout(() => {
      isRefreshing.value = false
    }, 400)
  }
}
</script>

<template>
  <div class="px-6 py-2.5 bg-slate-900/60 border-b border-slate-800 flex flex-wrap items-center justify-between gap-3 min-h-[56px]">
    <!-- Left Filters Section -->
    <div class="flex flex-wrap items-center gap-2.5">
      <!-- Search Input -->
      <div class="relative w-56">
        <Search class="absolute left-2.5 top-2.5 w-3.5 h-3.5 text-slate-500" />
        <input
          v-model="taskStore.searchQuery"
          type="text"
          placeholder="Filter tasks or ID..."
          class="w-full h-8 pl-8 pr-7 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-blue-500 transition-colors"
        />
        <button
          v-if="taskStore.searchQuery"
          @click="taskStore.searchQuery = ''"
          type="button"
          class="absolute right-2 top-2 text-slate-500 hover:text-slate-300 transition-colors"
        >
          <X class="w-3.5 h-3.5" />
        </button>
      </div>

      <!-- Workflow Selector -->
      <select
        v-model="workflowStore.activeWorkflowId"
        class="h-8 px-2.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-300 focus:outline-none focus:border-blue-500 max-w-[190px] truncate"
      >
        <option v-if="workflowStore.workflows.length === 0" value="general_ai_sdlc">
          General AI SDLC (8 Stages)
        </option>
        <option v-for="w in workflowStore.workflows" :key="w.id" :value="w.id">
          {{ w.name }}
        </option>
      </select>

      <div class="h-4 w-px bg-slate-800 hidden sm:block"></div>

      <!-- Status Filter Chips -->
      <div class="flex items-center gap-1 bg-slate-950 p-0.5 rounded-lg border border-slate-800/80">
        <button
          @click="taskStore.selectedStatus = 'all'"
          type="button"
          class="px-2 py-1 rounded text-[11px] font-medium transition-colors"
          :class="taskStore.selectedStatus === 'all'
            ? 'bg-slate-800 text-slate-100 shadow-sm font-semibold'
            : 'text-slate-400 hover:text-slate-200'"
        >
          All ({{ taskStore.statusCounts.all }})
        </button>

        <button
          @click="taskStore.selectedStatus = 'running'"
          type="button"
          class="px-2 py-1 rounded text-[11px] font-medium flex items-center gap-1 transition-colors"
          :class="taskStore.selectedStatus === 'running'
            ? 'bg-emerald-950/80 text-emerald-300 border border-emerald-800/80 shadow-sm font-semibold'
            : 'text-slate-400 hover:text-emerald-400'"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
          Running ({{ taskStore.statusCounts.running }})
        </button>

        <button
          @click="taskStore.selectedStatus = 'gate'"
          type="button"
          class="px-2 py-1 rounded text-[11px] font-medium flex items-center gap-1 transition-colors"
          :class="taskStore.selectedStatus === 'gate'
            ? 'bg-amber-950/80 text-amber-300 border border-amber-800/80 shadow-sm font-semibold'
            : 'text-slate-400 hover:text-amber-400'"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-amber-400"></span>
          Gate ({{ taskStore.statusCounts.gate }})
        </button>

        <button
          @click="taskStore.selectedStatus = 'blocked'"
          type="button"
          class="px-2 py-1 rounded text-[11px] font-medium flex items-center gap-1 transition-colors"
          :class="taskStore.selectedStatus === 'blocked'
            ? 'bg-rose-950/80 text-rose-300 border border-rose-800/80 shadow-sm font-semibold'
            : 'text-slate-400 hover:text-rose-400'"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-rose-400"></span>
          Blocked ({{ taskStore.statusCounts.blocked }})
        </button>

        <button
          @click="taskStore.selectedStatus = 'completed'"
          type="button"
          class="px-2 py-1 rounded text-[11px] font-medium flex items-center gap-1 transition-colors"
          :class="taskStore.selectedStatus === 'completed'
            ? 'bg-slate-800 text-slate-200 border border-slate-700 shadow-sm font-semibold'
            : 'text-slate-400 hover:text-slate-200'"
        >
          Completed ({{ taskStore.statusCounts.completed }})
        </button>
      </div>

      <!-- My Assigned Tasks Toggle Chip -->
      <button
        @click="taskStore.onlyMyTasks = !taskStore.onlyMyTasks"
        type="button"
        title="Show only tasks linked to your active JIRA user/credentials"
        class="h-8 px-2.5 rounded-lg border text-xs font-medium flex items-center gap-1.5 transition-all"
        :class="taskStore.onlyMyTasks
          ? 'bg-blue-950/80 border-blue-600 text-blue-300 shadow-sm font-semibold'
          : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700'"
      >
        <User class="w-3.5 h-3.5" :class="taskStore.onlyMyTasks ? 'text-blue-400' : 'text-slate-500'" />
        <span>My JIRA Tasks</span>
      </button>

      <!-- Reset Filters Pill -->
      <button
        v-if="isFiltered"
        @click="resetFilters"
        type="button"
        class="h-8 px-2.5 rounded-lg bg-slate-800/80 hover:bg-slate-800 border border-slate-700 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5 transition-colors"
      >
        <RotateCcw class="w-3 h-3" />
        <span>Reset</span>
      </button>
    </div>

    <!-- Right Actions Section -->
    <div class="flex items-center gap-2">
      <!-- Focus Active / Collapse Inactive Columns Toggle -->
      <button
        @click="$emit('toggle-collapse-empty')"
        type="button"
        :title="hasCollapsedEmpty ? 'Expand all columns' : 'Collapse empty columns into rails to reduce scrolling'"
        class="h-8 px-2.5 rounded-lg border text-xs font-medium flex items-center gap-1.5 transition-all"
        :class="hasCollapsedEmpty
          ? 'bg-purple-950/70 border-purple-700/80 text-purple-300 shadow-sm'
          : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700'"
      >
        <EyeOff v-if="!hasCollapsedEmpty" class="w-3.5 h-3.5 text-purple-400" />
        <Eye v-else class="w-3.5 h-3.5 text-purple-300" />
        <span class="hidden md:inline">{{ hasCollapsedEmpty ? 'Expand All' : 'Focus Active' }}</span>
      </button>

      <!-- Density Toggle Button -->
      <button
        @click="toggleDensity"
        type="button"
        :title="density === 'comfortable' ? 'Switch to Compact view (see more tasks)' : 'Switch to Comfortable view'"
        class="h-8 px-2.5 rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 text-xs font-medium flex items-center gap-1.5 transition-colors"
      >
        <LayoutList v-if="density === 'comfortable'" class="w-3.5 h-3.5 text-slate-400" />
        <LayoutGrid v-else class="w-3.5 h-3.5 text-emerald-400" />
        <span class="hidden sm:inline capitalize">{{ density }}</span>
      </button>

      <!-- Refresh Button -->
      <button
        @click="handleRefresh"
        :disabled="isRefreshing"
        title="Refresh task board"
        type="button"
        class="h-8 w-8 flex items-center justify-center rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
      >
        <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin text-emerald-400': isRefreshing }" />
      </button>

      <!-- Import JIRA Button -->
      <button
        @click="$emit('open-jira-import')"
        type="button"
        title="Import tickets assigned to your credentials"
        class="h-8 px-3 rounded-lg bg-blue-950/60 hover:bg-blue-900/80 border border-blue-800/80 text-blue-300 hover:text-blue-100 flex items-center gap-1.5 text-xs font-medium transition-all shadow-sm"
      >
        <Download class="w-3.5 h-3.5 text-blue-400" />
        <span>Import JIRA</span>
      </button>

      <!-- New Task Button -->
      <BtnPrimary @click="$emit('open-new-task')">
        <Plus class="w-3.5 h-3.5" />
        <span>New Task</span>
      </BtnPrimary>
    </div>
  </div>
</template>
