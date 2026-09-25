import type { Task, ToolDTO, ProviderDTO, WorkflowDefinition, RegistryDTO, BenchmarkCellDTO } from '../types'

const BASE_URL = '/api/v1'

export const api = {
  // Tasks
  async getTasks(params?: { search?: string; method?: string; repo?: string }): Promise<Task[]> {
    const query = new URLSearchParams()
    if (params?.search) query.set('search', params.search)
    if (params?.method) query.set('method', params.method)
    if (params?.repo) query.set('repo', params.repo)
    
    const res = await fetch(`${BASE_URL}/tasks?${query.toString()}`)
    if (!res.ok) throw new Error('Failed to fetch tasks')
    return res.json()
  },

  async getTask(id: string): Promise<Task> {
    const res = await fetch(`${BASE_URL}/tasks/${id}`)
    if (!res.ok) throw new Error(`Failed to fetch task ${id}`)
    return res.json()
  },

  async createTask(payload: any): Promise<Task> {
    const res = await fetch(`${BASE_URL}/tasks`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) throw new Error('Failed to create task')
    return res.json()
  },

  // HITL
  async injectContext(taskId: string, instruction: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/inject`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ instruction })
    })
    if (!res.ok) throw new Error('Failed to inject context')
    return res.json()
  },

  async resetWorkspace(taskId: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/reset`, {
      method: 'POST'
    })
    if (!res.ok) throw new Error('Failed to reset workspace')
    return res.json()
  },

  async gateApproval(taskId: string, approved: boolean, feedback?: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/gate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ approved, feedback })
    })
    if (!res.ok) throw new Error('Failed to update gate')
    return res.json()
  },

  // Tools
  async getTools(category?: string, search?: string): Promise<ToolDTO[]> {
    const query = new URLSearchParams()
    if (category) query.set('category', category)
    if (search) query.set('search', search)
    const res = await fetch(`${BASE_URL}/tools?${query.toString()}`)
    if (!res.ok) throw new Error('Failed to fetch tools')
    return res.json()
  },

  async installTool(toolId: string, version: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tools/install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tool_id: toolId, version })
    })
    return res.json()
  },

  async rollbackTool(toolId: string, targetVersion: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tools/rollback`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tool_id: toolId, target_version: targetVersion })
    })
    return res.json()
  },

  async resolveBestFitMatrix(): Promise<any> {
    const res = await fetch(`${BASE_URL}/tools/resolve-matrix`, { method: 'POST' })
    return res.json()
  },

  // Providers & Benchmarks
  async getProviders(): Promise<{ providers: ProviderDTO[]; tier_mapping: any; is_override: boolean }> {
    const res = await fetch(`${BASE_URL}/providers`)
    return res.json()
  },

  async testProvider(providerId: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/providers/test`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider_id: providerId })
    })
    return res.json()
  },

  async getBenchmarks(): Promise<{ total_cells: number; matrix: BenchmarkCellDTO[] }> {
    const res = await fetch(`${BASE_URL}/benchmarks`)
    return res.json()
  },

  // Workflows & Registries
  async getWorkflows(): Promise<WorkflowDefinition[]> {
    const res = await fetch(`${BASE_URL}/workflows`)
    return res.json()
  },

  async createWorkflow(workflow: WorkflowDefinition): Promise<any> {
    const res = await fetch(`${BASE_URL}/workflows`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(workflow)
    })
    return res.json()
  },

  async getRegistries(): Promise<RegistryDTO> {
    const res = await fetch(`${BASE_URL}/registries`)
    return res.json()
  },

  // Artifacts
  async getArtifact(taskId: string, filename: string): Promise<{ task_id: string; filename: string; content: string }> {
    const res = await fetch(`${BASE_URL}/artifacts/${taskId}/${filename}`)
    return res.json()
  }
}
