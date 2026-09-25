import { defineStore } from 'pinia'
import { ref } from 'vue'
import { wsService } from '../services/websocket'

export const useTerminalStore = defineStore('terminal', () => {
  const logs = ref<string[]>([])
  const thoughts = ref<any[]>([])
  const isAutoScroll = ref(true)
  const isBuffering = ref(false)
  const isTerminated = ref(false)

  function appendLog(line: string) {
    logs.value.push(line)
    if (logs.value.length > 5000) {
      logs.value.shift()
    }
  }

  function appendThought(thought: any) {
    thoughts.value.push(thought)
  }

  function clearLogs() {
    logs.value = []
  }

  function initTaskListeners(taskId: string) {
    return wsService.subscribe((event) => {
      if (event.task_id && event.task_id !== taskId) return

      if (event.type === 'agent.terminal' && event.payload?.chunk) {
        appendLog(event.payload.chunk)
      } else if (event.type === 'agent.thought' && event.payload) {
        appendThought({
          timestamp: event.timestamp || new Date().toISOString(),
          ...event.payload,
        })
      }
    })
  }

  return {
    logs,
    thoughts,
    isAutoScroll,
    isBuffering,
    isTerminated,
    appendLog,
    appendThought,
    clearLogs,
    initTaskListeners,
  }
})
