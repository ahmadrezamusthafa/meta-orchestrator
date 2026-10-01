import type { 
  Task, TaskProcessDTO, ToolDTO, ProviderDTO, ProviderUsage, ProviderUsageWindow, ProviderQuota, ProviderConnection, WorkflowDefinition, RegistryDTO, BenchmarkCellDTO, 
  Project, ScanDirResult, BrowseFSResponse, CreateFolderResponse,
  ConnectorsConfig, JiraConfig, ConfluenceConfig, JiraIssueDTO, ImportJiraIssueRequest,
  JiraSyncConfig, JiraSyncSettings,
  ConfluencePublishRequest, ConfluencePublishResponse, TestConnectorRequest, TestConnectorResponse,
  ConnectorItem, ToggleConnectorRequest, MCPConfig, ConnectorPingConfig, PingAllSummary,
  UniversalSkillDTO, SkillSourceDTO, CheckPathCompatibilityResponse,
  PromptItemDTO, PromptSourceDTO, TaskWorktreeDTO, TaskDependencyInfoDTO,
  OAuthStatus, RouterMode, PriorityModelItem, RouterSettingsDTO, RoutePreviewDTO, TierAssignmentDTO,
  BenchmarksResponseDTO, TelemetrySummaryDTO, TelemetryTrendsDTO, TelemetryWindow,
  TaskActivityResponse, TaskChatResponse, TaskDiffDTO, DiffAgainst, TaskArtifactDTO,
  PullRequestDraftDTO, PullRequestOpenedDTO, UATGuideStatusDTO, UATGuideSettings,
  TaskAnalysisDTO, TaskRoutingDTO, StageRoute
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

  async getTaskProcess(taskId: string): Promise<TaskProcessDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/process`)
    if (!res.ok) throw new Error(`Failed to fetch background process for task ${taskId}`)
    return res.json()
  },

  async executeTaskProcessCommand(taskId: string, command: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/process/execute`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command })
    })
    if (!res.ok) throw new Error('Failed to execute command in background process')
    return res.json()
  },

  async clearTaskProcessLogs(taskId: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/process/clear`, {
      method: 'POST'
    })
    if (!res.ok) throw new Error('Failed to clear process logs')
    return res.json()
  },

  // Agent Console (structured activity transcript)
  async getTaskActivity(taskId: string, after?: string): Promise<TaskActivityResponse> {
    const query = new URLSearchParams()
    if (after) query.set('after', after)
    const qs = query.toString()
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/activity${qs ? `?${qs}` : ''}`)
    if (!res.ok) throw new Error(`Failed to fetch activity for task ${taskId}`)
    return res.json()
  },

  async clearTaskActivity(taskId: string): Promise<{ status: string }> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/activity`, {
      method: 'DELETE'
    })
    if (!res.ok) {
      const errData = await res.json().catch(() => null)
      throw new Error(errData?.error || `Failed to clear activity for task ${taskId}`)
    }
    return res.json()
  },

  async sendTaskChat(taskId: string, message: string, model?: string): Promise<TaskChatResponse> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(model ? { message, model } : { message })
    })
    if (!res.ok) {
      const errData = await res.json().catch(() => null)
      const err = new Error(errData?.error || `Failed to send message (HTTP ${res.status})`) as Error & { status?: number }
      err.status = res.status
      throw err
    }
    return res.json()
  },

  async cancelTaskChat(taskId: string): Promise<{ status: 'cancelled' | 'idle' | string }> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/chat/cancel`, {
      method: 'POST'
    })
    if (!res.ok) {
      const errData = await res.json().catch(() => null)
      throw new Error(errData?.error || `Failed to cancel turn for task ${taskId}`)
    }
    return res.json()
  },

  async resumeTask(taskId: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/resume`, {
      method: 'POST'
    })
    if (!res.ok) {
      const errData = await res.json().catch(() => null)
      throw new Error(errData?.error || `Failed to resume task ${taskId}`)
    }
    return res.json()
  },

  async pauseTask(taskId: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/pause`, {
      method: 'POST'
    })
    if (!res.ok) {
      const errData = await res.json().catch(() => null)
      throw new Error(errData?.error || `Failed to pause task ${taskId}`)
    }
    return res.json()
  },

  async getTaskWorktree(taskId: string): Promise<TaskWorktreeDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/worktree`)
    if (!res.ok) throw new Error(`Failed to fetch worktree for task ${taskId}`)
    return res.json()
  },

  // AI grades the draft task's complexity and the router proposes a method/model per stage.
  // Passing complexity re-plans with the operator's grade instead of asking the AI.
  async analyzeTask(payload: {
    title: string; description?: string; assigned_repos?: string[]; complexity?: string; task_type?: string
    start_stage?: string; halt_stage?: string
  }): Promise<TaskAnalysisDTO> {
    const res = await fetch(`${BASE_URL}/tasks/analyze`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error((await res.json().catch(() => null))?.error || 'Failed to analyze task')
    return res.json()
  },

  async getTaskRouting(taskId: string): Promise<TaskRoutingDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/routing`)
    if (!res.ok) throw new Error(`Failed to load routing plan for ${taskId}`)
    return res.json()
  },

  async updateTaskRouting(taskId: string, body: { complexity?: string; routing_plan: StageRoute[] }): Promise<Task> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/routing`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error((await res.json().catch(() => null))?.error || 'Failed to update routing plan')
    return res.json()
  },

  async getTaskDependencies(taskId: string): Promise<TaskDependencyInfoDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/dependencies`)
    if (!res.ok) throw new Error(`Failed to fetch dependencies for task ${taskId}`)
    return res.json()
  },

  async patchTask(taskId: string, payload: { current_stage_id?: string; state?: string; assigned_repos?: string[] }): Promise<Task> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || `Failed to update task ${taskId}`)
    }
    return res.json()
  },

  async decideApproval(taskId: string, approvalId: string, decision: 'allow' | 'always' | 'all' | 'deny', message?: string): Promise<{ status: string }> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/approvals/${encodeURIComponent(approvalId)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decision, message }),
    })
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || 'Could not send the decision')
    }
    return res.json()
  },

  async setAllowAllApprovals(taskId: string, allowAll: boolean): Promise<{ allow_all: boolean }> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/approvals`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ allow_all: allowAll }),
    })
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || 'Could not change the approval mode')
    }
    return res.json()
  },

  async getTaskArtifacts(taskId: string): Promise<{ task_id: string; artifacts: TaskArtifactDTO[] }> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/artifacts`)
    if (!res.ok) throw new Error(`Failed to list documents for ${taskId}`)
    return res.json()
  },

  async getPullRequestDrafts(taskId: string): Promise<{ task_id: string; stage_ready: boolean; drafts: PullRequestDraftDTO[] }> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/pull-request`)
    if (!res.ok) throw new Error(`Failed to prepare the pull request for ${taskId}`)
    return res.json()
  },

  async openPullRequest(taskId: string, body: { repo: string; title: string; body: string }): Promise<PullRequestOpenedDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/pull-request`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    const data = await res.json().catch(() => null)
    if (!res.ok) throw new Error(data?.error || `Could not open the pull request for ${taskId}`)
    return data
  },

  async getUATGuide(taskId: string): Promise<UATGuideStatusDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/uat-guide`)
    if (!res.ok) throw new Error(`Failed to load the UAT guide for ${taskId}`)
    return res.json()
  },

  async generateUATGuide(taskId: string, settings: UATGuideSettings): Promise<UATGuideStatusDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/uat-guide`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings),
    })
    const data = await res.json().catch(() => null)
    if (!res.ok) throw new Error(data?.error || `Could not generate the UAT guide for ${taskId}`)
    return data
  },

  async uatSignIn(taskId: string, app: string, forget = false): Promise<UATGuideStatusDTO> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/uat-guide/login`, {
      method: forget ? 'DELETE' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ app }),
    })
    const data = await res.json().catch(() => null)
    if (!res.ok) throw new Error(data?.error || `Could not ${forget ? 'forget the session' : 'open the sign-in window'} for ${app}`)
    return data
  },

  async getTaskDiff(taskId: string, against: DiffAgainst, repo?: string): Promise<TaskDiffDTO> {
    const q = new URLSearchParams({ against })
    if (repo) q.set('repo', repo)
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/diff?${q.toString()}`)
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || `Failed to load changes for ${taskId}`)
    }
    return res.json()
  },

  async deleteTask(taskId: string): Promise<{ status: string; task_id: string }> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}`, { method: 'DELETE' })
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || `Failed to delete task ${taskId}`)
    }
    return res.json()
  },

  async executeTask(taskId: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/tasks/${taskId}/execute`, {
      method: 'POST'
    })
    if (!res.ok) {
      const errData = await res.json().catch(() => null)
      throw new Error(errData?.error || `Failed to execute task ${taskId} (HTTP ${res.status})`)
    }
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

  async getProviderConnections(refresh = false): Promise<{ connections: Record<string, ProviderConnection> }> {
    const res = await fetch(`${BASE_URL}/providers/connection${refresh ? '?refresh=1' : ''}`)
    if (!res.ok) throw new Error('Failed to check provider connections')
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

  async saveProviderConfig(providerId: string, config: { api_key?: string; clear_api_key?: boolean; base_url?: string; model?: string; session_token?: string; auth_method?: string }): Promise<any> {
    const res = await fetch(`${BASE_URL}/providers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider_id: providerId, ...config })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to save provider config' }))
      throw new Error(err.error || 'Failed to save provider config')
    }
    return res.json()
  },

  async getProviderUsage(window: ProviderUsageWindow = '7d', refresh = false): Promise<{ window: ProviderUsageWindow; generated_at: string; providers: ProviderUsage[] }> {
    const res = await fetch(`${BASE_URL}/providers/usage?window=${window}${refresh ? '&refresh=1' : ''}`)
    if (!res.ok) throw new Error('Failed to fetch provider usage')
    return res.json()
  },

  async saveProviderQuota(providerId: string, quota: ProviderQuota): Promise<any> {
    const res = await fetch(`${BASE_URL}/providers/quota`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider_id: providerId, ...quota })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to save quota' }))
      throw new Error(err.error || 'Failed to save quota')
    }
    return res.json()
  },

  async getRouterSettings(): Promise<RouterSettingsDTO> {
    const res = await fetch(`${BASE_URL}/router/settings`)
    if (!res.ok) throw new Error('Failed to fetch router settings')
    return res.json()
  },

  async updateRouterSettings(payload: { mode: RouterMode; priority_chain: PriorityModelItem[] }): Promise<any> {
    const res = await fetch(`${BASE_URL}/router/settings`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to update router settings' }))
      throw new Error(err.error || 'Failed to update router settings')
    }
    return res.json()
  },

  async previewRouter(payload: { mode: RouterMode; priority_chain: PriorityModelItem[] }): Promise<{ preview: RoutePreviewDTO[]; tiers: TierAssignmentDTO[] }> {
    const res = await fetch(`${BASE_URL}/router/preview`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) throw new Error('Failed to preview routing')
    return res.json()
  },

  async registerCustomModel(payload: {
    provider_id: string
    model_id: string
    model_name?: string
    cost_per_1k?: number
    latency_ms?: number
  }): Promise<any> {
    const res = await fetch(`${BASE_URL}/router/models`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to register model' }))
      throw new Error(err.error || 'Failed to register model')
    }
    return res.json()
  },

  async getBenchmarks(): Promise<BenchmarksResponseDTO> {
    const res = await fetch(`${BASE_URL}/benchmarks`)
    if (!res.ok) throw new Error('Failed to fetch benchmark matrix')
    return res.json()
  },

  // Telemetry & Analytics (Phase 4)
  async getTelemetrySummary(window: TelemetryWindow, repo = ''): Promise<TelemetrySummaryDTO> {
    const query = new URLSearchParams({ window, repo })
    const res = await fetch(`${BASE_URL}/telemetry/summary?${query.toString()}`)
    if (!res.ok) throw new Error('Failed to fetch telemetry summary')
    return res.json()
  },

  async getTelemetryTrends(window: TelemetryWindow, repo = ''): Promise<TelemetryTrendsDTO> {
    const query = new URLSearchParams({ window, repo })
    const res = await fetch(`${BASE_URL}/telemetry/trends?${query.toString()}`)
    if (!res.ok) throw new Error('Failed to fetch telemetry trends')
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
  async getArtifact(taskId: string, filename: string): Promise<{ task_id: string; filename: string; content: string; modified_at?: string }> {
    const path = filename.split('/').map(encodeURIComponent).join('/')
    const res = await fetch(`${BASE_URL}/artifacts/${encodeURIComponent(taskId)}/${path}`)
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || `Could not open ${filename}`)
    }
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

  async browseDirectory(path?: string, showHidden: boolean = true): Promise<BrowseFSResponse> {
    const query = new URLSearchParams()
    if (path) query.set('path', path)
    query.set('show_hidden', showHidden ? 'true' : 'false')
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
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || 'Failed to fetch JIRA issues')
    }
    return (await res.json()) || []
  },

  async getJiraSync(): Promise<JiraSyncSettings> {
    const res = await fetch(`${BASE_URL}/connectors/jira/sync`)
    if (!res.ok) throw new Error('Failed to load JIRA sync rules')
    return res.json()
  },

  async updateJiraSync(cfg: JiraSyncConfig): Promise<JiraSyncSettings> {
    const res = await fetch(`${BASE_URL}/connectors/jira/sync`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg)
    })
    if (!res.ok) {
      const err = await res.json().catch(() => null)
      throw new Error(err?.error || 'Failed to save JIRA sync rules')
    }
    return res.json()
  },

  // Resolves with the sync outcome even when JIRA fails: status.last_error explains why.
  async runJiraSync(): Promise<JiraSyncSettings> {
    const res = await fetch(`${BASE_URL}/connectors/jira/sync/run`, { method: 'POST' })
    const data = await res.json().catch(() => null)
    if (!data?.status) throw new Error(data?.error || 'Failed to sync JIRA')
    return { config: data.config, status: data.status }
  },

  async clearJiraDismissed(): Promise<JiraSyncSettings> {
    const res = await fetch(`${BASE_URL}/connectors/jira/sync/dismissed`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to restore dismissed JIRA issues')
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
  },

  // OAuth Provider Authentication
  async initiateOAuth(providerId: string, redirectUri?: string): Promise<{ auth_url: string; state: string; provider_id: string; scopes: string }> {
    const res = await fetch(`${BASE_URL}/providers/oauth/initiate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider_id: providerId, redirect_uri: redirectUri })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to initiate OAuth' }))
      throw new Error(err.error || 'Failed to initiate OAuth')
    }
    return res.json()
  },

  async getOAuthStatus(providerId: string): Promise<OAuthStatus> {
    const res = await fetch(`${BASE_URL}/providers/oauth/status?provider_id=${encodeURIComponent(providerId)}`)
    if (!res.ok) throw new Error('Failed to fetch OAuth status')
    return res.json()
  },

  async disconnectOAuth(providerId: string): Promise<{ status: string; provider_id: string }> {
    const res = await fetch(`${BASE_URL}/providers/oauth/disconnect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider_id: providerId })
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Failed to disconnect OAuth' }))
      throw new Error(err.error || 'Failed to disconnect OAuth')
    }
    return res.json()
  }
}




