<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../../services/api'
import { wsService } from '../../services/websocket'
import { useTaskStore } from '../../stores/tasks'
import type { TaskProcessDTO } from '../../types'
import AgentConsole from '../console/AgentConsole.vue'
import { Terminal, X, ExternalLink, Folder, ChevronDown, ChevronUp } from 'lucide-vue-next'

const props = defineProps<{
  taskId: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()
const taskStore = useTaskStore()

const processInfo = ref<TaskProcessDTO | null>(null)
const showProcess = ref(false)

const currentTask = computed(() => taskStore.tasks.find(t => t.id === props.taskId))

async function loadProcess() {
  try {
    processInfo.value = await api.getTaskProcess(props.taskId)
  } catch {
    processInfo.value = null
  }
}

function openFullWorkspace() {
  emit('close')
  router.push(`/tasks/${props.taskId}`)
}

// Esc closes the drawer unless the console consumed it (interrupt / clear input / gate).
function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && !e.defaultPrevented) emit('close')
}

let unsubscribeWS: (() => void) | null = null

onMounted(() => {
  loadProcess()
  unsubscribeWS = wsService.subscribe(event => {
    if (event.type === 'task.status' && event.payload?.id === props.taskId) loadProcess()
  })
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  if (unsubscribeWS) unsubscribeWS()
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex justify-end overflow-hidden">
    <!-- Backdrop -->
    <div
      class="fixed inset-0 bg-slate-950/80 backdrop-blur-sm transition-opacity"
      @click="emit('close')"
    ></div>

    <!-- Drawer Panel -->
    <div
      class="relative z-10 flex h-full w-full max-w-3xl flex-col border-l border-slate-800 bg-slate-950 shadow-2xl"
      role="dialog"
      aria-modal="true"
      :aria-label="`Agent console for ${taskId}`"
    >
      <!-- Header -->
      <div class="flex h-14 flex-shrink-0 items-center justify-between border-b border-slate-800 bg-slate-900 px-5">
        <div class="flex min-w-0 items-center gap-3">
          <div class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg border border-emerald-800/80 bg-emerald-950/80 text-emerald-400">
            <Terminal class="h-4 w-4" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-sm font-semibold text-slate-100">Agent Console</span>
              <span class="rounded border border-emerald-800/60 bg-emerald-950/80 px-2 py-0.5 font-mono text-xs font-bold text-emerald-400">
                {{ taskId }}
              </span>
            </div>
            <div class="mt-0.5 truncate text-[11px] text-slate-400" :title="currentTask?.title">
              {{ currentTask?.title || '—' }}
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            v-if="processInfo"
            type="button"
            class="flex items-center gap-1 rounded px-2 py-1 font-mono text-[11px] text-slate-400 hover:bg-slate-800 hover:text-slate-200 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
            :aria-expanded="showProcess"
            title="Background process details"
            @click="showProcess = !showProcess"
          >
            <span>process</span>
            <ChevronUp v-if="showProcess" class="h-3 w-3" />
            <ChevronDown v-else class="h-3 w-3" />
          </button>
          <button
            type="button"
            class="rounded-lg p-1.5 text-slate-400 transition-colors hover:bg-slate-800 hover:text-slate-100 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
            title="Close drawer (Esc)"
            aria-label="Close drawer"
            @click="emit('close')"
          >
            <X class="h-5 w-5" />
          </button>
        </div>
      </div>

      <!-- Background process details (reported by GET /tasks/{id}/process) -->
      <div
        v-if="showProcess && processInfo"
        class="flex-shrink-0 space-y-1 border-b border-slate-800 bg-slate-900/60 px-5 py-2 font-mono text-[11px] text-slate-400"
      >
        <div class="flex items-center gap-2 truncate">
          <span class="font-bold text-emerald-400">$</span>
          <span class="truncate text-slate-200" :title="processInfo.command">{{ processInfo.command || '—' }}</span>
        </div>
        <div class="flex flex-wrap gap-x-3 gap-y-0.5">
          <span>status <span class="text-slate-200">{{ processInfo.status }}</span></span>
          <span>step <span class="text-slate-200">{{ processInfo.current_step || '—' }}</span></span>
          <span class="flex items-center gap-1 min-w-0" title="Directory the agent works in">
            <Folder class="h-3 w-3 flex-shrink-0" />
            <span class="text-slate-200 truncate">{{ processInfo.working_dir || 'set on first run' }}</span>
          </span>
        </div>
      </div>

      <!-- Console body -->
      <div class="min-h-0 flex-1">
        <AgentConsole :task-id="taskId" :show-header="true" @task-updated="loadProcess" />
      </div>

      <!-- Bottom Navigation Bar -->
      <div class="flex h-10 flex-shrink-0 items-center justify-between border-t border-slate-800 bg-slate-950 px-5">
        <span class="font-mono text-[11px] text-slate-500">
          <kbd class="rounded border border-slate-700 bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-400">ESC</kbd>
          interrupts a running turn, otherwise closes
        </span>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded text-xs font-medium text-emerald-400 transition-colors hover:text-emerald-300 hover:underline focus:outline-none focus-visible:ring-1 focus-visible:ring-emerald-500"
          @click="openFullWorkspace"
        >
          <span>Open Full Task Workspace</span>
          <ExternalLink class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  </div>
</template>
