<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../../services/api'
import { wsService } from '../../services/websocket'
import { useToastStore } from '../../stores/toast'
import { useTaskStore } from '../../stores/tasks'
import type { TaskProcessDTO } from '../../types'
import {
  Terminal, X, Copy, Check, Download, Trash2, ArrowDownCircle,
  ExternalLink, Play, Pause, RotateCcw, Cpu, HardDrive, Clock, Activity,
  Folder, ShieldAlert, Sparkles, Send, AlertCircle, AlertTriangle,
  CheckCircle2, XCircle
} from 'lucide-vue-next'

const props = defineProps<{
  taskId: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()
const toastStore = useToastStore()
const taskStore = useTaskStore()

const isLoading = ref(true)
const isExecutingAI = ref(false)
const isResuming = ref(false)
const isPausing = ref(false)
const isExecuting = ref(false)
const processInfo = ref<TaskProcessDTO | null>(null)
const logContainer = ref<HTMLDivElement | null>(null)
const isAutoScroll = ref(true)
const copiedCommand = ref(false)
const copiedLogs = ref(false)
const commandInput = ref('')
const commandHistory = ref<string[]>([])
const historyIndex = ref(-1)

const currentTask = computed(() => taskStore.tasks.find(t => t.id === props.taskId))

// Check if interactive confirmation or gate approval is pending from the operator
const isConfirmationNeeded = computed(() => {
  if (currentTask.value?.state === 'WAITING_GATE_APPROVAL') return true
  const logs = processInfo.value?.logs || []
  if (logs.length === 0) return false
  const lastLog = logs[logs.length - 1].toLowerCase()
  return (
    lastLog.includes('[y/n]') ||
    lastLog.includes('(y/n)') ||
    lastLog.includes('proceed?') ||
    lastLog.includes('confirm?') ||
    lastLog.includes('waiting for operator') ||
    lastLog.includes('approval required')
  )
})

const confirmationPrompt = computed(() => {
  if (currentTask.value?.state === 'WAITING_GATE_APPROVAL') {
    return `Milestone Gate Review: Operator approval required to advance stage to ${currentTask.value.current_stage_id}`
  }
  const logs = processInfo.value?.logs || []
  if (logs.length > 0) {
    return logs[logs.length - 1].replace(/\x1b\[[0-9;]*m/g, '')
  }
  return 'Operator confirmation required to proceed'
})

// Trigger manual run
async function handleRunAI() {
  if (isExecutingAI.value) return
  isExecutingAI.value = true
  try {
    await taskStore.executeTask(props.taskId)
    await loadProcess()
  } catch (err: any) {
    console.error('Run AI error:', err)
  } finally {
    setTimeout(() => {
      isExecutingAI.value = false
    }, 1500)
  }
}

// Resume paused / interrupted session completely
async function handleResume() {
  if (isResuming.value) return
  isResuming.value = true
  try {
    await taskStore.resumeTask(props.taskId)
    await loadProcess()
  } catch (err: any) {
    console.error('Resume error:', err)
  } finally {
    setTimeout(() => {
      isResuming.value = false
    }, 1000)
  }
}

// Pause running session while preserving state and logs
async function handlePause() {
  if (isPausing.value) return
  isPausing.value = true
  try {
    await taskStore.pauseTask(props.taskId)
    await loadProcess()
  } catch (err: any) {
    console.error('Pause error:', err)
  } finally {
    setTimeout(() => {
      isPausing.value = false
    }, 1000)
  }
}

// Operator confirmation handler (y / n)
async function handleConfirm(approved: boolean) {
  if (currentTask.value?.state === 'WAITING_GATE_APPROVAL') {
    try {
      await taskStore.updateGate(
        props.taskId,
        approved,
        approved ? 'Approved by operator via Termux console' : 'Rejected by operator'
      )
    } catch (e) {
      console.warn('Gate update notice:', e)
    }
  }
  await executeCommand(approved ? 'y' : 'n')
}

let unsubscribeWS: (() => void) | null = null
let pollTimer: any = null

// Parse ANSI terminal color codes to HTML spans with Monokai / Termux colors
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
  out = out.replace(/\x1b\[[0-9;]*m/g, '')

  return out
}

// Fetch process without wiping existing logs unless explicitly cleared
async function loadProcess() {
  try {
    const data = await api.getTaskProcess(props.taskId)
    if (!processInfo.value) {
      processInfo.value = data
    } else {
      processInfo.value.status = data.status
      processInfo.value.current_step = data.current_step
      processInfo.value.duration_seconds = data.duration_seconds
      processInfo.value.cpu_percent = data.cpu_percent
      processInfo.value.memory_mb = data.memory_mb
      processInfo.value.command = data.command
      processInfo.value.process_id = data.process_id
      if (data.logs && data.logs.length > 0) {
        processInfo.value.logs = data.logs
      }
    }
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

// Manual Clear Only: Clears logs explicitly on backend and frontend
async function clearConsole() {
  try {
    await api.clearTaskProcessLogs(props.taskId)
    if (processInfo.value) {
      processInfo.value.logs = []
    }
    toastStore.info('Console Cleared', 'Terminal buffer cleared manually')
  } catch {
    if (processInfo.value) {
      processInfo.value.logs = []
    }
  }
}

function downloadLogFile() {
  if (!processInfo.value?.logs) return
  const plainText = processInfo.value.logs.join('\n').replace(/\x1b\[[0-9;]*m/g, '')
  const blob = new Blob([plainText], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${props.taskId}-termux-session-${Date.now()}.log`
  a.click()
  URL.revokeObjectURL(url)
  toastStore.success('Downloaded', 'Saved terminal log file')
}

// History navigation for Termux input
function navigateHistory(direction: 'up' | 'down') {
  if (commandHistory.value.length === 0) return
  if (direction === 'up') {
    if (historyIndex.value === -1) {
      historyIndex.value = commandHistory.value.length - 1
    } else if (historyIndex.value > 0) {
      historyIndex.value--
    }
  } else {
    if (historyIndex.value !== -1) {
      if (historyIndex.value < commandHistory.value.length - 1) {
        historyIndex.value++
      } else {
        historyIndex.value = -1
        commandInput.value = ''
        return
      }
    }
  }
  if (historyIndex.value >= 0 && historyIndex.value < commandHistory.value.length) {
    commandInput.value = commandHistory.value[historyIndex.value]
  }
}

// Virtual key shortcuts for Termux bottom bar
function handleVirtualKey(key: string) {
  switch (key) {
    case 'ESC':
      commandInput.value = ''
      break
    case 'TAB':
      if (!commandInput.value) {
        commandInput.value = 'status'
      } else if (commandInput.value.startsWith('r')) {
        commandInput.value = 'resume'
      } else if (commandInput.value.startsWith('t')) {
        commandInput.value = 'test'
      } else if (commandInput.value.startsWith('g')) {
        commandInput.value = 'git status'
      } else if (commandInput.value.startsWith('c')) {
        commandInput.value = 'clear'
      }
      break
    case 'CTRL_C':
      executeCommand('^C')
      break
  }
}

async function executeCommand(cmd?: string) {
  const toRun = (cmd || commandInput.value).trim()
  if (!toRun) return

  if (!cmd) {
    commandHistory.value.push(toRun)
    historyIndex.value = -1
  }

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

  // Real-time WebSocket updates for this task (preserves logs completely)
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

  // Poll duration every second; sync logs/status every 2 seconds if running
  let pollCycle = 0
  pollTimer = setInterval(() => {
    if (processInfo.value?.status === 'RUNNING') {
      processInfo.value.duration_seconds++
      pollCycle++
      if (pollCycle % 2 === 0) {
        loadProcess()
      }
    }
  }, 1000)

  // Escape key closes drawer
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
              <span class="text-sm font-semibold text-slate-100">Termux SDLC Shell</span>
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
          <!-- Dynamic Action Button: Resume / Pause / Run Stage -->
          <button
            v-if="processInfo?.status === 'PAUSED' || currentTask?.state === 'SUSPENDED' || currentTask?.state === 'BLOCKED_FRUSTRATION'"
            @click="handleResume"
            :disabled="isResuming"
            type="button"
            class="h-7 px-2.5 rounded-lg text-xs font-mono font-medium flex items-center gap-1.5 transition-all shadow-md bg-emerald-600 hover:bg-emerald-500 text-white active:scale-95"
            title="Resume session execution with full context preserved"
          >
            <span v-if="isResuming" class="animate-spin text-white">⟳</span>
            <Play v-else class="w-3 h-3 fill-current" />
            <span>{{ isResuming ? 'Resuming...' : 'Resume Session' }}</span>
          </button>

          <button
            v-else-if="processInfo?.status === 'RUNNING'"
            @click="handlePause"
            :disabled="isPausing"
            type="button"
            class="h-7 px-2.5 rounded-lg text-xs font-mono font-medium flex items-center gap-1.5 transition-all bg-amber-950/80 hover:bg-amber-900 border border-amber-700/80 text-amber-200 active:scale-95"
            title="Pause active session while preserving all terminal logs"
          >
            <Pause class="w-3 h-3 text-amber-400" />
            <span>{{ isPausing ? 'Pausing...' : 'Pause' }}</span>
          </button>

          <button
            v-else
            @click="handleRunAI"
            :disabled="isExecutingAI"
            type="button"
            class="h-7 px-2.5 rounded-lg text-xs font-mono font-medium flex items-center gap-1.5 transition-all shadow-md bg-emerald-600 hover:bg-emerald-500 text-white active:scale-95"
            title="Execute stage using 9Router multi-provider cascade"
          >
            <span v-if="isExecutingAI" class="animate-spin text-white">⟳</span>
            <Play v-else class="w-3 h-3 fill-current" />
            <span>{{ isExecutingAI ? 'Routing...' : 'Run Stage' }}</span>
          </button>

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

      <!-- Live Active Execution Streaming Banner -->
      <div
        v-if="processInfo?.status === 'RUNNING' || isExecutingAI"
        class="px-5 py-2.5 bg-gradient-to-r from-emerald-950/80 via-slate-900 to-emerald-950/80 border-b border-emerald-500/30 flex items-center justify-between text-xs font-mono text-emerald-400 animate-pulse flex-shrink-0"
      >
        <div class="flex items-center gap-2">
          <span class="relative flex h-2.5 w-2.5">
            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500"></span>
          </span>
          <span class="font-semibold text-emerald-300">AI Agent Session Active</span>
          <span class="text-slate-400 hidden sm:inline">• 9Router Cascade Routing</span>
        </div>
        <div class="flex items-center gap-2 text-slate-300 text-[11px]">
          <span>Elapsed: {{ formattedRuntime }}</span>
        </div>
      </div>

      <!-- Blocked Dependency Banner -->
      <div
        v-else-if="processInfo?.status === 'BLOCKED'"
        class="px-5 py-2.5 bg-rose-950/70 border-b border-rose-500/30 flex items-center justify-between text-xs font-mono text-rose-300 flex-shrink-0"
      >
        <div class="flex items-center gap-2">
          <AlertCircle class="w-4 h-4 text-rose-400 flex-shrink-0" />
          <span class="font-semibold">Execution Blocked: Unmet Dependencies</span>
        </div>
        <span class="text-rose-400/80 text-[11px]">Prerequisite tasks must reach COMPLETED first</span>
      </div>

      <!-- 2. Active Process Telemetry Banner -->
      <div v-if="processInfo" class="px-5 py-2.5 bg-slate-900/60 border-b border-slate-800 space-y-2 flex-shrink-0">
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
          <div class="p-1.5 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <Clock class="w-3 h-3 text-slate-500" />
              <span>DURATION</span>
            </div>
            <div class="text-slate-100 font-semibold mt-0.5">{{ formattedRuntime }}</div>
          </div>

          <div class="p-1.5 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <Cpu class="w-3 h-3 text-slate-500" />
              <span>CPU LOAD</span>
            </div>
            <div class="text-emerald-400 font-semibold mt-0.5">{{ processInfo.cpu_percent }}%</div>
          </div>

          <div class="p-1.5 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <HardDrive class="w-3 h-3 text-slate-500" />
              <span>MEM USAGE</span>
            </div>
            <div class="text-cyan-400 font-semibold mt-0.5">{{ processInfo.memory_mb }} MB</div>
          </div>

          <div class="p-1.5 rounded bg-slate-950/60 border border-slate-800/70">
            <div class="text-[10px] text-slate-400 flex items-center gap-1">
              <Activity class="w-3 h-3 text-slate-500" />
              <span>PIPELINE</span>
            </div>
            <div class="text-purple-300 font-semibold truncate mt-0.5" :title="processInfo.current_step">
              {{ processInfo.current_step }}
            </div>
          </div>
        </div>

        <!-- Active Background Worker Live Inspector Bar -->
        <div v-if="processInfo?.status === 'RUNNING'" class="p-2.5 rounded-lg bg-black/90 border border-emerald-500/40 font-mono text-[11px] space-y-1.5 shadow-inner">
          <div class="flex items-center justify-between text-emerald-400">
            <div class="flex items-center gap-2">
              <span class="relative flex h-2 w-2">
                <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              <span class="font-bold text-emerald-300">ACTIVE BACKGROUND WORKER:</span>
              <span class="text-slate-100 font-semibold truncate max-w-sm">{{ processInfo.current_step }}</span>
            </div>
            <span class="text-[10px] text-emerald-400 bg-emerald-950/90 px-2 py-0.5 rounded border border-emerald-700/80 font-mono">
              PID {{ processInfo.process_id }} • Live TTY
            </span>
          </div>

          <div v-if="processInfo.subprocesses && processInfo.subprocesses.length > 0" class="flex flex-wrap gap-1.5 pt-0.5">
            <div
              v-for="sub in processInfo.subprocesses"
              :key="sub.pid"
              class="px-2 py-0.5 rounded bg-slate-950 border border-slate-800 text-[10px] text-slate-300 flex items-center gap-1.5"
            >
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
              <span class="text-amber-300 font-mono">PID {{ sub.pid }}</span>
              <span class="text-slate-400 truncate max-w-xs">{{ sub.command }}</span>
              <span class="text-[9px] text-emerald-400 font-semibold uppercase">[{{ sub.status }}]</span>
            </div>
          </div>
          <div v-else class="text-[10px] text-slate-400 flex items-center gap-2">
            <span>Daemon Threads:</span>
            <span class="text-emerald-300/90 font-mono">PID {{ processInfo.process_id }} (orchestrator-agent)</span>
            <span class="text-slate-600">•</span>
            <span class="text-slate-300 font-mono">PID {{ processInfo.process_id + 1 }} (inotify-watcher)</span>
            <span class="text-slate-600">•</span>
            <span class="text-slate-300 font-mono">PID {{ processInfo.process_id + 2 }} (test-runner)</span>
          </div>
        </div>
      </div>

      <!-- 3. Terminal Window Body (100% Termux Look & Feel) -->
      <div class="flex-1 flex flex-col min-h-0 bg-black">
        <!-- Terminal Sub-Header -->
        <div class="h-8 px-4 bg-[#0a0f1d] border-b border-slate-800 flex items-center justify-between text-xs font-mono text-slate-400 flex-shrink-0">
          <div class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full" :class="processInfo?.status === 'RUNNING' ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400'"></span>
            <span class="text-[11px] font-medium text-slate-300 tracking-wide">TERMUX TTY1 STREAM (NO LOGS CLEARED)</span>
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
              title="Clear terminal buffer manually (Only manual clear wipes screen)"
              class="p-1 rounded hover:bg-slate-800 text-slate-400 hover:text-rose-400 transition-colors"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>

        <!-- Interactive Operator Confirmation Box (Termux Prompt / HITL Gate) -->
        <div
          v-if="isConfirmationNeeded"
          class="mx-4 my-2 p-3 rounded-lg bg-amber-950/70 border border-amber-500/60 shadow-lg shadow-amber-950/40 font-mono text-xs text-amber-200 flex-shrink-0 animate-in fade-in"
        >
          <div class="flex items-start gap-2.5">
            <AlertTriangle class="w-4 h-4 text-amber-400 mt-0.5 flex-shrink-0 animate-pulse" />
            <div class="flex-1 space-y-1.5 min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-bold text-amber-300 uppercase tracking-wide text-[11px]">
                  [Interactive Operator Confirmation Required]
                </span>
                <span class="px-1.5 py-0.5 rounded bg-amber-900/90 border border-amber-600/70 text-[10px] text-amber-200">
                  HITL Gate
                </span>
              </div>
              <p class="text-xs text-slate-100 whitespace-pre-wrap break-words leading-relaxed font-sans">
                {{ confirmationPrompt }}
              </p>
              <div class="flex items-center gap-2 pt-1 flex-wrap">
                <button
                  @click="handleConfirm(true)"
                  :disabled="isExecuting"
                  type="button"
                  class="h-7 px-3 rounded-md bg-emerald-600 hover:bg-emerald-500 active:scale-95 text-white font-semibold text-xs flex items-center gap-1.5 shadow transition-all"
                  title="Confirm and proceed with execution (shortcut: y)"
                >
                  <Check class="w-3.5 h-3.5" />
                  <span>Confirm / Proceed (y)</span>
                </button>
                <button
                  @click="handleConfirm(false)"
                  :disabled="isExecuting"
                  type="button"
                  class="h-7 px-3 rounded-md bg-rose-950/80 hover:bg-rose-900 border border-rose-700/80 text-rose-300 hover:text-white font-medium text-xs flex items-center gap-1.5 transition-all"
                  title="Reject or pause stage (shortcut: n)"
                >
                  <X class="w-3.5 h-3.5" />
                  <span>Reject / Pause (n)</span>
                </button>
                <span class="text-[10px] text-slate-400 hidden sm:inline">
                  (Or press virtual key 'y'/'n' below)
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- Terminal Output Area (Monokai Black Canvas) -->
        <div
          ref="logContainer"
          @scroll="handleScroll"
          class="flex-1 p-4 overflow-y-auto font-mono text-xs text-slate-200 bg-black space-y-1 select-text leading-relaxed"
        >
          <!-- Termux Welcome MOTD Banner -->
          <div class="p-3 mb-3 rounded bg-[#070b14] border border-slate-800 text-xs font-mono space-y-1">
            <div class="text-emerald-400 font-bold flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Terminal class="w-3.5 h-3.5 text-emerald-400" />
                <span>META-ORCHESTRATOR TERMUX SANDBOX</span>
              </div>
              <span class="text-[10px] text-emerald-500/80 font-mono">v2.4.0-linux</span>
            </div>
            <div class="text-slate-400 text-[11px] leading-relaxed">
              * Session: <span :class="processInfo?.status === 'RUNNING' ? 'text-emerald-400 font-bold' : 'text-amber-400 font-bold'">{{ processInfo?.status || 'IDLE' }}</span> (PID: <span class="text-amber-300">{{ processInfo?.process_id || '4900' }}</span>) | Task: <span class="text-cyan-300 font-bold">{{ taskId }}</span><br />
              * Sandbox Worktree: <span class="text-slate-300 font-mono">/workspaces/{{ taskId.toLowerCase() }}</span><br />
              * Shell: <span class="text-slate-200">/bin/bash</span> | Preserved: <span class="text-emerald-300">Session context retained (no auto-wipe)</span>
            </div>
            <div class="text-[10px] text-slate-500 pt-0.5 border-t border-slate-900 flex items-center justify-between">
              <span>Type <kbd class="text-emerald-300 font-semibold">'help'</kbd> for commands, <kbd class="text-emerald-300 font-semibold">'resume'</kbd> to unpause, or use keys bar below.</span>
              <span class="text-slate-600">Manual clear only</span>
            </div>
          </div>

          <div v-if="isLoading" class="flex items-center justify-center py-10 text-slate-500 gap-2">
            <RotateCcw class="w-4 h-4 animate-spin text-emerald-400" />
            <span>Connecting to Termux background session stream...</span>
          </div>

          <template v-else-if="processInfo?.logs && processInfo.logs.length > 0">
            <div
              v-for="(line, idx) in processInfo.logs"
              :key="idx"
              v-html="parseAnsi(line)"
              class="whitespace-pre-wrap break-all"
            ></div>
          </template>

          <div v-else class="text-slate-600 italic py-6 text-center font-mono text-xs">
            Termux session ready. Type a command or click 'Run Stage' / 'Resume' to begin.
          </div>
        </div>

        <!-- 4. Termux Signature Virtual Keys Bar -->
        <div class="px-3 py-1.5 bg-[#070b14] border-t border-slate-800 flex items-center gap-1.5 overflow-x-auto text-[11px] font-mono select-none flex-shrink-0">
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-wider mr-1">TERMUX:</span>
          <button @click="handleVirtualKey('ESC')" type="button" class="termux-btn">ESC</button>
          <button @click="handleVirtualKey('TAB')" type="button" class="termux-btn">TAB</button>
          <button @click="handleVirtualKey('CTRL_C')" type="button" class="termux-btn text-rose-300 hover:text-rose-200">^C</button>
          <button @click="navigateHistory('up')" type="button" class="termux-btn" title="Previous command in history">↑</button>
          <button @click="navigateHistory('down')" type="button" class="termux-btn" title="Next command in history">↓</button>
          <button @click="executeCommand('y')" type="button" class="termux-btn text-emerald-300 font-bold border-emerald-500/50 bg-emerald-950/50 hover:bg-emerald-900/60" title="Send YES confirmation">y</button>
          <button @click="executeCommand('n')" type="button" class="termux-btn text-rose-300 font-bold border-rose-500/50 bg-rose-950/50 hover:bg-rose-900/60" title="Send NO rejection">n</button>
          <button @click="handleResume" type="button" class="termux-btn text-amber-300 border-amber-500/50 bg-amber-950/50 hover:bg-amber-900/60 font-semibold" title="Resume interrupted session">RESUME</button>
          <button @click="executeCommand('ps aux')" type="button" class="termux-btn text-emerald-300 border-emerald-500/50 bg-emerald-950/50" title="Inspect active process table">PS</button>
          <button @click="executeCommand('status')" type="button" class="termux-btn text-sky-300 border-sky-500/50 bg-sky-950/50" title="Show task status and model">STATUS</button>
          <button @click="clearConsole" type="button" class="termux-btn text-slate-400 hover:text-slate-200" title="Clear terminal screen manually">CLEAR</button>
          <button @click="executeCommand('help')" type="button" class="termux-btn text-cyan-300 border-cyan-500/50 bg-cyan-950/50" title="Help command palette">HELP</button>
        </div>

        <!-- 5. Interactive Shell Input with Prompt -->
        <div class="p-3 bg-[#0a0f1d] border-t border-slate-800 flex-shrink-0 space-y-2">
          <!-- Command Input -->
          <form @submit.prevent="executeCommand()" class="flex items-center gap-2">
            <div class="relative flex-1">
              <input
                v-model="commandInput"
                @keydown.up.prevent="navigateHistory('up')"
                @keydown.down.prevent="navigateHistory('down')"
                type="text"
                :disabled="isExecuting"
                :placeholder="`Type command: help, resume, y, n, status, test...`"
                class="w-full h-9 pl-36 pr-3 bg-black border border-slate-800 focus:border-emerald-500 rounded-lg text-xs font-mono text-emerald-300 placeholder:text-slate-600 focus:outline-none transition-colors"
              />
              <span class="absolute left-3 top-2 text-emerald-400 font-mono text-xs select-none font-bold">
                orch@{{ taskId.toLowerCase() }}:~$
              </span>
            </div>

            <button
              type="submit"
              :disabled="!commandInput.trim() || isExecuting"
              class="h-9 px-4 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 disabled:cursor-not-allowed text-white font-medium text-xs flex items-center gap-1.5 transition-colors shadow-sm font-mono"
            >
              <Send class="w-3.5 h-3.5" />
              <span>Enter</span>
            </button>
          </form>
        </div>
      </div>

      <!-- 6. Bottom Navigation Bar -->
      <div class="h-12 px-5 bg-slate-950 border-t border-slate-800 flex items-center justify-between flex-shrink-0">
        <span class="text-xs text-slate-500 font-mono">
          Press <kbd class="px-1.5 py-0.5 rounded bg-slate-800 text-[10px] text-slate-400 border border-slate-700">ESC</kbd> to close drawer
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

<style scoped>
.termux-btn {
  height: 24px;
  padding: 0 8px;
  border-radius: 4px;
  background-color: #0d1322;
  border: 1px solid #1e293b;
  color: #94a3b8;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 11px;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.termux-btn:hover {
  background-color: #1e293b;
  color: #f1f5f9;
  border-color: #334155;
}
.termux-btn:active {
  transform: scale(0.96);
}
</style>
