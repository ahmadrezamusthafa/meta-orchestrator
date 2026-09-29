<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useTaskStore } from '../stores/tasks'
import { useWorkflowStore } from '../stores/workflows'
import KanbanToolbar from '../components/kanban/KanbanToolbar.vue'
import KanbanColumn from '../components/kanban/KanbanColumn.vue'
import NewTaskModal from '../components/kanban/NewTaskModal.vue'
import JiraImportModal from '../components/kanban/JiraImportModal.vue'
import TaskConsoleDrawer from '../components/kanban/TaskConsoleDrawer.vue'
import KanbanSwimlanes from '../components/kanban/KanbanSwimlanes.vue'
import JiraSyncSettingsModal from '../components/kanban/JiraSyncSettingsModal.vue'
import ConfirmDeleteModal from '../components/common/ConfirmDeleteModal.vue'
import { LayoutDashboard, Plus, Download, Settings2 } from 'lucide-vue-next'

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()

const showNewTaskModal = ref(false)
const showJiraImportModal = ref(false)
const showJiraSyncModal = ref(false)
const pendingDeleteId = ref<string | null>(null)
const isDeleting = ref(false)

const pendingDeleteTask = computed(() => taskStore.tasks.find(t => t.id === pendingDeleteId.value) || null)
const isBoardEmpty = computed(() => !taskStore.isLoading && taskStore.tasks.length === 0)
const jiraConnected = computed(() => !!taskStore.jiraSync?.status.connected)

async function confirmDelete() {
  if (!pendingDeleteId.value) return
  isDeleting.value = true
  const id = pendingDeleteId.value
  try {
    if (activeConsoleTaskId.value === id) activeConsoleTaskId.value = null
    await taskStore.deleteTask(id)
  } finally {
    isDeleting.value = false
    pendingDeleteId.value = null
  }
}
const activeConsoleTaskId = ref<string | null>(null)
const initialStageForNewTask = ref<string | undefined>(undefined)
const density = ref<'comfortable' | 'compact'>('comfortable')
const collapsedStages = ref<Record<string, boolean>>({})

function toggleColumnCollapse(stageId: string) {
  collapsedStages.value = {
    ...collapsedStages.value,
    [stageId]: !collapsedStages.value[stageId]
  }
}

const hasCollapsedEmpty = computed(() => {
  return workflowStore.projectedColumns.some(stage => {
    const taskCount = (taskStore.tasksByStage[stage.id] || []).length
    return taskCount === 0 && !!collapsedStages.value[stage.id]
  })
})

function toggleCollapseEmpty() {
  if (hasCollapsedEmpty.value) {
    // Expand all
    collapsedStages.value = {}
  } else {
    // Collapse all stages with 0 tasks
    const next: Record<string, boolean> = {}
    workflowStore.projectedColumns.forEach(stage => {
      const taskCount = (taskStore.tasksByStage[stage.id] || []).length
      if (taskCount === 0) {
        next[stage.id] = true
      }
    })
    collapsedStages.value = next
  }
}

function openNewTaskForStage(stageId?: string) {
  initialStageForNewTask.value = stageId
  showNewTaskModal.value = true
}

onMounted(async () => {
  taskStore.initWebSocketSync()
  await Promise.all([
    taskStore.fetchTasks(),
    workflowStore.fetchWorkflows(),
    taskStore.fetchJiraSync(),
  ])
})
</script>

<template>
  <div class="h-full flex flex-col overflow-hidden bg-slate-950">
    <!-- Filter Sub-header Toolbar -->
    <KanbanToolbar
      :density="density"
      :has-collapsed-empty="hasCollapsedEmpty"
      @update:density="(d) => density = d"
      @toggle-collapse-empty="toggleCollapseEmpty"
      @open-new-task="openNewTaskForStage(undefined)"
      @open-jira-import="showJiraImportModal = true"
      @open-jira-sync="showJiraSyncModal = true"
    />

    <!-- Horizontal Scrolling Kanban Board Layout -->
    <main class="relative flex-1 p-6 overflow-x-auto" :class="taskStore.groupBy === 'epic' ? 'overflow-y-auto' : 'overflow-y-hidden'">
      <!-- Empty board: explain how work gets here -->
      <div v-if="isBoardEmpty" class="absolute inset-0 z-10 flex items-center justify-center p-6 bg-slate-950/70 backdrop-blur-[1px]">
        <div class="max-w-md w-full text-center bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-xl">
          <div class="mx-auto w-10 h-10 rounded-lg bg-slate-800 flex items-center justify-center text-slate-300 mb-3">
            <LayoutDashboard class="w-5 h-5" />
          </div>
          <h2 class="text-sm font-semibold text-slate-100">No tasks yet</h2>
          <p class="text-xs text-slate-400 mt-1.5 leading-relaxed">
            <template v-if="jiraConnected">JIRA is connected. Issues matching your sync rules land here automatically — or pick specific ones to import.</template>
            <template v-else>Create a task, or connect JIRA so your assigned issues show up here automatically.</template>
          </p>
          <div class="flex flex-wrap justify-center gap-2 mt-4">
            <button type="button" @click="openNewTaskForStage(undefined)"
              class="h-8 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-xs font-medium text-white flex items-center gap-1.5">
              <Plus class="w-3.5 h-3.5" /> New task
            </button>
            <template v-if="jiraConnected">
              <button type="button" @click="showJiraImportModal = true"
                class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 flex items-center gap-1.5">
                <Download class="w-3.5 h-3.5" /> Import from JIRA
              </button>
              <button type="button" @click="showJiraSyncModal = true"
                class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 flex items-center gap-1.5">
                <Settings2 class="w-3.5 h-3.5" /> Sync rules
              </button>
            </template>
            <router-link v-else to="/connectors"
              class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 flex items-center gap-1.5">
              Connect JIRA
            </router-link>
          </div>
        </div>
      </div>

      <KanbanSwimlanes
        v-if="taskStore.groupBy === 'epic'"
        :stages="workflowStore.projectedColumns"
        :lanes="taskStore.swimlanes"
        :density="density"
        @open-console="(id) => activeConsoleTaskId = id"
        @request-delete="(id) => pendingDeleteId = id"
      />
      <div v-else class="flex items-start gap-4 h-full min-w-max pb-2">
        <KanbanColumn
          v-for="stage in workflowStore.projectedColumns"
          :key="stage.id"
          :stage="stage"
          :tasks="taskStore.tasksByStage[stage.id] || []"
          :is-loading="taskStore.isLoading"
          :is-collapsed="!!collapsedStages[stage.id]"
          :density="density"
          :all-stages="workflowStore.projectedColumns"
          @toggle-collapse="toggleColumnCollapse"
          @quick-add="openNewTaskForStage"
          @open-console="(id) => activeConsoleTaskId = id"
          @request-delete="(id) => pendingDeleteId = id"
        />
      </div>
    </main>

    <!-- New Task Modal Dialog -->
    <NewTaskModal
      v-if="showNewTaskModal"
      :initial-stage-id="initialStageForNewTask"
      @close="showNewTaskModal = false; initialStageForNewTask = undefined"
      @created="() => {}"
    />

    <!-- JIRA Import Modal Dialog -->
    <JiraImportModal
      v-if="showJiraImportModal"
      @close="showJiraImportModal = false"
    />

    <JiraSyncSettingsModal
      v-if="showJiraSyncModal"
      @close="showJiraSyncModal = false"
    />

    <ConfirmDeleteModal
      :is-open="!!pendingDeleteId"
      title="Remove task from board"
      :item-name="pendingDeleteTask ? `${pendingDeleteTask.id} · ${pendingDeleteTask.title}` : ''"
      message="Any running agent turn is stopped and the task's console history is cleared."
      :note="pendingDeleteTask?.metadata?.jira_key
        ? `The JIRA issue ${pendingDeleteTask.metadata.jira_key} is untouched, and sync won't re-add it. Generated artifacts stay on disk.`
        : 'Generated artifacts stay on disk.'"
      confirm-text="Remove"
      :loading="isDeleting"
      @confirm="confirmDelete"
      @close="pendingDeleteId = null"
    />

    <!-- Task Background Process & Console Terminal Slide-over Drawer -->
    <TaskConsoleDrawer
      v-if="activeConsoleTaskId"
      :task-id="activeConsoleTaskId"
      @close="activeConsoleTaskId = null"
    />
  </div>
</template>
