import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ToolDTO } from '../types'
import { api } from '../services/api'
import { wsService } from '../services/websocket'

export const useToolsStore = defineStore('tools', () => {
  const tools = ref<ToolDTO[]>([])
  const isLoading = ref(false)
  const installProgress = ref<any>(null)

  async function fetchTools(category?: string, search?: string) {
    isLoading.value = true
    try {
      tools.value = await api.getTools(category, search)
    } finally {
      isLoading.value = false
    }
  }

  async function installTool(toolId: string, version: string) {
    return api.installTool(toolId, version)
  }

  async function rollbackTool(toolId: string, targetVersion: string) {
    const res = await api.rollbackTool(toolId, targetVersion)
    await fetchTools()
    return res
  }

  async function resolveBestFitMatrix() {
    return api.resolveBestFitMatrix()
  }

  function initProgressSubscription() {
    wsService.subscribe((event) => {
      if (event.type === 'tool.progress' && event.payload) {
        installProgress.value = event.payload
      }
    })
  }

  return {
    tools,
    isLoading,
    installProgress,
    fetchTools,
    installTool,
    rollbackTool,
    resolveBestFitMatrix,
    initProgressSubscription,
  }
})
