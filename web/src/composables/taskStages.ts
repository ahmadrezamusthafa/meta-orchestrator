import { computed, type ComputedRef } from 'vue'
import type { Task, WorkflowDefinition } from '../types'
import { useWorkflowStore } from '../stores/workflows'
import { STAGES } from './taskLifecycle'

/**
 * The stages a task runs: its workflow's (the default pipeline when the workflow is unknown or not
 * loaded yet), cut to its slice — from the start stage to the halt stage. Mirrors the daemon's
 * taskStages, so progress ("stage 2 of 7") matches what will actually run.
 */
export function stagesOfTask(task: Pick<Task, 'workflow_id' | 'active_slice'> | null | undefined, workflows: WorkflowDefinition[]): string[] {
  const wf = workflows.find((w) => w.id === (task?.workflow_id || 'general_ai_sdlc'))
  const all = wf?.stages?.length ? wf.stages.map((s) => s.id) : [...STAGES]
  const slice = task?.active_slice
  const from = slice?.start_stage_id ? Math.max(0, all.indexOf(slice.start_stage_id)) : 0
  const halt = slice?.halt_stage_id ? all.indexOf(slice.halt_stage_id) : -1
  return all.slice(from, halt >= from ? halt + 1 : all.length)
}

/** The task's stages and the position of its current stage (-1 when it is not among them). */
export function useTaskStages(task: () => Task | null | undefined): { stages: ComputedRef<string[]>; index: ComputedRef<number> } {
  const workflowStore = useWorkflowStore()
  if (!workflowStore.workflows.length && !workflowStore.isLoading) workflowStore.fetchWorkflows()
  const stages = computed(() => stagesOfTask(task(), workflowStore.workflows))
  const index = computed(() => stages.value.indexOf(task()?.current_stage_id || ''))
  return { stages, index }
}
