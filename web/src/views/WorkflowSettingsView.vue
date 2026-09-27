<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useWorkflowStore } from '../stores/workflows'
import { useToastStore } from '../stores/toast'
import WorkflowCatalog from '../components/workflows/WorkflowCatalog.vue'
import WorkflowStageBuilder from '../components/workflows/WorkflowStageBuilder.vue'
import KanbanProjectionPreview from '../components/workflows/KanbanProjectionPreview.vue'
import BtnPrimary from '../components/common/BtnPrimary.vue'
import ConfirmDeleteModal from '../components/common/ConfirmDeleteModal.vue'
import { GitFork, Plus, Trash2, Download, Save, Check } from 'lucide-vue-next'

const workflowStore = useWorkflowStore()
const toastStore = useToastStore()

const currentStages = ref<any[]>([])

onMounted(async () => {
  await workflowStore.fetchWorkflows()
  if (workflowStore.activeWorkflow) {
    currentStages.value = JSON.parse(JSON.stringify(workflowStore.activeWorkflow.stages))
  }
})

function onSelectWorkflow(id: string) {
  workflowStore.activeWorkflowId = id
  if (workflowStore.activeWorkflow) {
    currentStages.value = JSON.parse(JSON.stringify(workflowStore.activeWorkflow.stages))
  }
}

function addStage() {
  const num = currentStages.value.length + 1
  currentStages.value.push({
    id: `custom_stage_${num}`,
    name: `Custom Stage ${num}`,
    type: 'automated',
    assigned_role: 'developer',
    allowed_methods: ['BMAD'],
    write_lock_workspace: false,
    requires_gate: false,
  })
  toastStore.info(`Added Stage: Custom Stage ${num}`)
}

function removeStage(index: number) {
  const removed = currentStages.value.splice(index, 1)
  toastStore.warning(`Removed Stage: ${removed[0]?.name || 'Stage'}`)
}

// Confirmation modal state
const isConfirmDeleteOpen = ref(false)
const confirmDeleteTitle = ref('Confirm Deletion')
const confirmDeleteMessage = ref('Are you sure you want to proceed?')
const confirmDeleteItemName = ref('')
const confirmDeleteNote = ref('')
const confirmDeleteBtnText = ref('Delete')
const isConfirmDeleteLoading = ref(false)
let onConfirmDeleteCallback: (() => Promise<void>) | null = null

function triggerConfirmDelete(options: {
  title: string
  message: string
  itemName: string
  note?: string
  confirmText?: string
  onConfirm: () => Promise<void>
}) {
  confirmDeleteTitle.value = options.title
  confirmDeleteMessage.value = options.message
  confirmDeleteItemName.value = options.itemName
  confirmDeleteNote.value = options.note || ''
  confirmDeleteBtnText.value = options.confirmText || 'Delete'
  onConfirmDeleteCallback = options.onConfirm
  isConfirmDeleteOpen.value = true
}

async function handleExecuteConfirmDelete() {
  if (!onConfirmDeleteCallback) return
  isConfirmDeleteLoading.value = true
  try {
    await onConfirmDeleteCallback()
    isConfirmDeleteOpen.value = false
  } catch (err: any) {
    // Handled in callback
  } finally {
    isConfirmDeleteLoading.value = false
  }
}

function promptRemoveStage(index: number) {
  const stage = currentStages.value[index]
  const stageName = stage?.name || `Stage ${index + 1}`
  triggerConfirmDelete({
    title: 'Remove Workflow Stage',
    message: `Are you sure you want to remove stage "${stageName}" from this workflow?`,
    itemName: stageName,
    note: 'The stage will be removed from the pipeline. Click "Save Workflow" to persist.',
    confirmText: 'Remove Stage',
    onConfirm: async () => {
      removeStage(index)
    }
  })
}

const isSaving = ref(false)
async function saveWorkflowChanges() {
  if (!workflowStore.activeWorkflow) return
  isSaving.value = true
  try {
    const updated = {
      ...workflowStore.activeWorkflow,
      stages: currentStages.value,
    }
    await workflowStore.createWorkflow(updated)
    toastStore.success('Workflow Saved', `${updated.name} updated with ${currentStages.value.length} stages`)
  } catch (err: any) {
    toastStore.error('Save Failed', err?.message || 'Server error')
  } finally {
    isSaving.value = false
  }
}

const isExporting = ref(false)
function exportYaml() {
  isExporting.value = true
  try {
    const wf = workflowStore.activeWorkflow || {
      id: 'custom-sdlc',
      name: 'Custom SDLC Pipeline',
      description: 'Zero-trust software factory pipeline',
      version: '1.0.0',
    }

    let yaml = `# Declarative Meta-Orchestrator SDLC Pipeline Schema\n# Generated: ${new Date().toISOString()}\n\n`
    yaml += `workflow:\n`
    yaml += `  id: "${wf.id}"\n`
    yaml += `  name: "${wf.name}"\n`
    yaml += `  description: "${wf.description}"\n`
    yaml += `  version: "${wf.version || '1.0.0'}"\n`
    yaml += `  stages:\n`

    currentStages.value.forEach((stage) => {
      yaml += `    - id: "${stage.id}"\n`
      yaml += `      name: "${stage.name}"\n`
      yaml += `      method: "${stage.allowed_methods?.[0] || 'BMAD'}"\n`
      yaml += `      roles: ["${stage.assigned_role}"]\n`
      if (stage.write_lock_workspace) {
        yaml += `      write_locked: true\n`
      }
      yaml += `      gates:\n`
      if (stage.requires_gate) {
        yaml += `        human_approval: true\n`
      } else {
        yaml += `        auto_verify: true\n`
      }
    })

    const blob = new Blob([yaml], { type: 'text/yaml;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'workflow.yaml'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)

    toastStore.success('Workflow Exported', 'Saved to .sdlc/workflow.yaml')
  } catch (err) {
    toastStore.error('Export Failed')
  } finally {
    setTimeout(() => {
      isExporting.value = false
    }, 400)
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Header -->
    <div class="h-14 px-6 bg-slate-900/50 border-b border-slate-800 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <GitFork class="w-4 h-4 text-emerald-400" />
        <h2 class="text-xs font-semibold text-slate-100 uppercase tracking-wide">
          Custom SDLC Workflow Builder & Manager
        </h2>
      </div>

      <div class="flex items-center gap-2.5">
        <button
          @click="saveWorkflowChanges"
          :disabled="isSaving"
          type="button"
          class="h-8 px-3 rounded-lg bg-slate-900 border border-slate-800 hover:border-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-1.5 transition-colors"
        >
          <Save class="w-3.5 h-3.5 text-emerald-400" />
          <span>{{ isSaving ? 'Saving...' : 'Save Workflow' }}</span>
        </button>

        <BtnPrimary :loading="isExporting" @click="exportYaml">
          <Download class="w-3.5 h-3.5" />
          <span>Export .sdlc/workflow.yaml</span>
        </BtnPrimary>
      </div>
    </div>

    <!-- Content -->
    <main class="flex-1 p-6 overflow-y-auto space-y-6">
      <!-- 1. Catalog -->
      <WorkflowCatalog
        :workflows="workflowStore.workflows"
        :active-id="workflowStore.activeWorkflowId"
        @select="onSelectWorkflow"
      />

      <!-- 2. Live Kanban Projection Preview -->
      <KanbanProjectionPreview :stages="currentStages" />

      <!-- 3. Stage Sequence Builder -->
      <WorkflowStageBuilder
        :stages="currentStages"
        @add="addStage"
        @remove="promptRemoveStage"
        @update="currentStages = $event"
      />
    </main>

    <!-- Confirmation Modal -->
    <ConfirmDeleteModal
      :is-open="isConfirmDeleteOpen"
      :title="confirmDeleteTitle"
      :message="confirmDeleteMessage"
      :item-name="confirmDeleteItemName"
      :note="confirmDeleteNote"
      :confirm-text="confirmDeleteBtnText"
      :loading="isConfirmDeleteLoading"
      @confirm="handleExecuteConfirmDelete"
      @close="isConfirmDeleteOpen = false"
    />
  </div>
</template>

