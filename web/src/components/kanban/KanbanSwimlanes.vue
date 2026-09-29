<script setup lang="ts">
import { ref } from 'vue'
import type { Task, WorkflowStage } from '../../types'
import { useTaskStore } from '../../stores/tasks'
import KanbanCard from './KanbanCard.vue'
import { ChevronDown, ChevronRight, ExternalLink, Layers } from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  stages: WorkflowStage[]
  lanes: { key: string; name: string; url: string; total: number; completed: number; byStage: Record<string, Task[]> }[]
  density?: 'comfortable' | 'compact'
}>(), { density: 'comfortable' })

const emit = defineEmits<{
  (e: 'open-console', taskId: string): void
  (e: 'request-delete', taskId: string): void
}>()

const taskStore = useTaskStore()
const collapsed = ref<Record<string, boolean>>({})
const dragOver = ref<string | null>(null)

const cellWidth = () => (props.density === 'compact' ? 'w-[280px] min-w-[280px]' : 'w-[320px] min-w-[320px]')

function drop(stageId: string, e: DragEvent) {
  dragOver.value = null
  const taskId = e.dataTransfer?.getData('text/plain')
  if (taskId) taskStore.moveTaskToStage(taskId, stageId)
}
</script>

<template>
  <div class="min-w-max pb-4">
    <!-- Stage header row stays visible while lanes scroll -->
    <div class="sticky top-0 z-20 flex gap-3 pb-2 bg-slate-950">
      <div
        v-for="stage in stages"
        :key="stage.id"
        :class="cellWidth()"
        class="px-3 py-2 rounded-lg bg-slate-900/80 border border-slate-800 text-xs font-semibold text-slate-200 truncate"
      >
        {{ stage.name }}
      </div>
    </div>

    <p v-if="lanes.length === 0" class="py-10 text-center text-xs text-slate-500">No tasks match the current filters.</p>

    <section v-for="lane in lanes" :key="lane.key" class="mb-3">
      <!-- Lane header -->
      <div class="sticky left-0 flex items-center gap-2 py-1.5 w-max max-w-[calc(100vw-4rem)]">
        <button
          type="button"
          @click="collapsed[lane.key] = !collapsed[lane.key]"
          :aria-expanded="!collapsed[lane.key]"
          class="flex items-center gap-2 text-xs text-slate-200 hover:text-white"
        >
          <ChevronRight v-if="collapsed[lane.key]" class="w-3.5 h-3.5 text-slate-500" />
          <ChevronDown v-else class="w-3.5 h-3.5 text-slate-500" />
          <Layers class="w-3.5 h-3.5" :class="lane.key === taskStore.NO_EPIC ? 'text-slate-500' : 'text-violet-400'" />
          <span class="font-semibold truncate max-w-[420px]">{{ lane.name }}</span>
        </button>
        <a
          v-if="lane.key !== taskStore.NO_EPIC && lane.url"
          :href="lane.url"
          target="_blank"
          rel="noopener"
          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-violet-950/70 border border-violet-800/70 text-violet-300 hover:text-violet-100 text-[10px] font-mono"
        >
          {{ lane.key }} <ExternalLink class="w-2.5 h-2.5" />
        </a>
        <span class="text-[11px] text-slate-500">{{ lane.completed }}/{{ lane.total }} done</span>
        <div class="w-20 h-1 rounded-full bg-slate-800 overflow-hidden" aria-hidden="true">
          <div class="h-full bg-emerald-500" :style="{ width: `${lane.total ? (lane.completed / lane.total) * 100 : 0}%` }"></div>
        </div>
      </div>

      <!-- Lane cells, one per stage -->
      <div v-if="!collapsed[lane.key]" class="flex gap-3">
        <div
          v-for="stage in stages"
          :key="stage.id"
          :class="[cellWidth(), dragOver === lane.key + stage.id ? 'border-emerald-500 bg-emerald-950/20' : 'border-slate-800/70 bg-slate-900/30']"
          class="min-h-[72px] p-2 rounded-lg border border-dashed space-y-2"
          @dragover.prevent="dragOver = lane.key + stage.id"
          @dragleave="dragOver = null"
          @drop="drop(stage.id, $event)"
        >
          <KanbanCard
            v-for="task in lane.byStage[stage.id] || []"
            :key="task.id"
            :task="task"
            :density="density"
            :all-stages="stages"
            @open-console="(id) => emit('open-console', id)"
            @request-delete="(id) => emit('request-delete', id)"
          />
        </div>
      </div>
    </section>
  </div>
</template>
