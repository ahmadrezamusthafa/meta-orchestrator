<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { useTerminalBuffer } from './useTerminalBuffer'
import { Terminal as TerminalIcon, Download, Trash2, ArrowDownCircle } from 'lucide-vue-next'

const props = defineProps<{
  logs?: string[]
}>()

const container = ref<HTMLDivElement | null>(null)
let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null

const { isAutoScroll, isBuffering, enqueue, clear } = useTerminalBuffer(() => terminal)

onMounted(() => {
  if (!container.value) return

  terminal = new Terminal({
    fontFamily: '"JetBrains Mono", monospace',
    fontSize: 12,
    lineHeight: 1.4,
    theme: {
      background: '#020617',
      foreground: '#f8fafc',
      cursor: '#10b981',
      selectionBackground: '#334155',
    },
    convertEol: true,
    cursorBlink: true,
    disableStdin: true,
  })

  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.open(container.value)
  fitAddon.fit()

  terminal.onScroll(() => {
    if (terminal) {
      const isAtBottom = terminal.buffer.active.viewportY >= terminal.buffer.active.baseY
      isAutoScroll.value = isAtBottom
    }
  })

  enqueue("\x1b[36m[Orchestrator]\x1b[0m Container sandbox initialized on isolated docker network.\r\n")
  enqueue("\x1b[32m[Git Coordinator]\x1b[0m Synchronized feature branches across target repositories.\r\n")
  enqueue("\x1b[33m[Write-Lock Guard]\x1b[0m Read-only locks verified on application source tree.\r\n")

  window.addEventListener('resize', handleResize)
})

function handleResize() {
  fitAddon?.fit()
}

watch(
  () => props.logs,
  (newLogs) => {
    if (newLogs && newLogs.length > 0) {
      enqueue(newLogs[newLogs.length - 1])
    }
  },
  { deep: true }
)

function downloadLog() {
  if (!terminal) return
  let logText = ''
  const buffer = terminal.buffer.active
  for (let i = 0; i < buffer.length; i++) {
    const line = buffer.getLine(i)
    if (line) logText += line.translateToString(true) + '\n'
  }
  const blob = new Blob([logText], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `terminal-execution-${Date.now()}.log`
  a.click()
  URL.revokeObjectURL(url)
}

function scrollToBottom() {
  terminal?.scrollToBottom()
  isAutoScroll.value = true
}

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  terminal?.dispose()
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 border-t border-slate-800">
    <div class="h-9 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <TerminalIcon class="w-3.5 h-3.5 text-emerald-400" />
        <span class="font-mono text-slate-300 font-medium">STDOUT / STDERR STREAM</span>
        <span
          class="px-1.5 py-0.2 rounded text-[10px] font-mono font-medium"
          :class="isBuffering ? 'bg-amber-950 text-amber-300' : 'bg-emerald-950 text-emerald-300'"
        >
          {{ isBuffering ? 'Buffering' : 'Live' }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <button
          v-if="!isAutoScroll"
          @click="scrollToBottom"
          class="flex items-center gap-1 px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 border border-emerald-800 text-[10px] font-mono font-medium animate-pulse"
        >
          <ArrowDownCircle class="w-3 h-3" />
          <span>Resume Scroll</span>
        </button>

        <button
          @click="clear"
          title="Clear console"
          class="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
        >
          <Trash2 class="w-3.5 h-3.5" />
        </button>

        <button
          @click="downloadLog"
          title="Download execution logs"
          class="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
        >
          <Download class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <div ref="container" class="flex-1 p-2 overflow-hidden bg-slate-950"></div>
  </div>
</template>
