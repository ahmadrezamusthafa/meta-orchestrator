<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useTaskStore } from '../stores/tasks'
import { useWorkflowStore } from '../stores/workflows'
import KanbanToolbar from '../components/kanban/KanbanToolbar.vue'
import KanbanColumn from '../components/kanban/KanbanColumn.vue'
import NewTaskModal from '../components/kanban/NewTaskModal.vue'
import JiraImportModal from '../components/kanban/JiraImportModal.vue'

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()

const showNewTaskModal = ref(false)
const showJiraImportModal = ref(false)
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
    />

    <!-- Horizontal Scrolling Kanban Board Layout -->
    <main class="flex-1 p-6 overflow-x-auto overflow-y-hidden">
      <div class="flex items-start gap-4 h-full min-w-max pb-2">
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
  </div>
</template>
