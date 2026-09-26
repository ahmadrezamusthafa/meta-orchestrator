import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Project, ScanDirResult } from '../types'
import { api } from '../services/api'
import { useToastStore } from './toast'

export const useProjectStore = defineStore('projects', () => {
  const toast = useToastStore()
  const projects = ref<Project[]>([])
  const activeProjectId = ref<string>('')
  const isLoading = ref<boolean>(false)

  const activeProject = computed<Project | null>(() => {
    if (!projects.value.length) return null
    return projects.value.find((p) => p.id === activeProjectId.value) || projects.value[0] || null
  })

  const activeRepos = computed(() => {
    return activeProject.value?.repos || []
  })

  async function fetchProjects() {
    isLoading.value = true
    try {
      projects.value = await api.getProjects()
      if (!activeProjectId.value && projects.value.length > 0) {
        activeProjectId.value = projects.value[0].id
      }
    } catch (err: any) {
      toast.error('Failed to load projects', err.message)
    } finally {
      isLoading.value = false
    }
  }

  async function createProject(payload: Partial<Project>) {
    isLoading.value = true
    try {
      const created = await api.createProject(payload)
      await fetchProjects()
      activeProjectId.value = created.id
      toast.success('Project Provisioned', `Created project "${created.name}" and symlinked ${created.repos?.length || 0} repositories`)
      return created
    } catch (err: any) {
      toast.error('Failed to create project', err.message)
      throw err
    } finally {
      isLoading.value = false
    }
  }

  async function resyncProject(id: string) {
    isLoading.value = true
    try {
      const resynced = await api.resyncProject(id)
      await fetchProjects()
      toast.success('Symlinks Synchronized', `Re-established symlinks for ${resynced.repos?.length || 0} repositories`)
      return resynced
    } catch (err: any) {
      toast.error('Resync failed', err.message)
      throw err
    } finally {
      isLoading.value = false
    }
  }

  async function deleteProject(id: string) {
    isLoading.value = true
    try {
      await api.deleteProject(id)
      toast.info('Project Deleted', `Removed project ${id}`)
      await fetchProjects()
      if (activeProjectId.value === id) {
        activeProjectId.value = projects.value[0]?.id || ''
      }
    } catch (err: any) {
      toast.error('Delete failed', err.message)
      throw err
    } finally {
      isLoading.value = false
    }
  }

  async function scanDirectory(path?: string): Promise<ScanDirResult> {
    try {
      return await api.scanDirectory(path)
    } catch (err: any) {
      toast.error('Directory scan failed', err.message)
      throw err
    }
  }

  function selectProject(id: string) {
    activeProjectId.value = id
  }

  return {
    projects,
    activeProjectId,
    activeProject,
    activeRepos,
    isLoading,
    fetchProjects,
    createProject,
    resyncProject,
    deleteProject,
    scanDirectory,
    selectProject,
  }
})
