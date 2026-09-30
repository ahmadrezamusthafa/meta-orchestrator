export type TaskState = 
  | 'PENDING'
  | 'RUNNING'
  | 'WAITING_GATE_APPROVAL'
  | 'BLOCKED_FRUSTRATION'
  | 'WAITING_DEPENDENCY'
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
  dependencies?: string[]
  profile_name: string
  selected_method: string
  token_usage: TokenUsage
  max_token_budget: number
  artifact_dir: string
  metadata?: Record<string, string>
  created_at: string
  updated_at: string
}

export interface TaskRepoWorktreeDTO {
  repo: string
  source_path: string
  worktree_path: string
  checkout_path: string
  sub_path?: string
  branch: string
  base_ref: string
  exists: boolean
  dirty: boolean
  error?: string
}

export interface TaskWorktreeDTO {
  task_id: string
  is_worktree: boolean
  use_worktree: boolean
  worktree_path: string
  branch: string
  base_ref: string
  parallel_isolation: boolean
  status: 'PLANNED' | 'ACTIVE' | 'DISABLED' | 'NO_REPOS' | string
  assigned_repos?: string[]
  repos?: TaskRepoWorktreeDTO[]
  error?: string
}

export interface PullRequestDraftDTO {
  repo: string
  repos: string[]
  provider: 'bitbucket' | 'github' | string
  repo_url?: string
  source_branch: string
  target_branch: string
  title: string
  body: string
  commit_message: string
  uncommitted: boolean
  commits: number
  files: number
  existing_url?: string
  can_create: boolean
  blocker?: string
}

export interface PullRequestOpenedDTO {
  repo: string
  url: string
  number: number
  updated: boolean
  committed: boolean
}

export interface UATAppDTO {
  id: string
  name: string
  kind: 'web' | 'api' | string
  audience?: string
  summary?: string
  sign_in?: string[]
  repos?: string[]
  base_url?: string
  storage_state?: string
}

export interface UATScopeCaseDTO {
  id: string
  title: string
  kind: 'Sanity' | 'UAT' | 'Sanity + UAT' | string
  priority: string
  platform: string
  app: string
}

export interface UATVariableDTO {
  name: string
  value?: string
  source: 'task' | 'environment' | 'missing' | string
  secret: boolean
  used: boolean
  browser: boolean
}

export interface UATGuideStatusDTO {
  task_id: string
  status: 'NOT_STARTED' | 'GENERATING' | 'READY' | 'FAILED' | string
  error?: string
  generated_at?: string
  ignore_https_errors: boolean
  stage_ready: boolean
  can_request_changes: boolean
  guide_path?: string
  html_path?: string
  apps: UATAppDTO[]
  variables?: UATVariableDTO[]
  seed?: { language?: string; run?: string; script: string } | null
  seed_check?: {
    issues: { severity: 'error' | 'warning' | string; line?: number; message: string }[]
    syntax: { checked: boolean; ok: boolean; tool?: string; message?: string }
  } | null
  atdd: { source: string; path?: string; error?: string; total: number; in_scope: number; sanity: number; cases: UATScopeCaseDTO[] }
  coverage: { planned?: string; missing?: string[] }
}

export interface UATGuideSettings {
  apps?: Record<string, { url?: string; storage_state?: string }>
  atdd_path?: string
  variables?: Record<string, string>
  ignore_https_errors?: boolean
  save_only?: boolean
}

export interface TaskArtifactDTO {
  path: string
  name: string
  kind: 'stage_output' | 'document' | 'media' | 'other'
  stage_id?: string
  size: number
  modified_at: string
}

export type DiffAgainst = 'base' | 'head'

export interface DiffFileDTO {
  path: string
  old_path?: string
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'untracked'
  additions: number
  deletions: number
  binary?: boolean
}

export interface RepoDiffDTO {
  repo: string
  branch?: string
  base_ref?: string
  compare_ref?: string
  commits?: { sha: string; subject: string }[]
  files?: DiffFileDTO[]
  additions?: number
  deletions?: number
  patch?: string
  truncated?: boolean
  error?: string
}

export interface TaskDiffDTO {
  task_id: string
  against: DiffAgainst
  repos: RepoDiffDTO[]
}

export interface TaskDependencyInfoDTO {
  task_id: string
  dependencies: string[]
  all_satisfied: boolean
  unmet_dependencies: string[]
  details: Array<{
    id: string
    title: string
    state: TaskState
    current_stage_id: string
  }>
}

export interface TaskSubProcessDTO {
  pid: number
  command: string
  status: string
}

export interface TaskProcessDTO {
  task_id: string
  process_id: number
  command: string
  working_dir: string
  container_id: string
  status: 'RUNNING' | 'IDLE' | 'COMPLETED' | 'PAUSED' | 'BLOCKED' | 'FAILED'
  started_at: string
  duration_seconds: number
  cpu_percent: number
  memory_mb: number
  current_step: string
  active_agent: string
  exit_code?: number
  subprocesses?: TaskSubProcessDTO[]
  logs: string[]
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
  auth_method: 'api_key' | 'oauth' | 'session_token'
  supports_oauth: boolean
  /** A real API key is saved (masked_api_key shows it masked). */
  has_api_key?: boolean
}

export type ProviderConnectionStatus = 'connected' | 'not_configured' | 'invalid' | 'expired' | 'unreachable'

/** What the orchestrator actually uses to reach a provider, verified live. */
export interface ProviderConnection {
  provider_id: string
  status: ProviderConnectionStatus
  method: 'api_key' | 'claude_cli' | 'local_endpoint' | 'none'
  label: string
  detail?: string
  hint?: string
  plan?: string
  latency_ms?: number
  checked_at: string
  /** A saved session token / OAuth login that no request uses. */
  legacy_auth?: 'session_token' | 'oauth'
}

export interface ProviderUsageTotals {
  calls: number
  prompt_tokens: number
  completion_tokens: number
  cached_tokens: number
  total_tokens: number
  cost_usd: number
}

export interface ProviderModelUsage extends ProviderUsageTotals {
  model: string
}

export interface ProviderQuota {
  monthly_token_limit: number
  monthly_cost_limit_usd: number
}

export interface ProviderQuotaStatus extends ProviderQuota {
  token_percent?: number
  cost_percent?: number
  exceeded: boolean
}

export interface ProviderLimitWindow {
  id: string
  label: string
  used_percent: number
  resets_at?: string
  models?: string[]
}

/** Provider-reported plan usage (what `claude /usage` or Antigravity's quota view shows). */
export interface ProviderLimits {
  provider_id: string
  status: 'ok' | 'unavailable' | 'stale' | 'error'
  source: string
  plan?: string
  message?: string
  windows: ProviderLimitWindow[]
  fetched_at: string
}

export interface ProviderUsage {
  provider_id: string
  limits?: ProviderLimits
  window: ProviderUsageTotals
  last_24h: ProviderUsageTotals
  month_to_date: ProviderUsageTotals
  models: ProviderModelUsage[]
  last_used_at?: string
  quota: ProviderQuotaStatus
  source: string
}

export type ProviderUsageWindow = '24h' | '7d' | '30d'

export interface OAuthStatus {
  provider_id: string
  auth_method: 'api_key' | 'oauth' | 'session_token'
  is_connected: boolean
  connected_at?: string
  email?: string
  expires_at?: string
  token_type?: string
  scopes?: string
}

export type RouterMode =
  | 'priority_sequence'
  | 'best_practice'
  | 'cost_optimized'
  | 'latency_optimized'
  | 'round_robin'

export type ModelTier = 'tier1' | 'tier2' | 'tier3'

export interface PriorityModelItem {
  id: string
  provider: string
  model: string
  name: string
  enabled: boolean
  cost_per_1k: number
  latency_ms: number
  /** Tier tags; best practice picks the highest-ranked enabled item tagged for the stage's tier. */
  tiers?: ModelTier[]
}

/** A recommended model the router did not run because the chain does not allow it. */
export interface ModelSuggestion {
  model: string
  tier?: ModelTier
  action: 'add' | 'enable' | 'connect'
  reason: string
}

export interface RoutingDecisionDTO {
  strategy: string
  model: string
  tier?: ModelTier
  fallback_chain: string[]
  method: string
  token_budget: number
  reasoning: string
  suggestion?: ModelSuggestion
}

export interface RoutePreviewDTO {
  label: string
  stage: string
  complexity: string
  decision: RoutingDecisionDTO
}

export interface TierAssignmentDTO {
  tier: ModelTier
  recommended: string
  model: string
  suggestion?: ModelSuggestion
}

export interface RouterModeDTO {
  id: RouterMode
  name: string
  description: string
}

export interface AvailableModelDTO {
  provider_id: string
  provider_name: string
  model_id: string
  model_name: string
  cost_per_1k: number
  latency_ms: number
  tiers?: ModelTier[]
}

export interface RouterSettingsDTO {
  mode: RouterMode
  priority_chain: PriorityModelItem[]
  default_chain: PriorityModelItem[]
  available_modes: RouterModeDTO[]
  all_models: AvailableModelDTO[]
  preview: RoutePreviewDTO[]
  tiers: TierAssignmentDTO[]
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
  // Phase 4 shadow-benchmark fields (optional for backward compatibility)
  model_tier?: string
  avg_cost_usd?: number
  score?: number
  samples?: number
  // 'shadow_benchmark' = measured; 'default' = best-practice policy, never benchmarked
  source?: 'shadow_benchmark' | 'default' | string
}

export interface BenchmarksResponseDTO {
  total_cells: number
  measured_cells?: number
  generated_at?: string
  source?: 'shadow_benchmark' | 'default' | string
  matrix: BenchmarkCellDTO[]
}

export interface PromptItemDTO {
  id: string
  name: string
  source: string
  source_path?: string
  role?: string
  description?: string
  variables: string[]
  raw_content?: string
  system_override: boolean
  compatible?: boolean
}

export interface PromptSourceDTO {
  id: string
  name: string
  path: string
  template_count: number
  added_at: string
}


export interface HookItemDTO {
  id: string
  event: string
  type: string
  command: string
  policy: string
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
  prompts: PromptItemDTO[]
  hooks: HookItemDTO[]
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
  is_hidden?: boolean
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
  has_env_auth?: boolean
  env_auth_source?: string
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
  parent_key?: string
  epic_key?: string
  epic_summary?: string
}

export interface JiraSyncConfig {
  enabled: boolean
  jql: string
  interval_seconds: number
  max_issues: number
  workflow_id: string
  selected_method: string
  assigned_repos: string[] | null
  default_stage_id: string
  status_stage_map?: Record<string, string> | null
  update_existing: boolean
  exclude_statuses: string[] | null
  repo_rules?: Record<string, string[]> | null
}

export interface JiraSyncStatus {
  connected: boolean
  running: boolean
  last_run_at?: string
  last_error?: string
  fetched: number
  created: number
  updated: number
  skipped: number
  dismissed: number
  next_run_at?: string
  linked_tasks: number
}

export interface JiraSyncSettings {
  config: JiraSyncConfig
  status: JiraSyncStatus
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

export type SkillFormat = 'claude' | 'bmad' | 'superpower' | 'mcp' | 'openai' | 'native'
export type SkillCompatibilityStatus = 'compatible' | 'warning' | 'incompatible'

export interface SkillCompatibilityIssue {
  severity: 'error' | 'warning' | 'info'
  check: string
  message: string
  suggestion?: string
}

export interface SkillCompatibility {
  status: SkillCompatibilityStatus
  score: number
  issues: SkillCompatibilityIssue[]
  checked_at: string
  runtime_ready: boolean
  sandbox_safe: boolean
}

export interface UniversalSkillDTO {
  name: string
  description: string
  source_format: SkillFormat
  source_location?: string
  source_type?: string
  enabled: boolean
  compatibility?: SkillCompatibility
  isolation: 'subprocess' | 'docker' | 'host'
  input_schema?: Record<string, any>
  required_roles?: string[]
  timeout_seconds: number
  requires_network: boolean
  command?: string
  args?: string[]
  env?: Record<string, string>
  metadata?: Record<string, any>
}

export interface SkillSourceDTO {
  id: string
  name: string
  path: string
  format: SkillFormat
  enabled: boolean
  skill_count: number
  added_at: string
  description?: string
}

export interface CheckPathCompatibilityResponse {
  compatible: boolean
  path: string
  discovered_count: number
  skills: UniversalSkillDTO[]
  error?: string
}

// ─── Phase 4: Telemetry & Analytics ────────────────────────────────────────

export type TelemetryWindow = '24h' | '7d' | '30d' | 'all'

export interface TelemetryTotalsDTO {
  runs: number
  prompt_tokens: number
  completion_tokens: number
  cached_tokens: number
  total_tokens: number
  cost_usd: number
  fpvr_percent: number
  mttr_seconds: number
}

export interface TelemetryCategoryDTO {
  category: string
  runs: number
  avg_tpf_tokens: number
  avg_cost_usd: number
  fpvr_percent: number
}

export interface LeaderboardEntryDTO {
  model: string
  tier: string
  tasks_completed: number
  fpvr_percent: number
  avg_tpf_tokens: number
  avg_cost_usd: number
  avg_ttr_seconds: number
}

export interface MethodStatDTO {
  method: string
  runs: number
  total_cost_usd: number
  avg_cost_usd: number
  avg_tpf_tokens: number
  fpvr_percent: number
  avg_ttr_seconds: number
}

export interface RepoStabilityDTO {
  repo: string
  runs: number
  failure_loops: number
  avg_test_iterations: number
  flakiness_index: number // 0..1
}

export interface TelemetrySummaryDTO {
  window: TelemetryWindow | string
  repo: string
  generated_at: string
  query_latency_ms: number
  totals: TelemetryTotalsDTO
  by_category: TelemetryCategoryDTO[]
  leaderboard: LeaderboardEntryDTO[]
  methods: MethodStatDTO[]
  stability: RepoStabilityDTO[]
  repos: string[]
}

export interface TokenBurnPointDTO {
  date: string
  prompt_tokens: number
  completion_tokens: number
  cached_tokens: number
  cost_usd: number
}

export interface MttrPointDTO {
  date: string
  runs: number
  mttr_seconds: number
  fpvr_percent: number
}

export interface TelemetryTrendsDTO {
  window: TelemetryWindow | string
  bucket: string
  burn: TokenBurnPointDTO[]
  mttr: MttrPointDTO[]
}

// ---------------------------------------------------------------------------
// Agent Console (structured activity transcript) — .superpowers/console/contract.md
// ---------------------------------------------------------------------------

export type ConsoleEntryKind =
  | 'approval'
  | 'user'
  | 'assistant'
  | 'thinking'
  | 'tool_use'
  | 'tool_result'
  | 'request'
  | 'response'
  | 'system'
  | 'error'
  | 'state'

export type ConsoleEntryStatus = 'streaming' | 'done' | 'error' | 'cancelled'

export interface ConsoleToolInfo {
  id: string
  name: string
  input?: Record<string, any>
  is_error?: boolean
}

export interface ConsoleMessage {
  role: string
  content: string
}

export interface ConsoleRequestInfo {
  model: string
  method: string
  strategy: string
  fallback_chain?: string[]
  messages?: ConsoleMessage[]
  session_id?: string
  source?: 'chat' | 'execute' | string
}

export interface ConsoleUsageInfo {
  /** Model that actually answered. */
  model: string
  /** Router's pick, present only when it differs from `model`. */
  routed_model?: string
  provider: string
  prompt_tokens: number
  completion_tokens: number
  cached_tokens: number
  cost_usd: number
  duration_ms: number
  finish_reason: string
  session_id?: string
}

export interface ConsoleStateInfo {
  from: string
  to: string
  stage?: string
}

export interface ConsoleApprovalInfo {
  id: string
  tool_name: string
  summary: string
  rule_label?: string
  blocked_path?: string
  decision: 'pending' | 'allowed' | 'always' | 'all' | 'denied' | 'expired' | 'cancelled' | 'auto'
  message?: string
  expires_at: string
  decided_at?: string
}

export interface ConsoleEntry {
  id: string
  task_id: string
  turn_id?: string
  kind: ConsoleEntryKind
  content?: string
  status?: ConsoleEntryStatus
  tool?: ConsoleToolInfo
  request?: ConsoleRequestInfo
  usage?: ConsoleUsageInfo
  state?: ConsoleStateInfo
  approval?: ConsoleApprovalInfo
  created_at: string
  updated_at?: string
}

export interface TaskActivityResponse {
  entries: ConsoleEntry[]
  busy: boolean
  active_turn_id: string
  session_id: string
  model: string
}

export interface TaskChatResponse {
  turn_id: string
  entry_id: string
}

export interface ConsoleActivityDelta {
  id: string
  delta: string
  kind: 'assistant' | 'thinking'
}
