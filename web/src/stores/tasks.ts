import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Task, TaskState } from '../types'
import { api } from '../services/api'
import { wsService } from '../services/websocket'

export const useTaskStore = defineStore('tasks', () => {
  const tasks = ref<Task[]>([])
  const activeTaskId = ref<string | null>(null)
  const isLoading = ref(false)
  const searchQuery = ref('')
  const selectedMethod = ref('All')
  const selectedRepo = ref('All')

  // Available methods & repos for toolbar
  const availableMethods = ['All', 'BMAD', 'Supervisor', 'ReAct', 'Superpower']
  const availableRepos = ['All', 'frontend-portal', 'backend-core', 'api-contracts']

  // Filtered tasks
  const filteredTasks = computed(() => {
    return tasks.value.filter((t) => {
      if (searchQuery.value) {
        const q = searchQuery.value.toLowerCase()
        const matchesTitle = t.title.toLowerCase().includes(q)
        const matchesId = t.id.toLowerCase().includes(q)
        if (!matchesTitle && !matchesId) return false
      }
      if (selectedMethod.value !== 'All' && t.selected_method !== selectedMethod.value) {
        return false
      }
      if (selectedRepo.value !== 'All' && !t.assigned_repos.includes(selectedRepo.value)) {
        return false
      }
      return true
    })
  })

  // Grouped tasks by stage
  const tasksByStage = computed(() => {
    const map: Record<string, Task[]> = {
      prd_discovery: [],
      atdd_creation: [],
      techdoc_rfc: [],
      task_breakdown: [],
      task_implementation: [],
      e2e_validation: [],
      uat_verification: [],
      signoff_merge: [],
    }

    filteredTasks.value.forEach((task) => {
      const stage = task.current_stage_id || 'prd_discovery'
      if (!map[stage]) {
        map[stage] = []
      }
      map[stage].push(task)
    })
    return map
  })

  // Frustrated tasks count for global header badge
  const frustratedTaskCount = computed(() => {
    return tasks.value.filter((t) => t.state === 'BLOCKED_FRUSTRATION').length
  })

  // Total token burn across all tasks
  const totalTokensBurned = computed(() => {
    return tasks.value.reduce((acc, t) => acc + (t.token_usage?.total_tokens || 0), 0)
  })

  // Active task object
  const activeTask = computed(() => {
    if (!activeTaskId.value) return null
    return tasks.value.find((t) => t.id === activeTaskId.value) || null
  })

  async function fetchTasks() {
    isLoading.value = true
    try {
      tasks.value = await api.getTasks()
    } catch (e) {
      console.error('Failed to load tasks:', e)
    } finally {
      isLoading.value = false
    }
  }

  async function fetchTask(id: string) {
    try {
      const task = await api.getTask(id)
      const idx = tasks.value.findIndex((t) => t.id === id)
      if (idx !== -1) {
        tasks.value[idx] = task
      } else {
        tasks.value.push(task)
      }
      activeTaskId.value = id
      return task
    } catch (e) {
      console.error(`Failed to fetch task ${id}:`, e)
      return null
    }
  }

  async function createTask(payload: any) {
    const newTask = await api.createTask(payload)
    tasks.value.unshift(newTask)
    return newTask
  }

  async function injectContext(taskId: string, instruction: string) {
    const res = await api.injectContext(taskId, instruction)
    if (res.task) {
      const idx = tasks.value.findIndex((t) => t.id === taskId)
      if (idx !== -1) tasks.value[idx] = res.task
    }
  }

  async function resetWorkspace(taskId: string) {
    const res = await api.resetWorkspace(taskId)
    if (res.task) {
      const idx = tasks.value.findIndex((t) => t.id === taskId)
      if (idx !== -1) tasks.value[idx] = res.task
    }
  }

  async function updateGate(taskId: string, approved: boolean, feedback?: string) {
    const res = await api.gateApproval(taskId, approved, feedback)
    if (res.task) {
      const idx = tasks.value.findIndex((t) => t.id === taskId)
      if (idx !== -1) tasks.value[idx] = res.task
    }
  }

  function moveTaskToStage(taskId: string, targetStageId: string) {
    const task = tasks.value.find((t) => t.id === taskId)
    if (!task) return
    task.current_stage_id = targetStageId

    // Automatically synchronize gate status based on target stage
    const gateStages = ['techdoc_rfc', 'signoff_merge', 'hotfix_validation']
    if (gateStages.includes(targetStageId)) {
      task.state = 'WAITING_GATE_APPROVAL'
    } else if (task.state === 'WAITING_GATE_APPROVAL') {
      task.state = 'RUNNING'
    }
  }

  // Subscribe to live WebSocket events
  function initWebSocketSync() {
    wsService.subscribe((event) => {
      if (event.type === 'task.status' && event.payload) {
        const updated = event.payload as Task
        const idx = tasks.value.findIndex((t) => t.id === updated.id)
        if (idx !== -1) {
          tasks.value[idx] = updated
        } else {
          tasks.value.unshift(updated)
        }
      } else if (event.type === 'frustration.halt' && event.task_id) {
        const task = tasks.value.find((t) => t.id === event.task_id)
        if (task) {
          task.state = 'BLOCKED_FRUSTRATION'
          if (!task.metadata) task.metadata = {}
          task.metadata.failing_trace = event.payload?.failing_trace || 'Frustration threshold exceeded'
        }
      }
    })
  }

  return {
    tasks,
    activeTaskId,
    activeTask,
    isLoading,
    searchQuery,
    selectedMethod,
    selectedRepo,
    availableMethods,
    availableRepos,
    filteredTasks,
    tasksByStage,
    frustratedTaskCount,
    totalTokensBurned,
    fetchTasks,
    fetchTask,
    createTask,
    injectContext,
    resetWorkspace,
    updateGate,
    moveTaskToStage,
    initWebSocketSync,
  }
})
