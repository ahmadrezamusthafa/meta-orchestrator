import type { Task } from '../types'

/**
 * Single source of truth for "what is this task doing and what can I do next".
 * Every surface (Kanban card, task header, console status bar) renders from this so they never
 * disagree. It is derived only from the task state the daemon reports.
 */

/** Default SDLC stage order (mirrors the backend stage controller). */
export const STAGES = [
  'prd_discovery', 'atdd_creation', 'techdoc_rfc', 'task_breakdown',
  'task_implementation', 'e2e_validation', 'uat_verification', 'signoff_merge',
] as const

const STAGE_NAMES: Record<string, string> = {
  prd_discovery: 'PRD Discovery',
  atdd_creation: 'ATDD Creation',
  techdoc_rfc: 'Technical RFC',
  task_breakdown: 'Task Breakdown',
  task_implementation: 'Implementation',
  e2e_validation: 'E2E Validation',
  uat_verification: 'UAT Verification',
  signoff_merge: 'Sign-off & Merge',
}

export function stageName(id?: string): string {
  if (!id) return 'Unknown stage'
  return STAGE_NAMES[id] || id.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

export function stagePosition(id?: string): { index: number; total: number } {
  return { index: STAGES.indexOf((id || '') as (typeof STAGES)[number]), total: STAGES.length }
}

export type ActionKind = 'run' | 'resume' | 'pause' | 'reset' | 'review' | 'assign' | 'approve'

/** Stages that change repository files; they cannot run without assigned repositories. */
export const CODE_STAGES = new Set(['atdd_creation', 'task_implementation', 'e2e_validation'])
export type Tone = 'active' | 'idle' | 'warn' | 'error' | 'done' | 'review' | 'waiting'

export interface TaskAction {
  kind: ActionKind
  label: string
  hint: string
}

export interface Lifecycle {
  tone: Tone
  /** Short status for badges: "Running", "Ready", "Needs review"… */
  status: string
  /** One-line sentence: what is happening right now. */
  title: string
  /** What the operator should know or do. */
  detail: string
  /** The single recommended next action, if any. */
  primary?: TaskAction
  /** True only while an agent is actually working. */
  isRunning: boolean
}

/**
 * busy: an agent turn is streaming. hasSession: the console holds an agent conversation that a run
 * can continue (only the console knows; other surfaces leave it unset).
 */
export function taskLifecycle(task: Task | null | undefined, opts: { busy?: boolean; hasSession?: boolean } = {}): Lifecycle | null {
  if (!task) return null
  const s = stageName(task.current_stage_id)
  const meta = task.metadata || {}
  const needsRepos = CODE_STAGES.has(task.current_stage_id) && !(task.assigned_repos || []).length
  if (needsRepos && ['PENDING', 'SUSPENDED', 'FAILED'].includes(task.state)) {
    return {
      tone: 'warn', status: 'Needs repos', isRunning: false,
      title: `${s} needs repositories`,
      detail: 'This stage changes code. Assign the repositories it may change, then run it.',
      primary: { kind: 'assign', label: 'Assign repos', hint: 'Choose the repositories for this task.' },
    }
  }
  const approvals = Number(meta.pending_approvals || 0)
  if (task.state === 'RUNNING' && approvals > 0) {
    return {
      tone: 'review', status: 'Needs approval', isRunning: true,
      title: `The agent is waiting for your permission${approvals > 1 ? ` (${approvals} requests)` : ''}`,
      detail: 'It paused on an action that needs approval. Review the request in the console.',
      primary: { kind: 'approve', label: 'Review request', hint: 'Open the console to allow or deny.' },
    }
  }
  switch (task.state) {
    case 'RUNNING':
      return {
        tone: 'active', status: 'Running', isRunning: true,
        title: opts.busy === false ? `Starting ${s}…` : `Agent is working on ${s}`,
        detail: 'Live activity streams in the console. Pause to stop the agent; you can resume later.',
        primary: { kind: 'pause', label: 'Pause', hint: 'Stop the agent now. Resume later to run this stage again.' },
      }
    case 'PENDING':
      if (opts.hasSession) {
        return {
          tone: 'idle', status: 'Ready', isRunning: false,
          title: `Ready to continue on ${s}`,
          detail: 'The agent keeps its conversation from this console. Continue it here, or clear the console first to start fresh.',
          primary: { kind: 'resume', label: 'Continue session', hint: `Continue the agent's conversation on ${s}, keeping what it already knows.` },
        }
      }
      return {
        tone: 'idle', status: 'Ready', isRunning: false,
        title: `Ready to run ${s}`,
        detail: 'Nothing is running yet. Start the stage, or ask the agent a question first.',
        primary: { kind: 'run', label: 'Run stage', hint: `Start the agent on ${s}.` },
      }
    case 'SUSPENDED':
      return {
        tone: 'warn', status: 'Paused', isRunning: false,
        title: `Paused at ${s}`,
        detail: 'Nothing is running. Resume to continue the agent\'s session where it stopped.',
        primary: { kind: 'resume', label: 'Resume', hint: `Continue ${s} where the agent stopped.` },
      }
    case 'FAILED':
      return {
        tone: 'error', status: 'Failed', isRunning: false,
        title: `${s} failed`,
        detail: meta.last_error || 'The last run ended with an error.',
        primary: { kind: 'resume', label: 'Continue', hint: `Continue ${s} in the same agent session, without starting over.` },
      }
    case 'BLOCKED_FRUSTRATION':
      return {
        tone: 'error', status: 'Blocked', isRunning: false,
        title: `Blocked at ${s} after repeated failures`,
        detail: 'Give the agent guidance, or reset the failure counter and retry.',
        primary: { kind: 'reset', label: 'Reset & retry', hint: 'Clear failure counters and run the stage again.' },
      }
    case 'WAITING_DEPENDENCY': {
      const deps = meta.unmet_dependencies || (task.dependencies || []).join(', ')
      return {
        tone: 'waiting', status: 'Waiting', isRunning: false,
        title: `Waiting for ${deps || 'prerequisite tasks'}`,
        detail: 'Starts automatically when its prerequisites are completed.',
      }
    }
    case 'WAITING_GATE_APPROVAL':
      return {
        tone: 'review', status: 'Needs review', isRunning: false,
        title: `${s} is ready for your review`,
        detail: 'Read the output, then approve to continue or reject with feedback.',
        primary: { kind: 'review', label: 'Review', hint: 'Open the task to approve or request changes.' },
      }
    case 'COMPLETED':
      return { tone: 'done', status: 'Completed', isRunning: false, title: 'Completed', detail: 'All stages were approved.' }
    default:
      return { tone: 'idle', status: task.state, isRunning: false, title: task.state, detail: '' }
  }
}

export const TONE_BADGE: Record<Tone, string> = {
  active: 'bg-sky-950/70 border-sky-700/70 text-sky-300',
  idle: 'bg-slate-800 border-slate-700 text-slate-300',
  warn: 'bg-amber-950/70 border-amber-700/70 text-amber-300',
  error: 'bg-rose-950/70 border-rose-700/70 text-rose-300',
  done: 'bg-emerald-950/70 border-emerald-700/70 text-emerald-300',
  review: 'bg-violet-950/70 border-violet-700/70 text-violet-300',
  waiting: 'bg-orange-950/60 border-orange-800/70 text-orange-300',
}

export const ACTION_BUTTON: Record<ActionKind, string> = {
  run: 'bg-emerald-600 hover:bg-emerald-500 text-white border-emerald-500',
  resume: 'bg-emerald-600 hover:bg-emerald-500 text-white border-emerald-500',
  pause: 'bg-amber-950/80 hover:bg-amber-900 text-amber-200 border-amber-700/80',
  reset: 'bg-rose-950/80 hover:bg-rose-900 text-rose-200 border-rose-700/80',
  review: 'bg-violet-600 hover:bg-violet-500 text-white border-violet-500',
  assign: 'bg-amber-600 hover:bg-amber-500 text-white border-amber-500',
  approve: 'bg-amber-600 hover:bg-amber-500 text-white border-amber-500 animate-pulse',
}
