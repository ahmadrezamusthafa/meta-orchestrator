import { ref, onUnmounted } from 'vue'
import type { Terminal } from '@xterm/xterm'

export function useTerminalBuffer(terminalRef: () => Terminal | null) {
  const buffer: string[] = []
  let rafId: number | null = null
  const isAutoScroll = ref(true)
  const isBuffering = ref(false)

  function flushBuffer() {
    const term = terminalRef()
    if (term && buffer.length > 0) {
      const chunk = buffer.splice(0, buffer.length).join('')
      term.write(chunk)
      if (isAutoScroll.value) {
        term.scrollToBottom()
      }
    }
    isBuffering.value = buffer.length > 0
    rafId = null
  }

  function enqueue(text: string) {
    buffer.push(text)
    isBuffering.value = true
    if (!rafId) {
      rafId = window.requestAnimationFrame(flushBuffer)
    }
  }

  function clear() {
    buffer.length = 0
    const term = terminalRef()
    if (term) {
      term.clear()
    }
  }

  onUnmounted(() => {
    if (rafId) {
      window.cancelAnimationFrame(rafId)
    }
  })

  return {
    isAutoScroll,
    isBuffering,
    enqueue,
    clear,
  }
}
