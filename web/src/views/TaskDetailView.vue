<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTaskStore } from '../stores/tasks'
import { useTerminalStore } from '../stores/terminal'
import { useToastStore } from '../stores/toast'
import SplitPane from '../components/layout/SplitPane.vue'
import XtermTerminal from '../components/terminal/XtermTerminal.vue'
import ThoughtFeed from '../components/thought/ThoughtFeed.vue'
import WorkspaceGraph from '../components/workspace/WorkspaceGraph.vue'
import VideoPlayerVault from '../components/evidence/VideoPlayerVault.vue'
import ScreenshotDiff from '../components/evidence/ScreenshotDiff.vue'
import ArtifactMarkdownViewer from '../components/evidence/ArtifactMarkdownViewer.vue'
import FrustrationBanner from '../components/hitl/FrustrationBanner.vue'
import ContextInput from '../components/hitl/ContextInput.vue'
import ResetWorkspaceModal from '../components/hitl/ResetWorkspaceModal.vue'
import GateApprovalBar from '../components/hitl/GateApprovalBar.vue'
import PhaseStatusBadge from '../components/common/PhaseStatusBadge.vue'
import RoutingExplainerPill from '../components/common/RoutingExplainerPill.vue'
import OperatorGuidanceCard from '../components/common/OperatorGuidanceCard.vue'
import {
  ChevronLeft,
  Terminal,
  BrainCircuit,
  Network,
  Film,
  Image,
  FileCode,
} from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const taskId = computed(() => route.params.id as string)

const taskStore = useTaskStore()
const terminalStore = useTerminalStore()
const toastStore = useToastStore()

const leftTab = ref<'terminal' | 'thoughts' | 'graph'>('terminal')
const rightTab = ref<'artifacts' | 'video' | 'screenshots'>('artifacts')
const showResetModal = ref(false)

let unsubscribeWS: (() => void) | null = null

onMounted(async () => {
  await taskStore.fetchTask(taskId.value)
  unsubscribeWS = terminalStore.initTaskListeners(taskId.value)
})

onUnmounted(() => {
  if (unsubscribeWS) unsubscribeWS()
})

const currentTask = computed(() => taskStore.activeTask)
const isFrustrated = computed(() => currentTask.value?.state === 'BLOCKED_FRUSTRATION')
const isWaitingGate = computed(() => currentTask.value?.state === 'WAITING_GATE_APPROVAL')

async function handleInjectGuidance(instruction: string) {
  try {
    await taskStore.injectContext(taskId.value, instruction)
    toastStore.success('Steering Dispatched', 'Instruction injected into active agent loop')
  } catch (err: any) {
    toastStore.error('Steering Failed', err?.message || 'Server error')
  }
}

async function handleBannerInject() {
  await handleInjectGuidance('Operator reviewed failure trace and authorized retry from checkpoint.')
}

async function handleResetWorkspace() {
  try {
    await taskStore.resetWorkspace(taskId.value)
    showResetModal.value = false
    toastStore.warning('Workspace Reset', 'Container purged and volume state refreshed')
  } catch (err: any) {
    toastStore.error('Reset Failed', err?.message || 'Server error')
  }
}

async function handleGateApprove() {
  try {
    await taskStore.updateGate(taskId.value, true)
    toastStore.success('Gate Approved', 'Advancing stage in zero-trust pipeline')
  } catch (err: any) {
    toastStore.error('Gate Approval Failed', err?.message || 'Server error')
  }
}

async function handleGateReject() {
  try {
    await taskStore.updateGate(taskId.value, false, 'Revision requested by operator')
    toastStore.info('Gate Rejected', 'Task halted for operator revision')
  } catch (err: any) {
    toastStore.error('Gate Rejection Failed', err?.message || 'Server error')
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Sub-Header / Breadcrumb Toolbar -->
    <div class="h-12 px-4 bg-slate-900/60 border-b border-slate-800 flex items-center justify-between">
      <div class="flex items-center gap-3">
        <button
          @click="router.push('/')"
          class="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
        >
          <ChevronLeft class="w-5 h-5" />
        </button>

        <div class="flex items-center gap-2">
          <span class="font-mono text-xs font-bold text-slate-100">{{ taskId }}</span>
          <span class="text-slate-600">/</span>
          <h2 class="text-xs font-medium text-slate-300 truncate max-w-md">
            {{ currentTask?.title || 'Loading task details...' }}
          </h2>
        </div>
      </div>

      <!-- Right Metadata Indicators -->
      <div v-if="currentTask" class="flex items-center gap-2.5">
        <RoutingExplainerPill
          :source="currentTask.metadata?.router_source"
          :rationale="currentTask.metadata?.router_rationale"
        />

        <span class="px-2 py-0.5 rounded bg-slate-800 text-[10px] font-mono text-slate-300">
          Method: {{ currentTask.selected_method }}
        </span>

        <PhaseStatusBadge :state="currentTask.state" />
      </div>
    </div>

    <!-- Sticky Frustration Alert Banner (when BLOCKED_FRUSTRATION) -->
    <FrustrationBanner
      v-if="isFrustrated"
      :trace="currentTask?.metadata?.failing_trace"
      @inject-guidance="handleBannerInject"
      @reset-workspace="showResetModal = true"
    />

    <!-- Gate Approval Bar (when WAITING_GATE_APPROVAL) -->
    <GateApprovalBar
      v-if="isWaitingGate"
      :stage-id="currentTask?.current_stage_id || ''"
      stage-name="Tech Doc / Architecture RFC Gate"
      @approve="handleGateApprove"
      @reject="handleGateReject"
    />

    <!-- Step-by-Step Operator Guidance Card -->
    <div v-if="currentTask" class="px-4 py-2 bg-slate-950/80 border-b border-slate-800/80">
      <OperatorGuidanceCard
        :task="currentTask"
        @approve-gate="handleGateApprove"
        @reject-gate="handleGateReject"
        @steer-click="leftTab = 'terminal'"
      />
    </div>

    <!-- Main Resizable Execution Workspace (60/40 Split) -->
    <main class="flex-1 overflow-hidden">
      <SplitPane>
        <!-- LEFT EXECUTION PANE -->
        <template #left>
          <div class="h-full flex flex-col bg-slate-950">
            <!-- Left Tabs Bar -->
            <div class="h-9 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between">
              <div class="flex items-center gap-1">
                <button
                  @click="leftTab = 'terminal'"
                  class="h-7 px-3 rounded text-xs font-mono transition-colors flex items-center gap-1.5"
                  :class="leftTab === 'terminal' ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
                >
                  <Terminal class="w-3.5 h-3.5" />
                  <span>Terminal</span>
                </button>

                <button
                  @click="leftTab = 'thoughts'"
                  class="h-7 px-3 rounded text-xs font-mono transition-colors flex items-center gap-1.5"
                  :class="leftTab === 'thoughts' ? 'bg-slate-800 text-sky-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
                >
                  <BrainCircuit class="w-3.5 h-3.5" />
                  <span>Thought Stream</span>
                </button>

                <button
                  @click="leftTab = 'graph'"
                  class="h-7 px-3 rounded text-xs font-mono transition-colors flex items-center gap-1.5"
                  :class="leftTab === 'graph' ? 'bg-slate-800 text-purple-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
                >
                  <Network class="w-3.5 h-3.5" />
                  <span>Workspace Graph</span>
                </button>
              </div>

              <div class="text-[10px] font-mono text-slate-500">
                Container: /workspaces/{{ taskId }}
              </div>
            </div>

            <!-- Left Tab Content -->
            <div class="flex-1 overflow-hidden">
              <XtermTerminal
                v-if="leftTab === 'terminal'"
                :logs="terminalStore.logs"
              />
              <ThoughtFeed
                v-else-if="leftTab === 'thoughts'"
                :thoughts="terminalStore.thoughts"
                :task="currentTask || undefined"
              />
              <WorkspaceGraph
                v-else-if="leftTab === 'graph'"
                :task-id="taskId"
                :assigned-repos="currentTask?.assigned_repos"
                :current-stage-id="currentTask?.current_stage_id"
                :is-write-locked="currentTask?.current_stage_id === 'atdd_creation' || currentTask?.metadata?.write_lock === 'ACTIVE'"
              />
            </div>
          </div>
        </template>

        <!-- RIGHT CONTROL & ARTIFACT PANE -->
        <template #right>
          <div class="h-full flex flex-col justify-between bg-slate-900/30">
            <!-- Right Tabs Bar -->
            <div class="h-9 px-3 bg-slate-900 border-b border-slate-800 flex items-center gap-1">
              <button
                @click="rightTab = 'artifacts'"
                class="h-7 px-3 rounded text-xs font-mono transition-colors flex items-center gap-1.5"
                :class="rightTab === 'artifacts' ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
              >
                <FileCode class="w-3.5 h-3.5" />
                <span>SDLC Artifacts</span>
              </button>

              <button
                @click="rightTab = 'video'"
                class="h-7 px-3 rounded text-xs font-mono transition-colors flex items-center gap-1.5"
                :class="rightTab === 'video' ? 'bg-slate-800 text-purple-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
              >
                <Film class="w-3.5 h-3.5" />
                <span>Test Video (.mp4)</span>
              </button>

              <button
                @click="rightTab = 'screenshots'"
                class="h-7 px-3 rounded text-xs font-mono transition-colors flex items-center gap-1.5"
                :class="rightTab === 'screenshots' ? 'bg-slate-800 text-sky-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
              >
                <Image class="w-3.5 h-3.5" />
                <span>Screenshots</span>
              </button>
            </div>

            <!-- Right Tab Content -->
            <div class="flex-1 overflow-hidden">
              <ArtifactMarkdownViewer
                v-if="rightTab === 'artifacts'"
                :task-id="taskId"
              />
              <VideoPlayerVault
                v-else-if="rightTab === 'video'"
                :video-url="currentTask?.metadata?.video_url || `/api/v1/artifacts/${taskId}/run_final.mp4`"
              />
              <ScreenshotDiff
                v-else-if="rightTab === 'screenshots'"
              />
            </div>

            <!-- Sticky Bottom Prompt Steering Dock -->
            <ContextInput
              @send="handleInjectGuidance"
              @reset="showResetModal = true"
            />
          </div>
        </template>
      </SplitPane>
    </main>

    <!-- Reset Workspace Modal -->
    <ResetWorkspaceModal
      v-if="showResetModal"
      :task-id="taskId"
      @close="showResetModal = false"
      @confirm="handleResetWorkspace"
    />
  </div>
</template>
