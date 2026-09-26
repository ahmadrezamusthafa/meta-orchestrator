<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useWorkflowStore } from '../stores/workflows'
import WorkflowCatalog from '../components/workflows/WorkflowCatalog.vue'
import WorkflowStageBuilder from '../components/workflows/WorkflowStageBuilder.vue'
import KanbanProjectionPreview from '../components/workflows/KanbanProjectionPreview.vue'
import BtnPrimary from '../components/common/BtnPrimary.vue'
import { GitFork, Plus, Trash2, Download, Check } from 'lucide-vue-next'

const workflowStore = useWorkflowStore()

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
}

function removeStage(index: number) {
  currentStages.value.splice(index, 1)
}

const isExporting = ref(false)
function exportYaml() {
  isExporting.value = true
  setTimeout(() => {
    isExporting.value = false
  }, 500)
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

      <BtnPrimary :loading="isExporting" @click="exportYaml">
        <Download class="w-3.5 h-3.5" />
        <span>Export to .sdlc/workflow.yaml</span>
      </BtnPrimary>
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
        @remove="removeStage"
        @update="currentStages = $event"
      />
    </main>
  </div>
</template>
