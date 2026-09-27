<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../../services/api'
import { wsService } from '../../services/websocket'
import { useToastStore } from '../../stores/toast'
import type { TaskProcessDTO } from '../../types'
import {
  Terminal, X, Copy, Check, Download, Trash2, ArrowDownCircle,
  ExternalLink, Play, RotateCcw, Cpu, HardDrive, Clock, Activity,
  Folder, ShieldAlert, Sparkles, Send
} from 'lucide-vue-next'

const props = defineProps<{
  taskId: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()
const toastStore = useToastStore()

const isLoading = ref(true)
const processInfo = ref<TaskProcessDTO | null>(null)
const logContainer = ref<HTMLDivElement | null>(null)
const isAutoScroll = ref(true)
const copiedCommand = ref(false)
const copiedLogs = ref(false)
const commandInput = ref('')
const isExecuting = ref(false)

let unsubscribeWS: (() => void) | null = null
let pollTimer: any = null

// Parse ANSI terminal color codes to HTML spans
function parseAnsi(text: string): string {
  if (!text) return ''
  let out = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

  // Standard ANSI colors
  out = out.replace(/\x1b\[31;1m|\x1b\[1;31m/g, '<span class="text-rose-400 font-bold">')
  out = out.replace(/\x1b\[31m/g, '<span class="text-rose-400">')
  out = out.replace(/\x1b\[32m/g, '<span class="text-emerald-400">')
  out = out.replace(/\x1b\[33m/g, '<span class="text-amber-400">')
  out = out.replace(/\x1b\[34m/g, '<span class="text-sky-400">')
  out = out.replace(/\x1b\[35m/g, '<span class="text-purple-400">')
  out = out.replace(/\x1b\[36m/g, '<span class="text-cyan-400">')
  out = out.replace(/\x1b\[37m/g, '<span class="text-slate-200">')
  out = out.replace(/\x1b\[90m/g, '<span class="text-slate-500">')
  out = out.replace(/\x1b\[0m/g, '</span>')
  // Strip any remaining unhandled escape codes
  out = out.replace(/\x1b\[[0-9;]*m/g, '')

  return out
}

async function loadProcess() {
  try {
    const data = await api.getTaskProcess(props.taskId)
    processInfo.value = data
    if (isAutoScroll.value) {
      await nextTick()
      scrollToBottom()
    }
  } catch (err: any) {
    console.error('Failed to load process:', err)
  } finally {
    isLoading.value = false
  }
}

function scrollToBottom() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

function handleScroll() {
  if (!logContainer.value) return
  const { scrollTop, scrollHeight, clientHeight } = logContainer.value
  isAutoScroll.value = scrollHeight - (scrollTop + clientHeight) < 40
}

async function copyCommand() {
  if (!processInfo.value?.command) return
  try {
    await navigator.clipboard.writeText(processInfo.value.command)
    copiedCommand.value = true
    setTimeout(() => { copiedCommand.value = false }, 1800)
    toastStore.info('Copied', 'Command copied to clipboard')
  } catch {
    // fallback
  }
}

async function copyAllLogs() {
  if (!processInfo.value?.logs) return
  const plainText = processInfo.value.logs.join('\n').replace(/\x1b\[[0-9;]*m/g, '')
  try {
    await navigator.clipboard.writeText(plainText)
    copiedLogs.value = true
    setTimeout(() => { copiedLogs.value = false }, 1800)
    toastStore.success('Logs Copied', 'All terminal output copied to clipboard')
  } catch {
    // fallback
  }
}

function clearConsole() {
  if (processInfo.value) {
    processInfo.value.logs = []
    toastStore.info('Console Cleared', 'Terminal buffer cleared')
  }
}

function downloadLogFile() {
  if (!processInfo.value?.logs) return
  const plainText = processInfo.value.logs.join('\n').replace(/\x1b\[[0-9;]*m/g, '')
  const blob = new Blob([plainText], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${props.taskId}-process-terminal-${Date.now()}.log`
  a.click()
  URL.revokeObjectURL(url)
  toastStore.success('Downloaded', 'Saved terminal log file')
}

async function executeCommand(cmd?: string) {
  const toRun = (cmd || commandInput.value).trim()
  if (!toRun) return

  isExecuting.value = true
  commandInput.value = ''
  try {
    const res = await api.executeTaskProcessCommand(props.taskId, toRun)
    if (res.process) {
      processInfo.value = res.process
    }
    await nextTick()
    scrollToBottom()
  } catch (err: any) {
    toastStore.error('Execution Failed', err.message || 'Error sending command')
  } finally {
    isExecuting.value = false
  }
}

function openFullWorkspace() {
  emit('close')
  router.push(`/tasks/${props.taskId}`)
}

// Format duration into mm:ss or hh:mm:ss
const formattedRuntime = computed(() => {
  if (!processInfo.value?.duration_seconds) return '00:00'
  const secs = processInfo.value.duration_seconds
  const m = Math.floor(secs / 60)
  const s = secs % 60
  if (m < 60) {
    return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
  }
  const h = Math.floor(m / 60)
  const remM = m % 60
  return `${h}h ${remM}m`
})

onMounted(async () => {
  await loadProcess()

  // Real-time WebSocket updates for this task
  unsubscribeWS = wsService.subscribe((event) => {
    if (event.task_id && event.task_id !== props.taskId) return

    if (event.type === 'agent.terminal' && event.payload?.chunk && processInfo.value) {
      processInfo.value.logs.push(event.payload.chunk)
      if (isAutoScroll.value) {
        nextTick(scrollToBottom)
      }
    } else if (event.type === 'task.status' && event.payload) {
      loadProcess()
    }
  })

  // Poll duration & status update every 5 seconds if running
  pollTimer = setInterval(() => {
    if (processInfo.value?.status === 'RUNNING') {
      processInfo.value.duration_seconds++
    }
  }, 1000)

  // Escape key closes
  const handleKeydown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') emit('close')
  }
  window.addEventListener('keydown', handleKeydown)
  onUnmounted(() => {
    window.removeEventListener('keydown', handleKeydown)
    if (unsubscribeWS) unsubscribeWS()
    if (pollTimer) clearInterval(pollTimer)
  })
})
</script>

<template>
  <div class="fixed inset-0 z-50 overflow-hidden flex justify-end">
    <!-- Backdrop -->
    <div
      @click="emit('close')"
      class="fixed inset-0 bg-slate-950/80 backdrop-blur-sm transition-opacity"
    ></div>

    <!-- Drawer Panel -->
    <div class="relative w-full max-w-3xl bg-slate-950 border-l border-slate-800 shadow-2xl flex flex-col h-full z-10 animate-in slide-in-from-right duration-200">
      <!-- 1. Header Toolbar -->
      <div class="h-14 px-5 bg-slate-900 border-b border-slate-800 flex items-center justify-between flex-shrink-0">
        <div class="flex items-center gap-3">
          <div class="w-8 h-8 rounded-lg bg-emerald-950/80 border border-emerald-800/80 flex items-center justify-center text-emerald-400">
            <Terminal class="w-4 h-4" />
          </div>
          <div>
            <div class="flex items-center gap-2">
              <span class="text-sm font-semibold text-slate-100">Task Background Process & Console</span>
              <span class="font-mono text-xs font-bold text-emerald-400 bg-emerald-950/80 px-2 py-0.5 rounded border border-emerald-800/60">
                {{ taskId }}
              </span>
            </div>
            <div class="text-[11px] text-slate-400 flex items-center gap-1.5 mt-0.5 font-mono">
              <Folder class="w-3 h-3 text-slate-500" />
              <span>Container: {{ processInfo?.container_id || 'sandbox' }}</span>
              <span class="text-slate-600">•</span>
              <span>PID: {{ processInfo?.process_id || '—' }}</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <!-- Status Badge -->
          <div
            v-if="processInfo"
            class="px-2.5 py-1 rounded-full text-xs font-mono font-medium flex items-center gap-1.5"
            :class="{
              'bg-emerald-950 text-emerald-300 border border-emerald-800/80': processInfo.status === 'RUNNING',
              'bg-rose-950 text-rose-300 border border-rose-800/80': processInfo.status === 'BLOCKED' || processInfo.status === 'FAILED',
              'bg-amber-950 text-amber-300 border border-amber-800/80': processInfo.status === 'PAUSED',
              'bg-blue-950 text-blue-300 border border-blue-800/80': processInfo.status === 'COMPLETED',
            }"
          >
            <span
              class="w-2 h-2 rounded-full"
              :class="{
                'bg-emerald-400 animate-pulse': processInfo.status === 'RUNNING',
                'bg-rose-400': processInfo.status === 'BLOCKED' || processInfo.status === 'FAILED',
                'bg-amber-400': processInfo.status === 'PAUSED',
                'bg-blue-400': processInfo.status === 'COMPLETED',
              }"
            ></span>
            <span>{{ processInfo.status }}</span>
          </div>

          <!-- Close Button -->
          <button
            @click="emit('close')"
            type="button"
            class="p-1.5 rounded-lg text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition-colors"
            title="Close Drawer (Esc)"
          >
            <X class="w-5 h-5" />
          </button>
        </div>
      </div>

      <!-- 2. Active Process Telemetry Banner -->
      <div v-if="processInfo" class="px-5 py-3 bg-slate-900/60 border-b border-slate-800 space-y-2.5 flex-shrink-0">
        <!-- Command Line Strip with Copy -->
        <div class="flex items-center justify-between gap-2 p-2 rounded-lg bg-slate-950 border border-slate-800/90 font-mono text-xs text-slate-200">
          <div class="flex items-center gap-2 truncate flex-1 min-w-0">
            <span class="text-emerald-400 font-bold select-none">$</span>
            <span class="truncate select-all text-slate-100">{{ processInfo.command }}</span>
          </div>
          <button
            @click="copyCommand"
            type="button"
            title="Copy command"
            class="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors flex-shrink-0 flex items-center gap-1 text-[11px]"
          >
            <Check v-if="copiedCommand" class="w-3.5 h-3.5 text-emerald-400" />
            <Copy v-else class="w-3.5 h-3.5" />
            <span class="hidden sm:inline">{{ copiedCommand ? 'Copied' : 'Copy' }}</span>
          </button>
        </div>

        <!-- Telemetry Metrics Grid -->
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs font-mono">
          <div class="p-2 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <Activity class="w-3 h-3 text-cyan-400" />
              <span>PID & Subprocess</span>
            </div>
            <div class="font-semibold text-slate-200 mt-0.5 truncate">
              #{{ processInfo.process_id }}
              <span v-if="processInfo.subprocesses?.length" class="text-slate-400 font-normal text-[11px]">
                (+{{ processInfo.subprocesses.length }})
              </span>
            </div>
          </div>

          <div class="p-2 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <Clock class="w-3 h-3 text-emerald-400" />
              <span>Elapsed Runtime</span>
            </div>
            <div class="font-semibold text-slate-200 mt-0.5">
              {{ formattedRuntime }}
            </div>
          </div>

          <div class="p-2 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <Cpu class="w-3 h-3 text-purple-400" />
              <span>CPU Usage</span>
            </div>
            <div class="font-semibold text-slate-200 mt-0.5">
              {{ processInfo.cpu_percent }}%
            </div>
          </div>

          <div class="p-2 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <HardDrive class="w-3 h-3 text-amber-400" />
              <span>Resident RAM</span>
            </div>
            <div class="font-semibold text-slate-200 mt-0.5">
              {{ processInfo.memory_mb }} MB
            </div>
          </div>
        </div>

        <!-- Current Step Info Pill -->
        <div class="flex items-center justify-between text-[11px] font-mono text-slate-400 pt-0.5">
          <div class="flex items-center gap-1.5 truncate">
            <span class="text-slate-500">Active Step:</span>
            <span class="text-slate-300 font-medium truncate">{{ processInfo.current_step }}</span>
          </div>
          <span class="text-slate-500 truncate max-w-[200px]">{{ processInfo.working_dir }}</span>
        </div>
      </div>

      <!-- 3. Terminal Window Body -->
      <div class="flex-1 flex flex-col min-h-0 bg-slate-950">
        <!-- Terminal Sub-Header -->
        <div class="h-8 px-4 bg-slate-900/90 border-b border-slate-800 flex items-center justify-between text-xs font-mono text-slate-400 flex-shrink-0">
          <div class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
            <span class="text-[11px] font-medium text-slate-300">LIVE CONSOLE STREAM (STDOUT / STDERR)</span>
          </div>

          <div class="flex items-center gap-1.5">
            <button
              v-if="!isAutoScroll"
              @click="isAutoScroll = true; scrollToBottom()"
              type="button"
              class="px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 border border-emerald-800 text-[10px] flex items-center gap-1 animate-pulse"
            >
              <ArrowDownCircle class="w-3 h-3" />
              <span>Auto-Scroll</span>
            </button>

            <button
              @click="copyAllLogs"
              type="button"
              title="Copy all logs"
              class="p-1 rounded hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors"
            >
              <Copy class="w-3.5 h-3.5" />
            </button>

            <button
              @click="downloadLogFile"
              type="button"
              title="Download execution logs"
              class="p-1 rounded hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors"
            >
              <Download class="w-3.5 h-3.5" />
            </button>

            <button
              @click="clearConsole"
              type="button"
              title="Clear terminal"
              class="p-1 rounded hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>

        <!-- Terminal Output Area -->
        <div
          ref="logContainer"
          @scroll="handleScroll"
          class="flex-1 p-4 overflow-y-auto font-mono text-xs text-slate-200 bg-[#020617] space-y-1 select-text leading-relaxed"
        >
          <div v-if="isLoading" class="flex items-center justify-center h-full text-slate-500 gap-2">
            <RotateCcw class="w-4 h-4 animate-spin text-emerald-400" />
            <span>Connecting to background process stream...</span>
          </div>

          <template v-else-if="processInfo?.logs && processInfo.logs.length > 0">
            <div
              v-for="(line, idx) in processInfo.logs"
              :key="idx"
              v-html="parseAnsi(line)"
              class="whitespace-pre-wrap break-all"
            ></div>
          </template>

          <div v-else class="text-slate-600 italic py-8 text-center">
            No console output recorded yet for this task process.
          </div>
        </div>

        <!-- 4. Interactive Quick Actions & Prompt Steering Bar -->
        <div class="p-3 bg-slate-900 border-t border-slate-800 flex-shrink-0 space-y-2">
          <!-- Quick Action Buttons -->
          <div class="flex items-center gap-1.5 flex-wrap">
            <span class="text-[10px] font-mono text-slate-500 uppercase tracking-wider">Quick Actions:</span>
            <button
              @click="executeCommand('go test -v ./...')"
              type="button"
              class="h-6 px-2 rounded bg-slate-950 border border-slate-800 hover:border-emerald-500/80 text-slate-300 hover:text-emerald-300 text-[10px] font-mono flex items-center gap-1 transition-all"
            >
              <Play class="w-2.5 h-2.5 text-emerald-400" />
              <span>Run Tests</span>
            </button>

            <button
              @click="executeCommand('git status -s')"
              type="button"
              class="h-6 px-2 rounded bg-slate-950 border border-slate-800 hover:border-sky-500/80 text-slate-300 hover:text-sky-300 text-[10px] font-mono flex items-center gap-1 transition-all"
            >
              <span>Git Status</span>
            </button>

            <button
              @click="executeCommand('ps aux --status')"
              type="button"
              class="h-6 px-2 rounded bg-slate-950 border border-slate-800 hover:border-purple-500/80 text-slate-300 hover:text-purple-300 text-[10px] font-mono flex items-center gap-1 transition-all"
            >
              <Activity class="w-2.5 h-2.5 text-purple-400" />
              <span>Check Process</span>
            </button>
          </div>

          <!-- Command Input -->
          <form @submit.prevent="executeCommand()" class="flex items-center gap-2">
            <div class="relative flex-1">
              <input
                v-model="commandInput"
                type="text"
                :disabled="isExecuting"
                placeholder="Type shell command or steering instruction... (e.g. go test, npm run lint)"
                class="w-full h-9 pl-8 pr-3 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-slate-200 placeholder:text-slate-600 focus:outline-none focus:border-emerald-500 transition-colors"
              />
              <span class="absolute left-3 top-2.5 text-slate-500 font-mono text-xs select-none">$</span>
            </div>

            <button
              type="submit"
              :disabled="!commandInput.trim() || isExecuting"
              class="h-9 px-4 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 disabled:cursor-not-allowed text-white font-medium text-xs flex items-center gap-1.5 transition-colors shadow-sm"
            >
              <Send class="w-3.5 h-3.5" />
              <span>Run</span>
            </button>
          </form>
        </div>
      </div>

      <!-- 5. Bottom Navigation Bar -->
      <div class="h-12 px-5 bg-slate-950 border-t border-slate-800 flex items-center justify-between flex-shrink-0">
        <span class="text-xs text-slate-500 font-mono">
          Press <kbd class="px-1.5 py-0.5 rounded bg-slate-800 text-[10px] text-slate-400 border border-slate-700">ESC</kbd> to close
        </span>

        <button
          @click="openFullWorkspace"
          type="button"
          class="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-400 hover:text-emerald-300 hover:underline transition-colors"
        >
          <span>Open Full Task Workspace</span>
          <ExternalLink class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  </div>
</template>
