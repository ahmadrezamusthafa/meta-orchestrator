import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import type { Task, JiraSyncSettings, JiraSyncConfig } from '../types'
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

  const NO_EPIC = '__none'
  const GROUP_KEY = 'mo.kanban.groupBy'
  const readGroup = (): 'none' | 'epic' => {
    try { return localStorage.getItem(GROUP_KEY) === 'epic' ? 'epic' : 'none' } catch { return 'none' }
  }
  const groupBy = ref<'none' | 'epic'>(readGroup())
  watch(groupBy, (v) => { try { localStorage.setItem(GROUP_KEY, v) } catch { /* storage unavailable */ } })
  const selectedEpic = ref<string>('All') // 'All' | NO_EPIC | epic key

  const jiraSync = ref<JiraSyncSettings | null>(null)
  const isSyncingJira = ref(false)

  // Available methods & repos for toolbar
  const availableMethods = ['All', 'BMAD', 'Supervisor', 'ReAct', 'Superpower']
  const availableRepos = computed(() => {
    const repos = new Set<string>()
    tasks.value.forEach((t) => (t.assigned_repos || []).forEach((r) => repos.add(r)))
    return ['All', ...Array.from(repos).sort()]
  })

  // JIRA keys already on the board, for de-duplicating imports.
  const jiraKeysOnBoard = computed(() => {
    const keys = new Map<string, string>()
    tasks.value.forEach((t) => {
      const k = t.metadata?.jira_key
      if (k) keys.set(k.toUpperCase(), t.id)
    })
    return keys
  })

  // Filtered tasks
  const filteredTasks = computed(() => {
    return tasks.value.filter((t) => {
      if (searchQuery.value) {
        const q = searchQuery.value.toLowerCase()
        const matchesTitle = t.title.toLowerCase().includes(q)
        const matchesId = t.id.toLowerCase().includes(q)
        const matchesJira = (t.metadata?.jira_key || '').toLowerCase().includes(q)
        if (!matchesTitle && !matchesId && !matchesJira) return false
      }
      if (selectedMethod.value !== 'All' && t.selected_method !== selectedMethod.value) {
        return false
      }
      if (selectedRepo.value !== 'All' && !(t.assigned_repos || []).includes(selectedRepo.value)) {
        return false
      }
      if (selectedStatus.value !== 'all') {
        if (selectedStatus.value === 'running' && t.state !== 'RUNNING') return false
        if (selectedStatus.value === 'gate' && t.state !== 'WAITING_GATE_APPROVAL') return false
        if (selectedStatus.value === 'blocked' && t.state !== 'BLOCKED_FRUSTRATION') return false
        if (selectedStatus.value === 'waiting_dep' && t.state !== 'WAITING_DEPENDENCY') return false
        if (selectedStatus.value === 'completed' && t.state !== 'COMPLETED') return false
      }
      if (onlyMyTasks.value && !t.metadata?.jira_key) return false
      if (selectedEpic.value !== 'All') {
        const epic = t.metadata?.jira_epic_key || NO_EPIC
        if (epic !== selectedEpic.value) return false
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

  // Epics present on the board, for the filter and swimlane headers.
  const epics = computed(() => {
    const map = new Map<string, { key: string; name: string; url: string; count: number }>()
    tasks.value.forEach((t) => {
      const key = t.metadata?.jira_epic_key
      if (!key) return
      const e = map.get(key) || { key, name: '', url: '', count: 0 }
      e.count++
      if (!e.name && t.metadata?.jira_epic_name) e.name = t.metadata.jira_epic_name
      if (!e.url && t.metadata?.jira_url) e.url = t.metadata.jira_url.replace(/\/browse\/[^/]+$/, `/browse/${key}`)
      map.set(key, e)
    })
    return Array.from(map.values()).sort((a, b) => (a.name || a.key).localeCompare(b.name || b.key))
  })

  // Swimlanes: one row per epic (plus "No epic"), each split by stage.
  const swimlanes = computed(() => {
    const lanes = new Map<string, { key: string; name: string; url: string; total: number; completed: number; byStage: Record<string, Task[]> }>()
    const meta = new Map(epics.value.map((e) => [e.key, e]))
    filteredTasks.value.forEach((task) => {
      const key = task.metadata?.jira_epic_key || NO_EPIC
      let lane = lanes.get(key)
      if (!lane) {
        const e = meta.get(key)
        lane = { key, name: key === NO_EPIC ? 'No epic' : e?.name || key, url: e?.url || '', total: 0, completed: 0, byStage: {} }
        lanes.set(key, lane)
      }
      const stage = task.current_stage_id || 'prd_discovery'
      ;(lane.byStage[stage] ||= []).push(task)
      lane.total++
      if (task.state === 'COMPLETED') lane.completed++
    })
    return Array.from(lanes.values()).sort((a, b) => {
      if (a.key === NO_EPIC) return 1
      if (b.key === NO_EPIC) return -1
      return a.name.localeCompare(b.name)
    })
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

  function replaceTask(updated: Task) {
    const idx = tasks.value.findIndex((t) => t.id === updated.id)
    if (idx !== -1) tasks.value[idx] = updated
    else tasks.value.unshift(updated)
  }

  // Moves a card optimistically; the daemon decides the resulting state (dependencies, gates) and
  // the card snaps back if it refuses the move.
  async function moveTaskToStage(taskId: string, targetStageId: string) {
    const task = tasks.value.find((t) => t.id === taskId)
    if (!task || task.current_stage_id === targetStageId) return
    if (task.state === 'RUNNING') {
      toastStore.warning('Task is running', `Pause ${task.id} before moving it to another stage.`)
      return
    }
    const previous = { ...task }
    task.current_stage_id = targetStageId
    try {
      replaceTask(await api.patchTask(taskId, { current_stage_id: targetStageId }))
    } catch (err: any) {
      replaceTask(previous)
      toastStore.error('Move rejected', err?.message || `Could not move ${taskId}`)
    }
  }

  async function deleteTask(taskId: string) {
    const idx = tasks.value.findIndex((t) => t.id === taskId)
    const removed = idx !== -1 ? tasks.value[idx] : null
    if (idx !== -1) tasks.value.splice(idx, 1)
    try {
      await api.deleteTask(taskId)
      if (activeTaskId.value === taskId) activeTaskId.value = null
      const key = removed?.metadata?.jira_key
      toastStore.success('Task removed', key
        ? `${taskId} removed. JIRA sync will not re-add ${key}; import it again to bring it back.`
        : `${taskId} removed from the board.`)
    } catch (err: any) {
      if (removed) tasks.value.splice(idx, 0, removed)
      toastStore.error('Delete failed', err?.message || `Could not delete ${taskId}`)
    }
  }

  async function fetchJiraSync() {
    try {
      jiraSync.value = await api.getJiraSync()
    } catch (e) {
      console.debug('Failed to load JIRA sync status', e)
    }
  }

  async function runJiraSync(opts: { silent?: boolean } = {}) {
    if (isSyncingJira.value) return
    isSyncingJira.value = true
    try {
      const res = await api.runJiraSync()
      jiraSync.value = res
      const st = res.status
      if (st.last_error) {
        if (!opts.silent) toastStore.error('JIRA sync failed', st.last_error)
      } else if (!opts.silent || st.created > 0) {
        toastStore.success('JIRA synced', st.created || st.updated
          ? `${st.created} new, ${st.updated} updated from ${st.fetched} matching issues.`
          : `Board is up to date (${st.fetched} matching issues).`)
      }
      await fetchTasks()
    } catch (err: any) {
      if (!opts.silent) toastStore.error('JIRA sync failed', err?.message || 'Unknown error')
    } finally {
      isSyncingJira.value = false
    }
  }

  async function saveJiraSync(cfg: JiraSyncConfig) {
    jiraSync.value = await api.updateJiraSync(cfg)
    return jiraSync.value
  }

  async function restoreDismissedJira() {
    jiraSync.value = await api.clearJiraDismissed()
  }

  // Subscribe to live WebSocket events
  let wsSubscribed = false
  function initWebSocketSync() {
    if (wsSubscribed) return // the board view re-mounts; one listener is enough
    wsSubscribed = true
    wsService.subscribe((event) => {
      if (event.type === 'task.status' && event.payload) {
        const incoming = event.payload as Task
        const before = Number(tasks.value.find((t) => t.id === incoming.id)?.metadata?.pending_approvals || 0)
        const after = Number(incoming.metadata?.pending_approvals || 0)
        if (after > before) {
          toastStore.warning('Approval needed', `${incoming.id} is paused until you allow or deny an action. Open its console to decide.`)
          try {
            if (document.hidden && 'Notification' in window && Notification.permission === 'granted') {
              new Notification(`${incoming.id} needs approval`, { body: incoming.title })
            }
          } catch { /* notifications unavailable */ }
        }
        replaceTask(incoming)
      } else if (event.type === 'task.deleted' && event.task_id) {
        tasks.value = tasks.value.filter((t) => t.id !== event.task_id)
      } else if (event.type === 'jira.sync' && event.payload) {
        if (jiraSync.value) jiraSync.value = { ...jiraSync.value, status: event.payload }
        else fetchJiraSync()
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

  const pendingActions = ref<Record<string, boolean>>({})

  // Runs a lifecycle action and applies the task the daemon returns — the UI never guesses the
  // resulting state. The console shows the details; toasts only report failures.
  async function performTaskAction(taskId: string, kind: 'run' | 'resume' | 'pause' | 'reset') {
    if (pendingActions.value[taskId]) return
    pendingActions.value = { ...pendingActions.value, [taskId]: true }
    const call = { run: api.executeTask, resume: api.resumeTask, pause: api.pauseTask, reset: api.resetWorkspace }[kind]
    const label = { run: 'Run', resume: 'Resume', pause: 'Pause', reset: 'Reset' }[kind]
    try {
      const res = await call(taskId)
      if (res?.task) replaceTask(res.task)
      else await fetchTask(taskId)
      return res
    } catch (err: any) {
      toastStore.error(`${label} failed`, err?.message || `Could not ${label.toLowerCase()} ${taskId}`)
      await fetchTask(taskId)
      throw err
    } finally {
      const next = { ...pendingActions.value }
      delete next[taskId]
      pendingActions.value = next
    }
  }

  const executeTask = (taskId: string) => performTaskAction(taskId, 'run')
  const resumeTask = (taskId: string) => performTaskAction(taskId, 'resume')
  const pauseTask = (taskId: string) => performTaskAction(taskId, 'pause')

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
    jiraSync,
    isSyncingJira,
    groupBy,
    selectedEpic,
    epics,
    swimlanes,
    NO_EPIC,
    jiraKeysOnBoard,
    fetchTasks,
    fetchTask,
    createTask,
    executeTask,
    resumeTask,
    pauseTask,
    performTaskAction,
    pendingActions,
    injectContext,
    resetWorkspace,
    updateGate,
    moveTaskToStage,
    deleteTask,
    fetchJiraSync,
    runJiraSync,
    saveJiraSync,
    restoreDismissedJira,
    initWebSocketSync,
  }
})
