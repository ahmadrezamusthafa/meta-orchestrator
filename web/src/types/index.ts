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
