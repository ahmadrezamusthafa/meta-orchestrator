<script setup lang="ts">
import type { Task, WorkflowStage } from '../../types'
import KanbanCard from './KanbanCard.vue'
import KanbanColumnEmpty from './KanbanColumnEmpty.vue'
import KanbanCardSkeleton from './KanbanCardSkeleton.vue'
import { Lock, ShieldAlert, CheckCircle2 } from 'lucide-vue-next'

defineProps<{
  stage: WorkflowStage
  tasks: Task[]
  isLoading?: boolean
}>()
</script>

<template>
  <div class="w-[320px] min-w-[320px] h-full bg-slate-900/60 rounded-xl border border-slate-800 flex flex-col overflow-hidden shadow-sm">
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
