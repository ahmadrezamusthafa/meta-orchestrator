<script setup lang="ts">
import { computed, ref } from 'vue'
import { useTaskStore } from '../../stores/tasks'
import { useWorkflowStore } from '../../stores/workflows'
import BtnPrimary from '../common/BtnPrimary.vue'
import { Search, Plus, RotateCcw, RefreshCw, X, Filter } from 'lucide-vue-next'

defineEmits<{
  (e: 'open-new-task'): void
}>()

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()
const isRefreshing = ref(false)

const isFiltered = computed(() => {
  return (
    taskStore.searchQuery.trim() !== '' ||
    taskStore.selectedMethod !== 'All' ||
    taskStore.selectedRepo !== 'All'
  )
})

const visibleTaskCount = computed(() => {
  return Object.values(taskStore.tasksByStage).reduce((acc, list) => acc + list.length, 0)
})

function resetFilters() {
  taskStore.searchQuery = ''
  taskStore.selectedMethod = 'All'
  taskStore.selectedRepo = 'All'
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
  <div class="h-14 px-6 py-2.5 bg-slate-900/50 border-b border-slate-800 flex items-center justify-between gap-4">
    <div class="flex items-center gap-3">
      <!-- Search -->
      <div class="relative w-64">
        <Search class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
        <input
          v-model="taskStore.searchQuery"
          type="text"
          placeholder="Filter title or ID..."
          class="w-full h-9 pl-9 pr-8 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500 transition-colors"
        />
        <button
          v-if="taskStore.searchQuery"
          @click="taskStore.searchQuery = ''"
          type="button"
          class="absolute right-2.5 top-2.5 text-slate-500 hover:text-slate-300 transition-colors"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <!-- Workflow Selector -->
      <div class="flex items-center gap-2">
        <select
          v-model="workflowStore.activeWorkflowId"
          class="h-9 w-48 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-300 focus:outline-none focus:border-emerald-500"
        >
          <option v-if="workflowStore.workflows.length === 0" value="general_ai_sdlc">
            General AI SDLC (Default)
          </option>
          <option v-for="w in workflowStore.workflows" :key="w.id" :value="w.id">
            {{ w.name }}
          </option>
        </select>
      </div>

      <!-- Method Filter -->
      <div class="flex items-center gap-2">
        <select
          v-model="taskStore.selectedMethod"
          class="h-9 w-36 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-300 focus:outline-none focus:border-emerald-500"
        >
          <option v-for="m in taskStore.availableMethods" :key="m" :value="m">
            {{ m === 'All' ? 'All Methods' : m }}
          </option>
        </select>
      </div>

      <!-- Repo Filter -->
      <div class="flex items-center gap-2">
        <select
          v-model="taskStore.selectedRepo"
          class="h-9 w-44 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-300 focus:outline-none focus:border-emerald-500"
        >
          <option v-for="r in taskStore.availableRepos" :key="r" :value="r">
            {{ r === 'All' ? 'All Repositories' : r }}
          </option>
        </select>
      </div>

      <!-- Reset Filters Pill -->
      <button
        v-if="isFiltered"
        @click="resetFilters"
        type="button"
        class="h-8 px-2.5 rounded bg-slate-800/80 hover:bg-slate-800 border border-slate-700 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5 transition-colors"
      >
        <RotateCcw class="w-3 h-3" />
        <span>Reset Filters</span>
      </button>

      <!-- Task Count Badge -->
      <div class="hidden xl:flex items-center gap-1 text-[11px] font-mono text-slate-500 pl-1">
        <span>{{ visibleTaskCount }} of {{ taskStore.tasks.length }} tasks</span>
      </div>
    </div>

    <div class="flex items-center gap-2.5">
      <button
        @click="handleRefresh"
        :disabled="isRefreshing"
        title="Refresh task board"
        type="button"
        class="h-9 w-9 flex items-center justify-center rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
      >
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin text-emerald-400': isRefreshing }" />
      </button>

      <BtnPrimary @click="$emit('open-new-task')">
        <Plus class="w-4 h-4" />
        <span>New Task</span>
      </BtnPrimary>
    </div>
  </div>
</template>
