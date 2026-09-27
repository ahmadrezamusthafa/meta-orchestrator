import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { UniversalSkillDTO, SkillSourceDTO, CheckPathCompatibilityResponse } from '../types'
import { api } from '../services/api'

export const useSkillsStore = defineStore('skills', () => {
  const skills = ref<UniversalSkillDTO[]>([])
  const sources = ref<SkillSourceDTO[]>([])
  const isLoading = ref(false)
  const isRescanning = ref(false)
  const error = ref<string | null>(null)

  const enabledCount = computed(() => skills.value.filter((s) => s.enabled).length)
  const disabledCount = computed(() => skills.value.filter((s) => !s.enabled).length)
  const claudeCount = computed(() => skills.value.filter((s) => s.source_format === 'claude').length)
  const mcpCount = computed(() => skills.value.filter((s) => s.source_format === 'mcp').length)
  const compatibleCount = computed(() => skills.value.filter((s) => s.compatibility?.status === 'compatible').length)
  const warningCount = computed(() => skills.value.filter((s) => s.compatibility?.status === 'warning').length)
  const incompatibleCount = computed(() => skills.value.filter((s) => s.compatibility?.status === 'incompatible').length)

  async function fetchSkills(params?: { enabled_only?: boolean; format?: string; source_type?: string; search?: string }) {
    isLoading.value = true
    error.value = null
    try {
      skills.value = await api.getSkills(params)
    } catch (err: any) {
      error.value = err.message || 'Failed to fetch skills'
    } finally {
      isLoading.value = false
    }
  }

  async function fetchSources() {
    try {
      sources.value = await api.getSkillSources()
    } catch (err: any) {
      console.error('Failed to load skill sources:', err)
    }
  }

  async function toggleSkill(name: string, enabled: boolean) {
    // Optimistic update
    const target = skills.value.find((s) => s.name === name)
    const prev = target ? target.enabled : !enabled
    if (target) {
      target.enabled = enabled
    }

    try {
      await api.toggleSkill(name, enabled)
    } catch (err: any) {
      // Revert if failed
      if (target) {
        target.enabled = prev
      }
      throw err
    }
  }

  async function registerSource(name: string, path: string, format?: string) {
    const res = await api.registerSkillSource({ name, path, format })
    await fetchSources()
    await fetchSkills()
    return res
  }

  async function removeSource(sourceId: string) {
    await api.removeSkillSource(sourceId)
    await fetchSources()
    await fetchSkills()
  }

  async function checkPath(path: string): Promise<CheckPathCompatibilityResponse> {
    return api.checkPathCompatibility(path)
  }

  async function rescan() {
    isRescanning.value = true
    try {
      await api.rescanSkills()
      await fetchSources()
      await fetchSkills()
    } finally {
      setTimeout(() => {
        isRescanning.value = false
      }, 400)
    }
  }

  return {
    skills,
    sources,
    isLoading,
    isRescanning,
    error,
    enabledCount,
    disabledCount,
    claudeCount,
    mcpCount,
    compatibleCount,
    warningCount,
    incompatibleCount,
    fetchSkills,
    fetchSources,
    toggleSkill,
    registerSource,
    removeSource,
    checkPath,
    rescan,
  }
})
