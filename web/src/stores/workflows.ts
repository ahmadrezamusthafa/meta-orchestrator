import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { WorkflowDefinition, WorkflowStage } from '../types'
import { api } from '../services/api'

export const useWorkflowStore = defineStore('workflows', () => {
  const workflows = ref<WorkflowDefinition[]>([])
  const activeWorkflowId = ref('general_ai_sdlc')
  const isLoading = ref(false)

  const activeWorkflow = computed(() => {
    return workflows.value.find((w) => w.id === activeWorkflowId.value) || workflows.value[0] || null
  })

  // Dynamic columns projected from the active workflow
  const projectedColumns = computed<WorkflowStage[]>(() => {
    if (activeWorkflow.value && activeWorkflow.value.stages.length > 0) {
      return activeWorkflow.value.stages
    }
    // Default 8-stage General AI SDLC fallback
    return [
      { id: 'prd_discovery', name: 'PRD & Dynamic Repo Discovery', type: 'automated', assigned_role: 'product_manager', allowed_methods: ['BMAD', 'Supervisor'], write_lock_workspace: true, requires_gate: false },
      { id: 'atdd_creation', name: 'ATDD Creation (Red Phase)', type: 'automated', assigned_role: 'qa_engineer', allowed_methods: ['Supervisor', 'BMAD'], write_lock_workspace: true, requires_gate: false },
      { id: 'techdoc_rfc', name: 'Tech Doc / RFC Review (Gate)', type: 'review_gate', assigned_role: 'architect', allowed_methods: ['BMAD'], write_lock_workspace: true, requires_gate: true },
      { id: 'task_breakdown', name: 'Task Breakdown & Planning', type: 'automated', assigned_role: 'planner', allowed_methods: ['Supervisor'], write_lock_workspace: true, requires_gate: false },
      { id: 'task_implementation', name: 'Implementation (Write-Unlocked)', type: 'automated', assigned_role: 'developer', allowed_methods: ['BMAD', 'ReAct', 'Supervisor', 'Superpower'], write_lock_workspace: false, requires_gate: false },
      { id: 'e2e_validation', name: 'Automation & E2E Validation', type: 'automated', assigned_role: 'qa_engineer', allowed_methods: ['Supervisor', 'Superpower'], write_lock_workspace: false, requires_gate: false },
      { id: 'uat_verification', name: 'Manual & UAT Verification', type: 'manual_verification', assigned_role: 'product_manager', allowed_methods: ['Supervisor'], write_lock_workspace: false, requires_gate: false },
      { id: 'signoff_merge', name: 'Ready for Sign-Off & Merge', type: 'review_gate', assigned_role: 'release_manager', allowed_methods: ['BMAD'], write_lock_workspace: false, requires_gate: true },
    ]
  })

  async function fetchWorkflows() {
    isLoading.value = true
    try {
      workflows.value = await api.getWorkflows()
    } finally {
      isLoading.value = false
    }
  }

  async function createWorkflow(wf: WorkflowDefinition) {
    const res = await api.createWorkflow(wf)
    await fetchWorkflows()
    return res
  }

  return {
    workflows,
    activeWorkflowId,
    activeWorkflow,
    projectedColumns,
    isLoading,
    fetchWorkflows,
    createWorkflow,
  }
})
