<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useTaskStore } from '../../stores/tasks'
import { useWorkflowStore } from '../../stores/workflows'
import { useProjectStore } from '../../stores/projects'
import { useToastStore } from '../../stores/toast'
import BtnPrimary from '../common/BtnPrimary.vue'
import StageRangeSelector from './StageRangeSelector.vue'
import ArtifactUploadDropzone from './ArtifactUploadDropzone.vue'
import { X, Sparkles, Layers, GitFork, Zap, FolderGit2, Link2, GitBranch } from 'lucide-vue-next'

const props = defineProps<{
  initialStageId?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created', task: any): void
}>()

const taskStore = useTaskStore()
const workflowStore = useWorkflowStore()
const projectStore = useProjectStore()
const toastStore = useToastStore()

const title = ref('')
const description = ref('')
const selectedProjectId = ref(projectStore.activeProjectId || 'proj-core-platform')
const selectedWorkflowId = ref(workflowStore.activeWorkflowId || 'general_ai_sdlc')
const selectedRepos = ref<string[]>([])
const selectedDependencies = ref<string[]>([])
const useWorktree = ref(true)
const executionScope = ref<'full' | 'slice'>('full')
const startStage = ref(props.initialStageId || 'task_implementation')
const haltStage = ref('e2e_validation')
const produceVideo = ref(true)
const sourceBranch = ref('')
const routerStrategy = ref('BEST_PRACTICE')
const selectedMethod = ref('Auto')
const isSubmitting = ref(false)
const externalPlanContent = ref('')
const externalPlanName = ref('')

const activeProject = computed(() => {
  return projectStore.projects.find(p => p.id === selectedProjectId.value) || projectStore.activeProject
})

const availableRepos = computed(() => {
  if (activeProject.value?.repos && activeProject.value.repos.length > 0) {
    return activeProject.value.repos
  }
  return [
    { name: 'frontend-portal', role: 'frontend' },
    { name: 'backend-core', role: 'backend' },
    { name: 'api-contracts', role: 'contracts' }
  ]
})

watch(availableRepos, (repos) => {
  if (repos.length > 0 && selectedRepos.value.length === 0) {
    selectedRepos.value = repos.map(r => r.name)
  }
}, { immediate: true })

watch(selectedProjectId, () => {
  if (availableRepos.value.length > 0) {
    selectedRepos.value = availableRepos.value.map(r => r.name)
  }
})

function handleFileSelected(file: File) {
  externalPlanName.value = file.name
  const reader = new FileReader()
  reader.onload = (e) => {
    externalPlanContent.value = (e.target?.result as string) || ''
    toastStore.info('Prerequisite Ingested', `Loaded ${file.name} into slice pipeline`)
  }
  reader.readAsText(file)
}

const allRepos = ['frontend-portal', 'backend-core', 'api-contracts']

const presets = [
  {
    name: '⚡ Stripe Payment Gateway',
    title: 'Implement Stripe Checkout & Webhook Idempotency',
    desc: 'Integrate Stripe SDK in frontend-portal and webhook idempotency table in backend-core with Redis distributed locking.',
    repos: ['frontend-portal', 'backend-core'],
    scope: 'full' as const,
    strategy: 'BEST_PRACTICE',
    method: 'Auto',
  },
  {
    name: '🐛 Hotfix JWT Expiry Race',
    title: 'Hotfix: Refresh Token Expiry Race Condition',
    desc: 'Address token refresh race condition on concurrent HTTP requests causing premature 401 unauthorized errors.',
    repos: ['frontend-portal'],
    scope: 'slice' as const,
    startStage: 'task_implementation',
    haltStage: 'e2e_validation',
    strategy: 'BEST_PRACTICE',
    method: 'ReAct',
  },
  {
    name: '📦 Inventory Schema Contracts',
    title: 'Design & Implement Inventory Reservation Schema',
    desc: 'Contract-first OpenAPI & protobuf schemas for warehouse inventory sync across frontend and backend services.',
    repos: ['frontend-portal', 'backend-core', 'api-contracts'],
    scope: 'full' as const,
    strategy: 'RULE_BASED',
    method: 'Supervisor',
  },
]

function applyPreset(p: typeof presets[0]) {
  title.value = p.title
  description.value = p.desc
  selectedRepos.value = [...p.repos]
  executionScope.value = p.scope
  if (p.startStage) startStage.value = p.startStage
  if (p.haltStage) haltStage.value = p.haltStage
  routerStrategy.value = p.strategy
  selectedMethod.value = p.method
  toastStore.info(`Loaded preset: ${p.name}`)
}

const selectedWorkflow = computed(() => {
  return workflowStore.workflows.find(w => w.id === selectedWorkflowId.value) || workflowStore.activeWorkflow
})

const stagesForSelector = computed(() => {
  if (selectedWorkflow.value?.stages && selectedWorkflow.value.stages.length > 0) {
    return selectedWorkflow.value.stages.map((s, idx) => ({
      id: s.id,
      name: `Stage ${idx + 1}: ${s.name}`,
    }))
  }
  return []
})

watch(stagesForSelector, (stages) => {
  if (stages.length > 0) {
    if (props.initialStageId && stages.some(s => s.id === props.initialStageId)) {
      startStage.value = props.initialStageId
    } else if (!stages.some(s => s.id === startStage.value)) {
      startStage.value = stages[0].id
    }
    if (!stages.some(s => s.id === haltStage.value)) {
      haltStage.value = stages[stages.length - 1].id
    }
  }
}, { immediate: true })

const availableDependencyTasks = computed(() => {
  return taskStore.tasks.map(t => ({
    id: t.id,
    title: t.title,
    state: t.state,
    stage: t.current_stage_id,
  }))
})

function toggleDependency(taskId: string) {
  const idx = selectedDependencies.value.indexOf(taskId)
  if (idx >= 0) {
    selectedDependencies.value.splice(idx, 1)
  } else {
    selectedDependencies.value.push(taskId)
  }
}

async function handleSubmit() {
  if (!title.value.trim()) return

  isSubmitting.value = true
  try {
    const payload: any = {
      title: title.value,
      description: description.value,
      workflow_id: selectedWorkflowId.value,
      assigned_repos: selectedRepos.value,
      router_strategy: routerStrategy.value,
      selected_method: selectedMethod.value === 'Auto' ? 'BMAD' : selectedMethod.value,
      complexity: 'HIGH',
      max_token_budget: 50000,
      dependencies: selectedDependencies.value,
      use_worktree: useWorktree.value,
    }

    if (executionScope.value === 'slice') {
      payload.active_slice = {
        start_stage_id: startStage.value,
        halt_stage_id: haltStage.value,
        produce_video: produceVideo.value,
      }
      payload.source_branch = sourceBranch.value
      if (externalPlanContent.value) {
        if (externalPlanName.value.toLowerCase().includes('prd')) {
          payload.external_prd = externalPlanContent.value
        } else {
          payload.external_task_plan = externalPlanContent.value
        }
      }
    }

    const created = await taskStore.createTask(payload)
    toastStore.success(`Task Launched`, `${created.id} initialized into factory pipeline`)
    emit('created', created)
    emit('close')
  } catch (err: any) {
    toastStore.error(`Failed to launch task`, err?.message || 'Server error')
    console.error('Failed to create task:', err)
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl flex flex-col max-h-[90vh] overflow-hidden">
      <div class="px-6 py-4 border-b border-slate-800 flex items-center justify-between bg-slate-900">
        <div class="flex items-center gap-2">
          <Sparkles class="w-5 h-5 text-emerald-400" />
          <h2 class="text-sm font-semibold text-slate-100 uppercase tracking-wide">
            Launch Autonomous SDLC Task
          </h2>
        </div>
        <button
          @click="$emit('close')"
          type="button"
          class="text-slate-400 hover:text-slate-200 transition-colors"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="p-6 overflow-y-auto space-y-4">
        <!-- Quick Presets -->
        <div>
          <div class="flex items-center gap-1.5 text-xs font-medium text-slate-400 mb-2">
            <Zap class="w-3.5 h-3.5 text-amber-400" />
            <span>Quick Start Demo Presets:</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="p in presets"
              :key="p.name"
              @click="applyPreset(p)"
              type="button"
              class="px-2.5 py-1 rounded-md bg-slate-950 border border-slate-800 hover:border-emerald-700 hover:text-emerald-300 text-xs font-mono text-slate-300 transition-colors text-left"
            >
              {{ p.name }}
            </button>
          </div>
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-300 mb-1.5">Task Title / Feature Objective *</label>
          <input
            v-model="title"
            type="text"
            placeholder="e.g. Implement Stripe payment gateway & webhook idempotency"
            class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:border-emerald-500"
          />
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-300 mb-1.5">Specification / Ticket Body</label>
          <textarea
            v-model="description"
            rows="3"
            placeholder="Describe business criteria, API endpoints, or user journey..."
            class="w-full p-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:border-emerald-500 font-sans"
          ></textarea>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-300 mb-1.5 flex items-center justify-between">
            <span class="flex items-center gap-1.5">
              <FolderGit2 class="w-3.5 h-3.5 text-emerald-400" />
              Target Project Workspace
            </span>
            <router-link to="/projects" class="text-[11px] text-emerald-400 hover:underline">
              Manage Multi-Repos →
            </router-link>
          </label>
          <select
            v-model="selectedProjectId"
            class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500 font-mono"
          >
            <option v-for="p in projectStore.projects" :key="p.id" :value="p.id">
              {{ p.name }} ({{ p.repos?.length || 0 }} repos symlinked)
            </option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-300 mb-1.5 flex items-center justify-between">
            <span>Target Repositories (Symlinked in Project Root)</span>
            <span class="text-[11px] text-slate-500 font-mono">{{ selectedRepos.length }} selected</span>
          </label>
          <div class="flex flex-wrap gap-2">
            <label
              v-for="repo in availableRepos"
              :key="repo.name"
              class="flex items-center gap-2 px-3 py-1.5 rounded-lg border text-xs cursor-pointer select-none transition-colors"
              :class="selectedRepos.includes(repo.name) ? 'bg-emerald-950/60 border-emerald-800 text-emerald-300' : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
            >
              <input
                type="checkbox"
                :value="repo.name"
                v-model="selectedRepos"
                class="hidden"
              />
              <Layers class="w-3.5 h-3.5 text-emerald-400" />
              <span class="font-mono font-medium">{{ repo.name }}</span>
              <span class="text-[9px] uppercase px-1 rounded bg-slate-900 border border-slate-800 text-slate-400">{{ repo.role }}</span>
            </label>
          </div>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-300 mb-1.5">Workflow Pipeline Template</label>
          <select
            v-model="selectedWorkflowId"
            class="w-full h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
          >
            <option v-if="workflowStore.workflows.length === 0" value="general_ai_sdlc">
              General AI SDLC (9-Stage Factory) (Default)
            </option>
            <option v-for="w in workflowStore.workflows" :key="w.id" :value="w.id">
              {{ w.name }} ({{ w.stages.length }} stages)
            </option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-300 mb-1.5">Execution Scope</label>
          <div class="grid grid-cols-2 gap-3">
            <label
              class="p-3 rounded-lg border cursor-pointer select-none transition-colors flex flex-col gap-1"
              :class="executionScope === 'full' ? 'bg-emerald-950/40 border-emerald-600 text-emerald-200' : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
            >
              <div class="flex items-center gap-2 font-medium text-xs">
                <input type="radio" value="full" v-model="executionScope" class="text-emerald-500" />
                <span>Full SDLC Pipeline</span>
              </div>
              <span class="text-[11px] text-slate-500">Run all stages in selected workflow</span>
            </label>

            <label
              class="p-3 rounded-lg border cursor-pointer select-none transition-colors flex flex-col gap-1"
              :class="executionScope === 'slice' ? 'bg-emerald-950/40 border-emerald-600 text-emerald-200' : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
            >
              <div class="flex items-center gap-2 font-medium text-xs">
                <input type="radio" value="slice" v-model="executionScope" class="text-emerald-500" />
                <span>Partial Stage Slice (Mid-Process)</span>
              </div>
              <span class="text-[11px] text-slate-500">Run arbitrary range (e.g. Implementation to E2E)</span>
            </label>
          </div>
        </div>

        <template v-if="executionScope === 'slice'">
          <StageRangeSelector
            v-model:start-stage="startStage"
            v-model:halt-stage="haltStage"
            v-model:produce-video="produceVideo"
            :stages="stagesForSelector"
          />

          <ArtifactUploadDropzone
            v-model:branch-name="sourceBranch"
            @file-selected="handleFileSelected"
          />
        </template>

        <div class="p-3.5 rounded-lg bg-slate-950 border border-slate-800 grid grid-cols-2 gap-3">
          <div>
            <label class="block text-[11px] text-slate-400 mb-1">Router Strategy</label>
            <select
              v-model="routerStrategy"
              class="w-full h-8 px-2.5 bg-slate-900 border border-slate-700 rounded text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
            >
              <option value="BEST_PRACTICE">Best Practice Heuristics (Auto)</option>
              <option value="RULE_BASED">Custom Router Rules</option>
            </select>
          </div>

          <div>
            <label class="block text-[11px] text-slate-400 mb-1">Method Assignment</label>
            <select
              v-model="selectedMethod"
              class="w-full h-8 px-2.5 bg-slate-900 border border-slate-700 rounded text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
            >
              <option value="Auto">Auto (Matched by Router)</option>
              <option value="BMAD">BMAD Multi-Agent</option>
              <option value="Supervisor">Supervisor</option>
              <option value="ReAct">ReAct</option>
              <option value="Superpower">Superpower</option>
            </select>
          </div>
        </div>

        <!-- Git Worktree & Dependency DAG Execution Options -->
        <div class="p-3.5 rounded-lg bg-slate-950 border border-slate-800 space-y-3">
          <!-- Worktree parallel execution toggle -->
          <div class="flex items-start justify-between gap-3">
            <div class="flex items-start gap-2">
              <GitBranch class="w-4 h-4 text-emerald-400 mt-0.5 shrink-0" />
              <div>
                <span class="text-xs font-medium text-slate-200">Git Worktree Parallel Isolation</span>
                <p class="text-[11px] text-slate-500 leading-relaxed">
                  Provisions dedicated zero-copy git worktree branches (<code class="text-emerald-400">feat/TASK-xxx</code>) allowing parallel agent development without repo checkout conflicts.
                </p>
              </div>
            </div>
            <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-0.5">
              <input type="checkbox" v-model="useWorktree" class="sr-only peer" />
              <div class="w-9 h-5 bg-slate-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-emerald-600"></div>
            </label>
          </div>

          <div class="border-t border-slate-800 pt-2.5">
            <div class="flex items-center justify-between mb-1.5">
              <span class="text-xs font-medium text-slate-200 flex items-center gap-1.5">
                <Link2 class="w-3.5 h-3.5 text-amber-400" />
                Prerequisite Tasks (DAG Scheduling)
              </span>
              <span class="text-[10px] text-slate-500 font-mono">
                {{ selectedDependencies.length }} prerequisite{{ selectedDependencies.length === 1 ? '' : 's' }}
              </span>
            </div>
            <p class="text-[11px] text-slate-500 mb-2">
              If selected tasks are not yet completed, this task will wait in <span class="text-amber-400 font-mono text-[10px]">WAITING_DEPENDENCY</span> and auto-unblock once prerequisites finish.
            </p>

            <div v-if="availableDependencyTasks.length === 0" class="text-xs text-slate-600 italic py-1">
              No previous tasks available to link as dependencies.
            </div>
            <div v-else class="flex flex-wrap gap-1.5 max-h-28 overflow-y-auto pr-1">
              <button
                v-for="task in availableDependencyTasks"
                :key="task.id"
                type="button"
                @click="toggleDependency(task.id)"
                class="px-2 py-1 rounded text-[11px] border font-mono transition-colors flex items-center gap-1.5"
                :class="selectedDependencies.includes(task.id)
                  ? 'bg-amber-950/50 border-amber-600/70 text-amber-300'
                  : 'bg-slate-900 border-slate-800 text-slate-400 hover:border-slate-700 hover:text-slate-300'"
              >
                <span class="font-bold">{{ task.id }}</span>
                <span class="text-slate-500 max-w-[140px] truncate">{{ task.title }}</span>
                <span
                  class="text-[9px] px-1 py-0.2 rounded font-sans uppercase"
                  :class="task.state === 'COMPLETED' ? 'bg-emerald-950 text-emerald-400' : 'bg-slate-800 text-slate-400'"
                >
                  {{ task.state }}
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="px-6 py-3 border-t border-slate-800 bg-slate-900 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
        >
          Cancel
        </button>
        <BtnPrimary
          :loading="isSubmitting"
          :disabled="!title.trim()"
          @click="handleSubmit"
        >
          Launch Task
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
