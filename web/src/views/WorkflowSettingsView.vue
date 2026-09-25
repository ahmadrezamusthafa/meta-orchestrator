<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useWorkflowStore } from '../stores/workflows'
import WorkflowCatalog from '../components/workflows/WorkflowCatalog.vue'
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
      <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-slate-800">
          <div>
            <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
              Sequential Stage Pipeline Designer
            </h3>
            <span class="text-[11px] text-slate-400">
              Configure write-locking rules, allowed methods, and approval gates per stage
            </span>
          </div>

          <button
            @click="addStage"
            type="button"
            class="h-7 px-2.5 rounded bg-emerald-950 border border-emerald-800 text-emerald-300 hover:bg-emerald-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
          >
            <Plus class="w-3.5 h-3.5" />
            <span>Add Pipeline Stage</span>
          </button>
        </div>

        <div class="space-y-3">
          <div
            v-for="(st, idx) in currentStages"
            :key="st.id || idx"
            class="p-3 bg-slate-950 border border-slate-800 rounded-lg flex items-center justify-between gap-4 text-xs font-mono"
          >
            <div class="flex items-center gap-3">
              <span class="text-slate-500 font-bold">#{{ idx + 1 }}</span>
              <input
                v-model="st.name"
                class="h-7 px-2 bg-slate-900 border border-slate-700 rounded text-slate-200 text-xs font-sans font-medium focus:outline-none focus:border-emerald-500 w-56"
              />
            </div>

            <!-- Role & Method Selection -->
            <div class="flex items-center gap-3">
              <select
                v-model="st.assigned_role"
                class="h-7 px-2 bg-slate-900 border border-slate-700 rounded text-slate-300 text-[11px] focus:outline-none"
              >
                <option value="architect">Role: Architect</option>
                <option value="developer">Role: Developer</option>
                <option value="qa_engineer">Role: QA Engineer</option>
                <option value="product_manager">Role: Product Manager</option>
              </select>

              <select
                v-model="st.allowed_methods[0]"
                class="h-7 px-2 bg-slate-900 border border-slate-700 rounded text-slate-300 text-[11px] focus:outline-none"
              >
                <option value="BMAD">BMAD</option>
                <option value="Supervisor">Supervisor</option>
                <option value="ReAct">ReAct</option>
                <option value="Superpower">Superpower</option>
              </select>

              <!-- Write Lock Checkbox -->
              <label class="flex items-center gap-1.5 cursor-pointer text-[11px] text-amber-300">
                <input
                  type="checkbox"
                  v-model="st.write_lock_workspace"
                  class="rounded bg-slate-900 border-slate-700 text-amber-500"
                />
                <span>Write-Locked</span>
              </label>

              <!-- Human Gate Checkbox -->
              <label class="flex items-center gap-1.5 cursor-pointer text-[11px] text-rose-300">
                <input
                  type="checkbox"
                  v-model="st.requires_gate"
                  class="rounded bg-slate-900 border-slate-700 text-rose-500"
                />
                <span>Gate Required</span>
              </label>
            </div>

            <button
              @click="removeStage(idx)"
              class="p-1 text-slate-500 hover:text-rose-400 transition-colors"
            >
              <Trash2 class="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>
