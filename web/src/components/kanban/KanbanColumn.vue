<script setup lang="ts">
import { ref } from 'vue'
import type { Task, WorkflowStage } from '../../types'
import { useTaskStore } from '../../stores/tasks'
import { useToastStore } from '../../stores/toast'
import KanbanCard from './KanbanCard.vue'
import KanbanColumnEmpty from './KanbanColumnEmpty.vue'
import KanbanCardSkeleton from './KanbanCardSkeleton.vue'
import { Lock, ShieldAlert, CheckCircle2 } from 'lucide-vue-next'

const props = defineProps<{
  stage: WorkflowStage
  tasks: Task[]
  isLoading?: boolean
}>()

const taskStore = useTaskStore()
const toastStore = useToastStore()
const isDragOver = ref(false)

function handleDrop(e: DragEvent) {
  isDragOver.value = false
  const taskId = e.dataTransfer?.getData('text/plain')
  if (taskId) {
    taskStore.moveTaskToStage(taskId, props.stage.id)
    toastStore.info('Stage Updated', `Moved ${taskId} to ${props.stage.name}`)
  }
}
</script>

<template>
  <div
    @dragover.prevent="isDragOver = true"
    @dragenter.prevent="isDragOver = true"
    @dragleave="isDragOver = false"
    @drop="handleDrop"
    class="w-[320px] min-w-[320px] h-full bg-slate-900/60 rounded-xl border flex flex-col overflow-hidden shadow-sm transition-all duration-150"
    :class="isDragOver ? 'border-emerald-500 bg-emerald-950/20 ring-2 ring-emerald-500/50' : 'border-slate-800'"
  >
    <div class="p-3.5 border-b border-slate-800 bg-slate-900 flex items-center justify-between">
      <div class="flex items-center gap-2 truncate">
        <span
          v-if="stage.write_lock_workspace"
          title="Write-locked stage (Red Phase)"
          class="text-amber-400"
        >
          <Lock class="w-3.5 h-3.5" />
        </span>
        <span
          v-else-if="stage.requires_gate"
          title="Requires Human Approval Gate"
          class="text-rose-400"
        >
          <ShieldAlert class="w-3.5 h-3.5" />
        </span>
        <span
          v-else
          class="text-emerald-400"
        >
          <CheckCircle2 class="w-3.5 h-3.5" />
        </span>

        <h3 class="text-xs font-semibold text-slate-200 truncate">
          {{ stage.name }}
        </h3>
      </div>

      <span class="px-2 py-0.5 rounded-full bg-slate-800 text-[11px] font-mono font-medium text-slate-300">
        {{ tasks.length }}
      </span>
    </div>

    <div class="flex-1 p-3 overflow-y-auto space-y-3">
      <template v-if="isLoading">
        <KanbanCardSkeleton />
        <KanbanCardSkeleton />
      </template>

      <template v-else-if="tasks.length > 0">
        <KanbanCard
          v-for="task in tasks"
          :key="task.id"
          :task="task"
        />
      </template>

      <template v-else>
        <KanbanColumnEmpty />
      </template>
    </div>
  </div>
</template>
