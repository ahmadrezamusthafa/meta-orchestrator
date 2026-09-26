import type { Task, WorkflowStage } from '../types'

export interface StageGuidance {
  stageNumber: number
  totalStages: number
  stageName: string
  nextStageName: string
  statusDescription: string
  nextStepDescription: string
  suggestion: string
  actionType: 'gate_approval' | 'blocked_steer' | 'autonomous_observe' | 'completed_review'
  isGateRequired: boolean
  isWriteLocked: boolean
}

const defaultStages: Array<{ id: string; name: string }> = [
  { id: 'prd_discovery', name: 'PRD & Dynamic Repo Discovery' },
  { id: 'atdd_creation', name: 'ATDD Creation (Red Phase)' },
  { id: 'techdoc_rfc', name: 'Tech Doc / RFC Review (Gate)' },
  { id: 'task_breakdown', name: 'Task Breakdown & Planning' },
  { id: 'task_implementation', name: 'Implementation (Write-Unlocked)' },
  { id: 'e2e_validation', name: 'Automation & E2E Validation' },
  { id: 'uat_verification', name: 'Manual & UAT Verification' },
  { id: 'signoff_merge', name: 'Ready for Sign-Off & Merge' },
]

export function getStageGuidance(task: Task | null | undefined, customStages?: WorkflowStage[]): StageGuidance {
  const stages = (customStages && customStages.length > 0) ? customStages : defaultStages
  
  if (!task) {
    return {
      stageNumber: 1,
      totalStages: stages.length,
      stageName: 'Loading Stage...',
      nextStageName: 'Unknown',
      statusDescription: 'Fetching task telemetry from daemon...',
      nextStepDescription: 'Synchronizing FSM state...',
      suggestion: 'Please wait while task details load.',
      actionType: 'autonomous_observe',
      isGateRequired: false,
      isWriteLocked: false,
    }
  }

  const currentIdx = stages.findIndex((s) => s.id === task.current_stage_id)
  const safeIdx = currentIdx !== -1 ? currentIdx : 0
  const stage = stages[safeIdx]
  const nextStage = safeIdx < stages.length - 1 ? stages[safeIdx + 1] : null

  const isWriteLocked = task.current_stage_id === 'atdd_creation' || task.metadata?.write_lock === 'ACTIVE'
  const isBlocked = task.state === 'BLOCKED_FRUSTRATION'
  const isGate = task.state === 'WAITING_GATE_APPROVAL'
  const isCompleted = task.state === 'COMPLETED'

  // Handling special task states first
  if (isBlocked) {
    return {
      stageNumber: safeIdx + 1,
      totalStages: stages.length,
      stageName: stage.name,
      nextStageName: 'Agent Recovery & Re-execution',
      statusDescription: `⚠️ Circuit Breaker Tripped: Subagent halted after repeated failures in ${stage.name}.`,
      nextStepDescription: 'Operator injection required to break error loop and resume execution.',
      suggestion: 'Click "Steer Agent" with a hint (e.g. "Use mock Stripe credentials") or click "Reset Workspace" to purge container volumes.',
      actionType: 'blocked_steer',
      isGateRequired: false,
      isWriteLocked,
    }
  }

  if (isGate) {
    return {
      stageNumber: safeIdx + 1,
      totalStages: stages.length,
      stageName: stage.name,
      nextStageName: nextStage ? nextStage.name : 'Sign-Off & Merge',
      statusDescription: `🛡️ Review Gate Active: Pipeline paused at ${stage.name} awaiting human approval.`,
      nextStepDescription: nextStage ? `On approval, pipeline advances to ${nextStage.name}.` : 'On approval, changes will be merged.',
      suggestion: 'Review generated RFC or Evidence in the SDLC Artifacts tab, then click "Approve Gate" to proceed or "Reject" with feedback.',
      actionType: 'gate_approval',
      isGateRequired: true,
      isWriteLocked,
    }
  }

  if (isCompleted) {
    return {
      stageNumber: stages.length,
      totalStages: stages.length,
      stageName: 'Pipeline Completed',
      nextStageName: 'Production Ready (Merged)',
      statusDescription: '✅ All acceptance criteria, ATDD specifications, and E2E regression tests verified.',
      nextStepDescription: 'Feature is packaged with HMAC evidence and ready for production deployment.',
      suggestion: 'Execution complete. You can download final artifacts or inspect test videos in the Evidence Vault.',
      actionType: 'completed_review',
      isGateRequired: false,
      isWriteLocked: false,
    }
  }

  // Stage-by-stage descriptions
  switch (task.current_stage_id) {
    case 'prd_discovery':
      return {
        stageNumber: 1,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage?.name || 'ATDD Creation',
        statusDescription: `Analyzing feature spec for repos [${(task.assigned_repos || []).join(', ')}] and generating PRD.md.`,
        nextStepDescription: `Next: ${nextStage?.name || 'ATDD Creation'} — QA agent will author failing test specs.`,
        suggestion: 'Autonomous: The system is indexing repository ASTs. No manual action required.',
        actionType: 'autonomous_observe',
        isGateRequired: false,
        isWriteLocked: true,
      }

    case 'atdd_creation':
      return {
        stageNumber: 2,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage?.name || 'Tech Doc / RFC Review',
        statusDescription: 'QA Engineer is synthesizing Playwright & unit test suites. Source code is write-locked to prove test failure (Red Phase).',
        nextStepDescription: `Next: ${nextStage?.name || 'Tech Doc / RFC Review'} — Architect will draft technical RFC.`,
        suggestion: 'Write-Locked: Code changes are prevented until red phase verifies that tests fail for the right reasons.',
        actionType: 'autonomous_observe',
        isGateRequired: false,
        isWriteLocked: true,
      }

    case 'techdoc_rfc':
      return {
        stageNumber: 3,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage?.name || 'Task Breakdown & Planning',
        statusDescription: 'System Architect generated TECH_DOC_RFC.md with database schema and API contracts.',
        nextStepDescription: `Next: ${nextStage?.name || 'Task Breakdown'} — Subtasks will be scheduled for parallel execution.`,
        suggestion: 'Review Gate: Please inspect TECH_DOC_RFC.md in the right pane and approve to unlock developer implementation.',
        actionType: 'gate_approval',
        isGateRequired: true,
        isWriteLocked: true,
      }

    case 'task_breakdown':
      return {
        stageNumber: 4,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage?.name || 'Implementation',
        statusDescription: 'Planner agent is partitioning RFC implementation into atomic subtasks for frontend and backend roles.',
        nextStepDescription: `Next: ${nextStage?.name || 'Implementation'} — Developer agents begin writing source code.`,
        suggestion: 'Autonomous: Mapping dependency graph between repositories. Work will begin shortly.',
        actionType: 'autonomous_observe',
        isGateRequired: false,
        isWriteLocked: true,
      }

    case 'task_implementation':
      return {
        stageNumber: 5,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage?.name || 'Automation & E2E Validation',
        statusDescription: `Write-locks disengaged. Subagents are modifying source code in [${(task.assigned_repos || []).join(', ')}] using ${task.selected_method} methodology.`,
        nextStepDescription: `Next: ${nextStage?.name || 'Automation & E2E Validation'} — Running full Playwright suite.`,
        suggestion: 'Active Coding: Developers are editing code. Watch the Live Terminal or Thought Stream for real-time progress.',
        actionType: 'autonomous_observe',
        isGateRequired: false,
        isWriteLocked: false,
      }

    case 'e2e_validation':
      return {
        stageNumber: 6,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage?.name || 'Manual & UAT Verification',
        statusDescription: 'Running Playwright browser automation and regression test suites inside isolated Docker sandbox.',
        nextStepDescription: `Next: ${nextStage?.name || 'Manual & UAT Verification'} — Evidence capture & HMAC signing.`,
        suggestion: 'Validating: Tests are running headlessly. Video recording (.mp4) is active.',
        actionType: 'autonomous_observe',
        isGateRequired: false,
        isWriteLocked: false,
      }

    case 'uat_verification':
      return {
        stageNumber: 7,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage?.name || 'Ready for Sign-Off & Merge',
        statusDescription: 'Synthesizing EVIDENCE.md, collecting viewport diff screenshots, and packaging test audit trail.',
        nextStepDescription: `Next: ${nextStage?.name || 'Sign-Off & Merge'} — Final release gate before git push.`,
        suggestion: 'Evidence Ready: Check the Evidence tab on the right to inspect recorded test video and screenshots.',
        actionType: 'autonomous_observe',
        isGateRequired: false,
        isWriteLocked: false,
      }

    case 'signoff_merge':
      return {
        stageNumber: 8,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: 'Pipeline Complete',
        statusDescription: 'All automated gates passed with 100% test pass rate. Awaiting Release Manager merge approval.',
        nextStepDescription: 'On sign-off, coordinated git merge will commit and push changes across all target repositories.',
        suggestion: 'Final Release Gate: Click "Approve Gate" to authorize the merge and complete the task.',
        actionType: 'gate_approval',
        isGateRequired: true,
        isWriteLocked: false,
      }

    default:
      return {
        stageNumber: safeIdx + 1,
        totalStages: stages.length,
        stageName: stage.name,
        nextStageName: nextStage ? nextStage.name : 'Next Stage',
        statusDescription: `Executing stage ${stage.name} under ${task.selected_method} methodology.`,
        nextStepDescription: nextStage ? `Next: ${nextStage.name}` : 'Next: Pipeline completion',
        suggestion: 'Autonomous stage running. Monitor logs or inject context if needed.',
        actionType: 'autonomous_observe',
        isGateRequired: 'requires_gate' in stage ? Boolean((stage as WorkflowStage).requires_gate) : false,
        isWriteLocked,
      }
  }
}
