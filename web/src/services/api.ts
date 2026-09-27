import type { 
  Task, ToolDTO, ProviderDTO, WorkflowDefinition, RegistryDTO, BenchmarkCellDTO, 
  Project, ScanDirResult, BrowseFSResponse, CreateFolderResponse,
  ConnectorsConfig, JiraConfig, ConfluenceConfig, JiraIssueDTO, ImportJiraIssueRequest,
  ConfluencePublishRequest, ConfluencePublishResponse, TestConnectorRequest, TestConnectorResponse,
  ConnectorItem, ToggleConnectorRequest, MCPConfig, ConnectorPingConfig, PingAllSummary,
  UniversalSkillDTO, SkillSourceDTO, CheckPathCompatibilityResponse,
  PromptItemDTO, PromptSourceDTO
} from '../types'


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
  },

  // Projects & Multi-Repo Management
  async getProjects(): Promise<Project[]> {
    const res = await fetch(`${BASE_URL}/projects`)
    if (!res.ok) throw new Error('Failed to fetch projects')
    return res.json()
  },

  async getProject(id: string): Promise<Project> {
    const res = await fetch(`${BASE_URL}/projects/${id}`)
    if (!res.ok) throw new Error(`Failed to fetch project ${id}`)
    return res.json()
  },

  async createProject(project: Partial<Project>): Promise<Project> {
    const res = await fetch(`${BASE_URL}/projects`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(project)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to create project' }))
      throw new Error(err.error || 'Failed to create project')
    }
    return res.json()
  },

  async updateProject(id: string, project: Partial<Project>): Promise<Project> {
    const res = await fetch(`${BASE_URL}/projects/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(project)
    })
    if (!res.ok) throw new Error(`Failed to update project ${id}`)
    return res.json()
  },

  async deleteProject(id: string): Promise<void> {
    const res = await fetch(`${BASE_URL}/projects/${id}`, {
      method: 'DELETE'
    })
    if (!res.ok) throw new Error(`Failed to delete project ${id}`)
  },

  async resyncProject(id: string): Promise<Project> {
    const res = await fetch(`${BASE_URL}/projects/${id}/resync`, {
      method: 'POST'
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to resync symlinks' }))
      throw new Error(err.error || 'Failed to resync symlinks')
    }
    return res.json()
  },

  async scanDirectory(path?: string): Promise<ScanDirResult> {
    const res = await fetch(`${BASE_URL}/projects/scan`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: path || '' })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to scan directory' }))
      throw new Error(err.error || 'Failed to scan directory')
    }
    return res.json()
  },

  async browseDirectory(path?: string): Promise<BrowseFSResponse> {
    const query = new URLSearchParams()
    if (path) query.set('path', path)
    const res = await fetch(`${BASE_URL}/fs/browse?${query.toString()}`)
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to browse directory' }))
      throw new Error(err.error || 'Failed to browse directory')
    }
    return res.json()
  },

  async createFolder(parentPath: string, folderName: string): Promise<CreateFolderResponse> {
    const res = await fetch(`${BASE_URL}/fs/mkdir`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ parent_path: parentPath, folder_name: folderName })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to create folder' }))
      throw new Error(err.error || 'Failed to create folder')
    }
    return res.json()
  },

  // Connectors (JIRA & Confluence)
  async getConnectors(): Promise<ConnectorsConfig> {
    const res = await fetch(`${BASE_URL}/connectors`)
    if (!res.ok) throw new Error('Failed to fetch connectors configuration')
    return res.json()
  },

  async updateJira(cfg: JiraConfig): Promise<JiraConfig> {
    const res = await fetch(`${BASE_URL}/connectors/jira`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to update JIRA configuration' }))
      throw new Error(err.error || 'Failed to update JIRA configuration')
    }
    return res.json()
  },

  async updateConfluence(cfg: ConfluenceConfig): Promise<ConfluenceConfig> {
    const res = await fetch(`${BASE_URL}/connectors/confluence`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to update Confluence configuration' }))
      throw new Error(err.error || 'Failed to update Confluence configuration')
    }
    return res.json()
  },

  async testConnector(req: TestConnectorRequest): Promise<TestConnectorResponse> {
    const res = await fetch(`${BASE_URL}/connectors/test`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to test connector' }))
      throw new Error(err.error || 'Failed to test connector')
    }
    return res.json()
  },

  async getJiraIssues(query?: string): Promise<JiraIssueDTO[]> {
    const q = new URLSearchParams()
    if (query) q.set('q', query)
    const res = await fetch(`${BASE_URL}/connectors/jira/issues?${q.toString()}`)
    if (!res.ok) throw new Error('Failed to fetch JIRA issues')
    return res.json()
  },

  async importJiraIssue(req: ImportJiraIssueRequest): Promise<Task> {
    const res = await fetch(`${BASE_URL}/connectors/jira/import`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to import JIRA issue' }))
      throw new Error(err.error || 'Failed to import JIRA issue')
    }
    return res.json()
  },

  async publishToConfluence(req: ConfluencePublishRequest): Promise<ConfluencePublishResponse> {
    const res = await fetch(`${BASE_URL}/connectors/confluence/publish`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to publish to Confluence' }))
      throw new Error(err.error || 'Failed to publish to Confluence')
    }
    return res.json()
  },

  // Modular Connector Catalog
  async getConnectorCatalog(): Promise<ConnectorItem[]> {
    const res = await fetch(`${BASE_URL}/connectors/catalog`)
    if (!res.ok) throw new Error('Failed to fetch connector catalog')
    return res.json()
  },

  async getConnector(id: string): Promise<ConnectorItem> {
    const res = await fetch(`${BASE_URL}/connectors/items/${id}`)
    if (!res.ok) throw new Error(`Failed to fetch connector ${id}`)
    return res.json()
  },

  async updateConnector(id: string, item: ConnectorItem): Promise<ConnectorItem> {
    const res = await fetch(`${BASE_URL}/connectors/items/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(item)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: `Failed to update connector ${id}` }))
      throw new Error(err.error || `Failed to update connector ${id}`)
    }
    return res.json()
  },

  async toggleConnector(id: string, enabled: boolean): Promise<ConnectorItem> {
    const res = await fetch(`${BASE_URL}/connectors/items/${id}/toggle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled } as ToggleConnectorRequest)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: `Failed to toggle connector ${id}` }))
      throw new Error(err.error || `Failed to toggle connector ${id}`)
    }
    return res.json()
  },

  async testGenericConnector(id: string, item?: Partial<ConnectorItem>): Promise<TestConnectorResponse> {
    const res = await fetch(`${BASE_URL}/connectors/items/${id}/test`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(item || {})
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: `Failed to test connector ${id}` }))
      throw new Error(err.error || `Failed to test connector ${id}`)
    }
    return res.json()
  },

  async testMCPConnector(id: string, mcp?: MCPConfig): Promise<TestConnectorResponse> {
    const res = await fetch(`${BASE_URL}/connectors/items/${id}/test-mcp`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(mcp || {})
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: `Failed to test MCP for ${id}` }))
      throw new Error(err.error || `Failed to test MCP for ${id}`)
    }
    return res.json()
  },

  async getMCPConfigExport(): Promise<{ mcpServers: Record<string, any> }> {
    const res = await fetch(`${BASE_URL}/connectors/mcp-config`)
    if (!res.ok) throw new Error('Failed to export MCP config')
    return res.json()
  },

  async pingAllConnectors(): Promise<PingAllSummary> {
    const res = await fetch(`${BASE_URL}/connectors/ping`, {
      method: 'POST'
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to ping connectors' }))
      throw new Error(err.error || 'Failed to ping connectors')
    }
    return res.json()
  },

  async getPingConfig(): Promise<ConnectorPingConfig> {
    const res = await fetch(`${BASE_URL}/connectors/ping-config`)
    if (!res.ok) throw new Error('Failed to fetch ping configuration')
    return res.json()
  },

  async updatePingConfig(cfg: ConnectorPingConfig): Promise<ConnectorPingConfig> {
    const res = await fetch(`${BASE_URL}/connectors/ping-config`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to update ping configuration' }))
      throw new Error(err.error || 'Failed to update ping configuration')
    }
    return res.json()
  },

  // Modular Skills & Claude Hub
  async getSkills(params?: { enabled_only?: boolean; format?: string; source_type?: string; search?: string }): Promise<UniversalSkillDTO[]> {
    const query = new URLSearchParams()
    if (params?.enabled_only) query.set('enabled_only', 'true')
    if (params?.format) query.set('format', params.format)
    if (params?.source_type) query.set('source_type', params.source_type)
    if (params?.search) query.set('search', params.search)

    const res = await fetch(`${BASE_URL}/skills?${query.toString()}`)
    if (!res.ok) throw new Error('Failed to fetch skills')
    return res.json()
  },

  async getSkill(name: string): Promise<UniversalSkillDTO> {
    const res = await fetch(`${BASE_URL}/skills/${encodeURIComponent(name)}`)
    if (!res.ok) throw new Error(`Failed to fetch skill ${name}`)
    return res.json()
  },

  async toggleSkill(name: string, enabled: boolean): Promise<{ name: string; enabled: boolean; status: string }> {
    const res = await fetch(`${BASE_URL}/skills/${encodeURIComponent(name)}/toggle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to toggle skill' }))
      throw new Error(err.error || 'Failed to toggle skill')
    }
    return res.json()
  },

  async getSkillSources(): Promise<SkillSourceDTO[]> {
    const res = await fetch(`${BASE_URL}/skills/sources`)
    if (!res.ok) throw new Error('Failed to fetch skill sources')
    return res.json()
  },

  async registerSkillSource(payload: { name: string; path: string; format?: string }): Promise<{ source: SkillSourceDTO; discovered_count: number; skills: UniversalSkillDTO[] }> {
    const res = await fetch(`${BASE_URL}/skills/sources`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to register skill source' }))
      throw new Error(err.error || 'Failed to register skill source')
    }
    return res.json()
  },

  async removeSkillSource(sourceId: string): Promise<{ status: string; source_id: string }> {
    const res = await fetch(`${BASE_URL}/skills/sources/${encodeURIComponent(sourceId)}`, {
      method: 'DELETE'
    })
    if (!res.ok) throw new Error('Failed to unregister skill source')
    return res.json()
  },

  async checkPathCompatibility(path: string): Promise<CheckPathCompatibilityResponse> {
    const res = await fetch(`${BASE_URL}/skills/check-compatibility`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path })
    })
    if (!res.ok) throw new Error('Failed to check path compatibility')
    return res.json()
  },

  async rescanSkills(): Promise<{ status: string; total_count: number }> {
    const res = await fetch(`${BASE_URL}/skills/rescan`, {
      method: 'POST'
    })
    if (!res.ok) throw new Error('Failed to rescan skills')
    return res.json()
  },

  // Prompts (Multi-Tier & Dynamic Parameter Extraction)
  async getPrompts(params?: { search?: string; source?: string }): Promise<PromptItemDTO[]> {
    const query = new URLSearchParams()
    if (params?.search) query.set('search', params.search)
    if (params?.source) query.set('source', params.source)
    const res = await fetch(`${BASE_URL}/prompts?${query.toString()}`)
    if (!res.ok) throw new Error('Failed to fetch prompt templates')
    return res.json()
  },

  async getPromptSources(): Promise<PromptSourceDTO[]> {
    const res = await fetch(`${BASE_URL}/prompts/sources`)
    if (!res.ok) throw new Error('Failed to fetch prompt sources')
    return res.json()
  },

  async registerPromptSource(payload: { name: string; path: string }): Promise<{ source: PromptSourceDTO; discovered_count: number; templates: PromptItemDTO[] }> {
    const res = await fetch(`${BASE_URL}/prompts/sources`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to register prompt source' }))
      throw new Error(err.error || 'Failed to register prompt source')
    }
    return res.json()
  },

  async removePromptSource(sourceId: string): Promise<{ status: string; source_id: string }> {
    const res = await fetch(`${BASE_URL}/prompts/sources/${encodeURIComponent(sourceId)}`, {
      method: 'DELETE'
    })
    if (!res.ok) throw new Error('Failed to unregister prompt source')
    return res.json()
  },

  async checkPromptCompatibility(path: string): Promise<{ compatible: boolean; path: string; discovered_count: number; templates: PromptItemDTO[]; error?: string }> {
    const res = await fetch(`${BASE_URL}/prompts/check-compatibility`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path })
    })
    if (!res.ok) throw new Error('Failed to check prompt compatibility')
    return res.json()
  },

  async renderPrompt(payload: { template_id?: string; raw_template?: string; parameters: Record<string, string> }): Promise<{ template_id: string; rendered: string }> {
    const res = await fetch(`${BASE_URL}/prompts/render`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to render prompt' }))
      throw new Error(err.error || 'Failed to render prompt')
    }
    return res.json()
  }
}




