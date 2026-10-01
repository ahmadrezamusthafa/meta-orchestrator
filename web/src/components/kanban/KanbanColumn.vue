<script setup lang="ts">
import { ref } from 'vue'
import type { Task, WorkflowStage } from '../../types'
import { useTaskStore } from '../../stores/tasks'
import KanbanCard from './KanbanCard.vue'
import KanbanColumnEmpty from './KanbanColumnEmpty.vue'
import KanbanCardSkeleton from './KanbanCardSkeleton.vue'
import { Lock, ShieldAlert, CheckCircle2, Plus, ChevronLeft, ChevronRight } from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  stage: WorkflowStage
  tasks: Task[]
  isLoading?: boolean
  isCollapsed?: boolean
  density?: 'comfortable' | 'compact'
  allStages?: WorkflowStage[]
}>(), {
  isLoading: false,
  isCollapsed: false,
  density: 'comfortable',
  allStages: () => []
})

const emit = defineEmits<{
  (e: 'toggle-collapse', stageId: string): void
  (e: 'quick-add', stageId: string): void
  (e: 'open-console', taskId: string): void
  (e: 'request-delete', taskId: string): void
}>()

const taskStore = useTaskStore()
const isDragOver = ref(false)

function handleDrop(e: DragEvent) {
  isDragOver.value = false
  const taskId = e.dataTransfer?.getData('text/plain')
  if (taskId) {
    taskStore.moveTaskToStage(taskId, props.stage.id)
  }
}
</script>

<template>
  <!-- Collapsed Rail View -->
  <div
    v-if="isCollapsed"
    @dragover.prevent="isDragOver = true"
    @dragenter.prevent="isDragOver = true"
    @dragleave="isDragOver = false"
    @drop="handleDrop"
    @click="$emit('toggle-collapse', stage.id)"
    title="Click to expand column"
    class="w-14 min-w-[56px] self-stretch bg-slate-900/40 rounded-xl border cursor-pointer hover:bg-slate-900/80 hover:border-slate-700 transition-all duration-150 select-none group shadow-sm"
    :class="isDragOver ? 'border-emerald-500 bg-emerald-950/30 ring-2 ring-emerald-500/50' : 'border-slate-800'"
  >
    <!-- Contents stay in view while the board scrolls -->
    <!-- -top-6 offsets the board's p-6 padding so sticky content pins to the visible top edge -->
    <div class="sticky -top-6 flex flex-col items-center gap-4 py-4">
    <!-- Top: Stage Icon & Expand Button -->
    <div class="flex flex-col items-center gap-3">
      <button
        type="button"
        title="Expand column"
        class="p-1 rounded text-slate-500 group-hover:text-slate-200 transition-colors"
      >
        <ChevronRight class="w-4 h-4" />
      </button>

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
    </div>

    <!-- Middle: Vertical Text Title -->
    <div class="flex items-center justify-center">
      <span
        class="text-xs font-semibold text-slate-400 group-hover:text-slate-200 tracking-wider whitespace-nowrap"
        style="writing-mode: vertical-rl; transform: rotate(180deg);"
      >
        {{ stage.name }}
      </span>
    </div>

    <!-- Bottom: Task Count Badge -->
    <div class="flex flex-col items-center gap-2">
      <span
        class="w-6 h-6 rounded-full flex items-center justify-center text-[11px] font-mono font-semibold"
        :class="tasks.length > 0
          ? 'bg-blue-950 text-blue-300 border border-blue-800'
          : 'bg-slate-800 text-slate-500'"
      >
        {{ tasks.length }}
      </span>
    </div>
    </div>
  </div>

  <!-- Expanded Full Column View -->
  <div
    v-else
    @dragover.prevent="isDragOver = true"
    @dragenter.prevent="isDragOver = true"
    @dragleave="isDragOver = false"
    @drop="handleDrop"
    class="self-stretch bg-slate-900/60 rounded-xl border flex flex-col shadow-sm transition-all duration-150"
    :class="[
      density === 'compact' ? 'min-w-[280px] w-[280px]' : 'min-w-[320px] w-[320px]',
      isDragOver ? 'border-emerald-500 bg-emerald-950/20 ring-2 ring-emerald-500/50' : 'border-slate-800'
    ]"
  >
    <!-- -top-6 offsets the board's p-6 padding so the header pins to the visible top edge -->
    <div class="sticky -top-6 z-10 p-3 border-b border-slate-800 bg-slate-900 rounded-t-xl flex items-center justify-between gap-2">
      <div class="flex items-center gap-2 truncate">
        <span
          v-if="stage.write_lock_workspace"
          title="Write-locked stage (Red Phase)"
          class="text-amber-400 flex-shrink-0"
        >
          <Lock class="w-3.5 h-3.5" />
        </span>
        <span
          v-else-if="stage.requires_gate"
          title="Requires Human Approval Gate"
          class="text-rose-400 flex-shrink-0"
        >
          <ShieldAlert class="w-3.5 h-3.5" />
        </span>
        <span
          v-else
          class="text-emerald-400 flex-shrink-0"
        >
          <CheckCircle2 class="w-3.5 h-3.5" />
        </span>

        <h3 class="text-xs font-semibold text-slate-200 truncate" :title="stage.name">
          {{ stage.name }}
        </h3>
      </div>

      <div class="flex items-center gap-1.5 flex-shrink-0">
        <!-- Quick Add Task in This Stage -->
        <button
          @click="$emit('quick-add', stage.id)"
          type="button"
          :title="`Add task starting at ${stage.name}`"
          class="p-1 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition-colors"
        >
          <Plus class="w-3.5 h-3.5" />
        </button>

        <!-- Task Count Badge -->
        <span
          class="px-2 py-0.5 rounded-full text-[11px] font-mono font-medium transition-colors"
          :class="tasks.length > 0 ? 'bg-slate-800 text-slate-200 font-semibold' : 'bg-slate-800/60 text-slate-500'"
        >
          {{ tasks.length }}
        </span>

        <!-- Collapse Column Button -->
        <button
          @click="$emit('toggle-collapse', stage.id)"
          type="button"
          title="Collapse column into rail"
          class="p-1 rounded text-slate-500 hover:text-slate-300 hover:bg-slate-800 transition-colors ml-0.5"
        >
          <ChevronLeft class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <!-- Tasks List Container -->
    <div class="flex-1 p-2.5 space-y-2.5">
      <template v-if="isLoading">
        <KanbanCardSkeleton />
        <KanbanCardSkeleton />
      </template>

      <template v-else-if="tasks.length > 0">
        <KanbanCard
          v-for="task in tasks"
          :key="task.id"
          :task="task"
          :density="density"
          :all-stages="allStages"
          @open-console="(id) => emit('open-console', id)"
          @request-delete="(id) => emit('request-delete', id)"
        />
      </template>

      <template v-else>
        <KanbanColumnEmpty />
      </template>
    </div>
  </div>
</template>
