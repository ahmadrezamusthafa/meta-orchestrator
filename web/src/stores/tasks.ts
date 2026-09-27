import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Task, TaskState } from '../types'
import { api } from '../services/api'
import { wsService } from '../services/websocket'
import { useToastStore } from './toast'

export const useTaskStore = defineStore('tasks', () => {
  const toastStore = useToastStore()
  const tasks = ref<Task[]>([])
  const activeTaskId = ref<string | null>(null)
  const isLoading = ref(false)
  const searchQuery = ref('')
  const selectedMethod = ref('All')
  const selectedRepo = ref('All')
  const selectedStatus = ref<'all' | 'running' | 'gate' | 'blocked' | 'completed' | 'waiting_dep'>('all')
  const onlyMyTasks = ref(false)

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
      if (selectedStatus.value !== 'all') {
        if (selectedStatus.value === 'running' && t.state !== 'RUNNING') return false
        if (selectedStatus.value === 'gate' && t.state !== 'WAITING_GATE_APPROVAL') return false
        if (selectedStatus.value === 'blocked' && t.state !== 'BLOCKED_FRUSTRATION') return false
        if (selectedStatus.value === 'waiting_dep' && t.state !== 'WAITING_DEPENDENCY') return false
        if (selectedStatus.value === 'completed' && t.state !== 'COMPLETED') return false
      }
      if (onlyMyTasks.value) {
        const hasJira = !!t.metadata?.jira_key || /\b([A-Z]{2,10}-\d+)\b/.test(t.title)
        if (!hasJira && t.state === 'COMPLETED') return false
      }
      return true
    })
  })

  // Status breakdown counts
  const statusCounts = computed(() => {
    let all = tasks.value.length
    let running = 0
    let gate = 0
    let blocked = 0
    let waiting_dep = 0
    let completed = 0

    tasks.value.forEach((t) => {
      if (t.state === 'RUNNING') running++
      else if (t.state === 'WAITING_GATE_APPROVAL') gate++
      else if (t.state === 'BLOCKED_FRUSTRATION') blocked++
      else if (t.state === 'WAITING_DEPENDENCY') waiting_dep++
      else if (t.state === 'COMPLETED') completed++
    })

    return { all, running, gate, blocked, waiting_dep, completed }
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
    } else if (task.state === 'WAITING_GATE_APPROVAL' || task.state === 'WAITING_DEPENDENCY') {
      task.state = 'RUNNING'
    }

    // Check task dependencies if advancing to implementation or execution
    if (['task_implementation', 'e2e_validation', 'uat_verification', 'signoff_merge'].includes(targetStageId)) {
      const unmet = (task.dependencies || []).filter(depId => {
        const dep = tasks.value.find(t => t.id === depId)
        return !dep || dep.state !== 'COMPLETED'
      })
      if (unmet.length > 0) {
        task.state = 'WAITING_DEPENDENCY'
        if (!task.metadata) task.metadata = {}
        task.metadata.unmet_dependencies = unmet.join(',')
        toastStore.warning(
          'Dependency Prerequisite Required',
          `Task ${task.id} is held in WAITING_DEPENDENCY until prerequisite ${unmet.join(', ')} is COMPLETED.`
        )
      }
    }

    // If task is completed, check if any dependent tasks can now be unblocked
    if (task.state === 'COMPLETED' || targetStageId === 'signoff_merge') {
      tasks.value.forEach(other => {
        if (other.dependencies?.includes(task.id) && other.state === 'WAITING_DEPENDENCY') {
          const remainingUnmet = other.dependencies.filter(dId => {
            const d = tasks.value.find(t => t.id === dId)
            return !d || d.state !== 'COMPLETED'
          })
          if (remainingUnmet.length === 0) {
            other.state = 'RUNNING'
            if (other.metadata) delete other.metadata.unmet_dependencies
            toastStore.success(
              'Dependency Resolved',
              `Prerequisites for ${other.id} are now complete! Worktree unblocked and ready for execution.`
            )
          }
        }
      })
    }

    // Persist to backend
    api.patchTask(taskId, { current_stage_id: targetStageId, state: task.state }).catch((err) => {
      console.debug('Failed to sync stage change with backend', err)
    })
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
    selectedStatus,
    onlyMyTasks,
    statusCounts,
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
