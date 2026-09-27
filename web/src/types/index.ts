export type TaskState = 
  | 'PENDING'
  | 'RUNNING'
  | 'WAITING_GATE_APPROVAL'
  | 'BLOCKED_FRUSTRATION'
  | 'COMPLETED'
  | 'FAILED'
  | 'SUSPENDED'

export interface TokenUsage {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  estimated_cost_usd: number
}

export interface StageSlice {
  start_stage_id: string
  halt_stage_id: string
  produce_video: boolean
}

export interface Task {
  id: string
  workflow_id: string
  title: string
  description: string
  current_stage_id: string
  current_stage_index: number
  state: TaskState
  active_slice?: StageSlice
  assigned_repos: string[]
  profile_name: string
  selected_method: string
  token_usage: TokenUsage
  max_token_budget: number
  artifact_dir: string
  metadata?: Record<string, string>
  created_at: string
  updated_at: string
}

export interface WorkflowStage {
  id: string
  name: string
  type: 'automated' | 'review_gate' | 'manual_verification'
  assigned_role: string
  allowed_methods: string[]
  write_lock_workspace: boolean
  requires_gate: boolean
  gate_criteria?: string
  required_artifacts?: string[]
}

export interface WorkflowDefinition {
  id: string
  name: string
  description: string
  version: string
  stages: WorkflowStage[]
  default_role: string
}

export interface OrchestratorEvent {
  type: string
  task_id: string
  stage_id: string
  timestamp: string
  payload: any
}

export interface ToolDTO {
  id: string
  name: string
  category: string
  current_version: string
  latest_version: string
  best_fit_version: string
  status: 'HEALTHY' | 'UPDATE_AVAILABLE' | 'ERROR' | 'NOT_INSTALLED'
  description: string
  past_versions: string[]
}

export interface ProviderDTO {
  id: string
  name: string
  enabled: boolean
  latency_ms: number
  masked_api_key: string
  base_url?: string
  default_model: string
  models: string[]
}

export interface TierMappingDTO {
  tier_1_reasoning: { provider_id: string; model_id: string }
  tier_2_codegen: { provider_id: string; model_id: string }
  tier_3_log_parsing: { provider_id: string; model_id: string }
}

export interface BenchmarkCellDTO {
  stage_id: string
  complexity: 'LOW' | 'MEDIUM' | 'HIGH' | 'SYSTEM'
  optimal_method: string
  winning_model: string
  fpvr_percent: number
  avg_tokens: number
  avg_duration_s: number
}

export interface RegistryDTO {
  skills: Array<{
    id: string
    name: string
    source: string
    format: string
    description: string
    repo_url?: string
  }>
  prompts: Array<{
    id: string
    name: string
    source: string
    variables: string[]
    system_override: boolean
  }>
  hooks: Array<{
    id: string
    event: string
    type: string
    command: string
    policy: string
  }>
}

export type ProjectRole = 'frontend' | 'backend' | 'automation-test' | 'contracts' | 'artifact' | 'other'

export interface ProjectRepo {
  id: string
  name: string
  path: string
  role: ProjectRole
  manifest_type: string
  symlink_path?: string
  status: 'linked' | 'missing_source' | 'error' | string
  error?: string
  git_branch?: string
  files_count?: number
  created_at?: string
}

export interface Project {
  id: string
  name: string
  description: string
  root_dir: string
  active_sdlc: string
  repos: ProjectRepo[]
  status: 'provisioned' | 'pending' | 'degraded' | string
  created_at: string
  updated_at: string
}

export interface DetectedRepo {
  name: string
  path: string
  suggested_role: ProjectRole
  manifest_type: string
  files_count: number
}

export interface ScanDirResult {
  scanned_path: string
  detected_repos: DetectedRepo[]
}

export interface BreadcrumbItem {
  name: string
  path: string
}

export interface DirectoryItem {
  name: string
  path: string
  is_repo: boolean
  manifest: string
  suggested_role: ProjectRole
  has_children: boolean
}

export interface QuickBookmark {
  name: string
  path: string
  icon: string
}

export interface BrowseFSResponse {
  current_path: string
  parent_path: string
  breadcrumbs: BreadcrumbItem[]
  directories: DirectoryItem[]
  quick_bookmarks: QuickBookmark[]
}

export interface CreateFolderRequest {
  parent_path?: string
  folder_name?: string
  path?: string
}

export interface CreateFolderResponse {
  success: boolean
  path: string
  name: string
  parent_path: string
}

export interface JiraConfig {
  enabled: boolean
  base_url: string
  username: string
  api_token: string
  project_key: string
  jql_filter: string
  auto_detect_keys: boolean
  auto_sync_status: boolean
  last_tested_at?: string
  status: 'connected' | 'configured' | 'error' | 'unconfigured'
  error_message?: string
}

export interface ConfluenceConfig {
  enabled: boolean
  base_url: string
  username: string
  api_token: string
  space_key: string
  parent_page_id?: string
  auto_publish_tech_docs: boolean
  auto_publish_prd: boolean
  last_tested_at?: string
  status: 'connected' | 'configured' | 'error' | 'unconfigured'
  error_message?: string
}

export type ConnectorCategory = 'issue_tracker' | 'documentation' | 'chatops' | 'vcs' | 'custom'

export interface MCPConfig {
  enabled: boolean
  command: string
  args: string[]
  env?: Record<string, string>
  transport: 'stdio' | 'sse' | 'streamable_http'
  endpoint_url?: string
}

export interface ConnectorItem {
  id: string
  name: string
  category: ConnectorCategory
  category_label: string
  description: string
  icon: string
  color: string
  enabled: boolean
  config_mode?: 'rest' | 'mcp' | 'hybrid'
  status: 'connected' | 'configured' | 'disabled' | 'unconfigured' | 'error'
  base_url: string
  username: string
  api_token: string
  target_entity: string
  target_label: string
  capabilities?: string[]
  extra_settings?: Record<string, any>
  mcp?: MCPConfig
  last_tested_at?: string
  latency_ms?: number
  error_message?: string
}

export interface ToggleConnectorRequest {
  enabled: boolean
}

export interface ConnectorPingConfig {
  enabled: boolean
  interval_seconds: number
}

export interface PingAllSummary {
  timestamp: string
  total_pinged: number
  results: Record<string, TestConnectorResponse>
}

export interface ConnectorsConfig {
  jira: JiraConfig
  confluence: ConfluenceConfig
  items?: ConnectorItem[]
  ping?: ConnectorPingConfig
}

export interface JiraIssueDTO {
  key: string
  summary: string
  description: string
  status: string
  priority: string
  issue_type: string
  url: string
  reporter?: string
  assignee?: string
  created?: string
}

export interface ImportJiraIssueRequest {
  issue_key: string
  workflow_id?: string
  selected_method?: string
  assigned_repos?: string[]
  start_stage_id?: string
  active_slice?: StageSlice
  metadata?: Record<string, string>
}

export interface ConfluencePublishRequest {
  task_id?: string
  doc_type?: string
  title?: string
  content_markdown?: string
  space_key?: string
  parent_page_id?: string
}

export interface ConfluencePublishResponse {
  success: boolean
  page_id: string
  page_title: string
  page_url: string
  space_key: string
  published_at: string
  version: number
}

export interface TestConnectorRequest {
  type: 'jira' | 'confluence'
  jira?: JiraConfig
  confluence?: ConfluenceConfig
}

export interface TestConnectorResponse {
  success: boolean
  latency_ms: number
  message: string
  connected_as?: string
  server_info?: string
  target_entity?: string
}


