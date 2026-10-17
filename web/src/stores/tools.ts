import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ToolDTO } from '../types'
import { api } from '../services/api'

export const useToolsStore = defineStore('tools', () => {
  const tools = ref<ToolDTO[]>([])
  const isLoading = ref(false)

  async function fetchTools(category?: string, search?: string) {
    isLoading.value = true
    try {
      tools.value = await api.getTools(category, search)
    } catch (e) {
      console.error('Failed to load tools:', e)
    } finally {
      isLoading.value = false
    }
  }

  /** Runs every health check again instead of using the cached answers. */
  async function recheckTools() {
    isLoading.value = true
    try {
      tools.value = await api.recheckTools()
    } finally {
      isLoading.value = false
    }
  }

  return {
    tools,
    isLoading,
    fetchTools,
    recheckTools,
    installTool: api.installTool,
    resolveBestFitMatrix: api.resolveBestFitMatrix,
  }
})
