<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useTaskStore } from '../stores/tasks'
import { useWorkflowStore } from '../stores/workflows'
import KanbanToolbar from '../components/kanban/KanbanToolbar.vue'
import KanbanColumn from '../components/kanban/KanbanColumn.vue'
import NewTaskModal from '../components/kanban/NewTaskModal.vue'

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()

const showNewTaskModal = ref(false)

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
    <KanbanToolbar @open-new-task="showNewTaskModal = true" />

    <!-- Horizontal Scrolling Kanban Board Layout -->
    <main class="flex-1 p-6 overflow-x-auto overflow-y-hidden">
      <div class="flex items-start gap-4 h-full min-w-max pb-2">
        <KanbanColumn
          v-for="stage in workflowStore.projectedColumns"
          :key="stage.id"
          :stage="stage"
          :tasks="taskStore.tasksByStage[stage.id] || []"
          :is-loading="taskStore.isLoading"
        />
      </div>
    </main>

    <!-- New Task Modal Dialog -->
    <NewTaskModal
      v-if="showNewTaskModal"
      @close="showNewTaskModal = false"
      @created="() => {}"
    />
  </div>
</template>
