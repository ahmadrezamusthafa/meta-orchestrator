<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../../services/api'
import { useTaskStore } from '../../stores/tasks'
import { useToastStore } from '../../stores/toast'
import { renderMarkdown } from '../../utils/markdown'
import { STAGES, stageName, stagePosition, taskLifecycle, TONE_BADGE } from '../../composables/taskLifecycle'
import type { Task, TaskArtifactDTO, TaskDependencyInfoDTO } from '../../types'
import {
  ExternalLink, FileText, CheckCircle2, Circle, CircleDot, Link2, MessageSquarePlus, X, Copy, Check, Layers, Loader2,
} from 'lucide-vue-next'

const props = defineProps<{
  task: Task
  dependencies: TaskDependencyInfoDTO | null
}>()
const emit = defineEmits<{ (e: 'updated'): void }>()

const taskStore = useTaskStore()
const toast = useToastStore()

// What each stage produces (mirrors the daemon's stage instructions).
const STAGE_GOAL: Record<string, string> = {
  prd_discovery: 'Goals, scope, non-goals, acceptance criteria and open questions.',
  atdd_creation: 'Given/When/Then acceptance tests covering every criterion and edge case.',
  techdoc_rfc: 'Technical design: approach, components, data/API changes, risks, alternatives.',
  task_breakdown: 'Small ordered sub-tasks, each with repository, files and verification.',
  task_implementation: 'The code change per file, and how to verify it.',
  e2e_validation: 'End-to-end scenarios, commands to run and expected results.',
  uat_verification: 'UAT checklist and the evidence the reviewer should see.',
  signoff_merge: 'Change summary, risks, rollout notes and pull request description.',
}

const artifacts = ref<TaskArtifactDTO[]>([])
const viewer = ref<{ path: string; content: string; loading: boolean; error: string; raw: boolean } | null>(null)
const guidanceDraft = ref('')
const sendingGuidance = ref(false)
const copied = ref(false)

const meta = computed(() => props.task.metadata || {})
const life = computed(() => taskLifecycle(props.task)!)
const current = computed(() => stagePosition(props.task.current_stage_id).index)
const description = computed(() => renderMarkdown(props.task.description))
const stageDocs = computed(() => Object.fromEntries(artifacts.value.filter((a) => a.kind === 'stage_output').map((a) => [a.stage_id!, a])))
const documents = computed(() => artifacts.value.filter((a) => a.kind !== 'stage_output'))
const budget = computed(() => {
  const used = props.task.token_usage?.total_tokens || 0
  const max = props.task.max_token_budget || 0
  return { used, max, pct: max ? Math.min(100, Math.round((used / max) * 100)) : 0 }
})

function stageStatus(i: number) {
  if (props.task.state === 'COMPLETED' || i < current.value) return 'done'
  if (i === current.value) return 'current'
  return 'upcoming'
}

async function loadArtifacts() {
  try {
    artifacts.value = (await api.getTaskArtifacts(props.task.id)).artifacts
  } catch {
    artifacts.value = []
  }
}

async function openDoc(path: string) {
  viewer.value = { path, content: '', loading: true, error: '', raw: false }
  try {
    viewer.value.content = (await api.getArtifact(props.task.id, path)).content
  } catch (err: any) {
    viewer.value.error = err?.message || 'Could not open document'
  } finally {
    viewer.value.loading = false
  }
}

async function copyDoc() {
  if (!viewer.value) return
  try {
    await navigator.clipboard.writeText(viewer.value.content)
    copied.value = true
    setTimeout(() => (copied.value = false), 1400)
  } catch {
    /* clipboard unavailable */
  }
}

async function sendGuidance() {
  const text = guidanceDraft.value.trim()
  if (!text) return
  sendingGuidance.value = true
  try {
    await taskStore.injectContext(props.task.id, text)
    guidanceDraft.value = ''
    toast.success('Guidance saved', props.task.state === 'BLOCKED_FRUSTRATION' ? 'Re-running the stage with your guidance.' : 'The agent follows it on the next stage run.')
    emit('updated')
  } catch (err: any) {
    toast.error('Could not save guidance', err?.message || 'Unknown error')
  } finally {
    sendingGuidance.value = false
  }
}

function formatSize(n: number) {
  return n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1048576).toFixed(1)} MB`
}

watch(() => [props.task.id, props.task.current_stage_id, props.task.state], loadArtifacts)
onMounted(loadArtifacts)
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="max-w-5xl mx-auto p-5 grid lg:grid-cols-3 gap-4">
      <!-- Main column -->
      <div class="lg:col-span-2 space-y-4">
        <!-- Summary -->
        <section class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
          <div class="flex flex-wrap items-center gap-2 text-[11px]">
            <span class="px-1.5 py-0.5 rounded border font-semibold" :class="TONE_BADGE[life.tone]">{{ life.status }}</span>
            <span class="text-slate-400">{{ stageName(task.current_stage_id) }} · stage {{ current + 1 }} of {{ STAGES.length }}</span>
            <a v-if="meta.jira_key" :href="meta.jira_url" target="_blank" rel="noopener"
              class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-blue-950/70 border border-blue-800/70 text-blue-300 hover:text-blue-100 font-mono">
              {{ meta.jira_key }} <ExternalLink class="w-2.5 h-2.5" />
            </a>
            <span v-if="meta.jira_epic_key && meta.jira_epic_key !== meta.jira_key"
              class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-violet-950/60 border border-violet-800/60 text-violet-300">
              <Layers class="w-2.5 h-2.5" /> {{ meta.jira_epic_name || meta.jira_epic_key }}
            </span>
          </div>
          <h2 class="text-base font-semibold text-slate-100 leading-snug">{{ task.title }}</h2>
          <dl v-if="meta.jira_key" class="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
            <div><dt class="text-[10px] uppercase tracking-wider text-slate-500">JIRA status</dt><dd class="text-slate-200">{{ meta.jira_status || '—' }}</dd></div>
            <div><dt class="text-[10px] uppercase tracking-wider text-slate-500">Type</dt><dd class="text-slate-200">{{ meta.jira_issue_type || '—' }}</dd></div>
            <div><dt class="text-[10px] uppercase tracking-wider text-slate-500">Priority</dt><dd class="text-slate-200">{{ meta.jira_priority || '—' }}</dd></div>
            <div><dt class="text-[10px] uppercase tracking-wider text-slate-500">Assignee</dt><dd class="text-slate-200 truncate">{{ meta.jira_assignee || 'Unassigned' }}</dd></div>
          </dl>
        </section>

        <!-- Requirements -->
        <section class="p-5 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
          <header class="flex items-center justify-between">
            <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Requirements</h3>
            <span class="text-[11px] text-slate-500">{{ meta.jira_key ? `From JIRA ${meta.jira_key}` : 'Written when the task was created' }}</span>
          </header>
          <div v-if="description" class="prose prose-invert prose-sm max-w-none text-slate-300" v-html="description"></div>
          <p v-else class="text-xs text-slate-500">
            No description. Add requirements as guidance below so the agent knows what "done" means.
          </p>
        </section>

        <!-- Stage plan -->
        <section class="p-5 rounded-xl bg-slate-900 border border-slate-800">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">Stage plan</h3>
          <ol class="space-y-1">
            <li v-for="(s, i) in STAGES" :key="s" class="flex items-start gap-3 p-2 rounded-lg"
              :class="stageStatus(i) === 'current' ? 'bg-slate-800/60' : ''">
              <CheckCircle2 v-if="stageStatus(i) === 'done'" class="w-4 h-4 text-emerald-400 flex-shrink-0 mt-0.5" />
              <CircleDot v-else-if="stageStatus(i) === 'current'" class="w-4 h-4 text-sky-400 flex-shrink-0 mt-0.5" />
              <Circle v-else class="w-4 h-4 text-slate-600 flex-shrink-0 mt-0.5" />
              <div class="min-w-0 flex-1">
                <div class="text-xs font-medium" :class="stageStatus(i) === 'upcoming' ? 'text-slate-400' : 'text-slate-100'">
                  {{ i + 1 }}. {{ stageName(s) }}
                  <span v-if="stageStatus(i) === 'current'" class="ml-1 text-[10px] font-normal text-sky-300">{{ life.status }}</span>
                </div>
                <div class="text-[11px] text-slate-500">{{ STAGE_GOAL[s] }}</div>
              </div>
              <button v-if="stageDocs[s]" type="button" @click="openDoc(stageDocs[s].path)"
                class="flex-shrink-0 h-6 px-2 rounded bg-slate-800 hover:bg-slate-700 text-[11px] text-slate-200 flex items-center gap-1">
                <FileText class="w-3 h-3" /> Output
              </button>
            </li>
          </ol>
        </section>
      </div>

      <!-- Side column -->
      <div class="space-y-4">
        <!-- Guidance -->
        <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-2.5">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Guidance for the agent</h3>
          <p class="text-[11px] text-slate-500">Constraints or context sent with every stage run.</p>
          <pre v-if="meta.operator_guidance" class="p-2.5 rounded-lg bg-slate-950 border border-slate-800 text-[11px] text-slate-300 whitespace-pre-wrap font-sans">{{ meta.operator_guidance }}</pre>
          <textarea v-model="guidanceDraft" rows="3" placeholder="e.g. Reuse the existing RefundService; don't add new tables."
            aria-label="New guidance" @keydown.meta.enter="sendGuidance" @keydown.ctrl.enter="sendGuidance"
            class="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:border-blue-500" />
          <button type="button" @click="sendGuidance" :disabled="!guidanceDraft.trim() || sendingGuidance"
            class="w-full h-8 rounded-lg bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-xs text-slate-100 flex items-center justify-center gap-1.5">
            <Loader2 v-if="sendingGuidance" class="w-3.5 h-3.5 animate-spin" /><MessageSquarePlus v-else class="w-3.5 h-3.5" />
            Add guidance
          </button>
        </section>

        <!-- Prerequisites -->
        <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-2">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Prerequisites</h3>
          <p v-if="!task.dependencies?.length" class="text-[11px] text-slate-500">None — this task can run independently.</p>
          <router-link v-for="d in dependencies?.details || []" :key="d.id" :to="`/tasks/${d.id}`"
            class="flex items-center gap-2 p-2 rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-xs">
            <Link2 class="w-3.5 h-3.5 flex-shrink-0" :class="d.state === 'COMPLETED' ? 'text-emerald-400' : 'text-orange-400'" />
            <span class="font-mono text-slate-200">{{ d.id }}</span>
            <span class="truncate text-slate-400">{{ d.title }}</span>
            <span class="ml-auto text-[10px]" :class="d.state === 'COMPLETED' ? 'text-emerald-400' : 'text-orange-300'">{{ d.state === 'COMPLETED' ? 'Done' : stageName(d.current_stage_id) }}</span>
          </router-link>
          <template v-if="task.dependencies?.length && !dependencies">
            <span v-for="id in task.dependencies" :key="id" class="block text-xs font-mono text-slate-400">{{ id }}</span>
          </template>
        </section>

        <!-- Documents -->
        <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-2">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Documents</h3>
          <p v-if="documents.length === 0" class="text-[11px] text-slate-500">No attached documents. Stage outputs appear in the stage plan as each stage finishes.</p>
          <button v-for="d in documents" :key="d.path" type="button" @click="d.kind === 'document' ? openDoc(d.path) : undefined"
            class="w-full flex items-center gap-2 p-2 rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-left text-xs">
            <FileText class="w-3.5 h-3.5 text-slate-400 flex-shrink-0" />
            <span class="truncate text-slate-200">{{ d.path }}</span>
            <span class="ml-auto text-[10px] text-slate-500 flex-shrink-0">{{ formatSize(d.size) }}</span>
          </button>
        </section>

        <!-- Budget -->
        <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-2 text-xs">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Execution</h3>
          <div class="flex justify-between"><span class="text-slate-500">Workflow</span><span class="text-slate-200">{{ task.workflow_id }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">Method</span><span class="text-slate-200">{{ task.selected_method }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">Last model</span><span class="text-purple-300 font-mono truncate ml-2">{{ meta.active_model || 'Not run yet' }}</span></div>
          <div>
            <div class="flex justify-between mb-1"><span class="text-slate-500">Token budget</span>
              <span class="font-mono text-slate-200">{{ budget.used.toLocaleString() }} / {{ budget.max ? budget.max.toLocaleString() : '∞' }}</span></div>
            <div v-if="budget.max" class="h-1.5 rounded-full bg-slate-800 overflow-hidden">
              <div class="h-full" :class="budget.pct > 90 ? 'bg-rose-500' : budget.pct > 70 ? 'bg-amber-500' : 'bg-emerald-500'" :style="{ width: `${budget.pct}%` }"></div>
            </div>
          </div>
          <div v-if="task.token_usage?.estimated_cost_usd" class="flex justify-between"><span class="text-slate-500">Estimated cost</span><span class="font-mono text-slate-200">${{ task.token_usage.estimated_cost_usd.toFixed(4) }}</span></div>
        </section>
      </div>
    </div>

    <!-- Document viewer -->
    <div v-if="viewer" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm" @click.self="viewer = null" @keydown.esc="viewer = null">
      <div role="dialog" aria-modal="true" :aria-label="viewer.path" class="w-full max-w-3xl max-h-[85vh] flex flex-col bg-slate-900 border border-slate-800 rounded-xl shadow-2xl">
        <header class="px-5 py-3 border-b border-slate-800 flex items-center gap-2">
          <FileText class="w-4 h-4 text-slate-400" />
          <span class="font-mono text-xs text-slate-200 truncate">{{ viewer.path }}</span>
          <div class="ml-auto flex items-center gap-1">
            <button type="button" @click="viewer.raw = !viewer.raw" class="h-7 px-2 rounded text-[11px] text-slate-300 hover:bg-slate-800">{{ viewer.raw ? 'Preview' : 'Markdown' }}</button>
            <button type="button" @click="copyDoc" aria-label="Copy document" class="h-7 w-7 rounded flex items-center justify-center text-slate-400 hover:text-slate-100 hover:bg-slate-800">
              <Check v-if="copied" class="w-3.5 h-3.5 text-emerald-400" /><Copy v-else class="w-3.5 h-3.5" />
            </button>
            <button type="button" @click="viewer = null" aria-label="Close" class="h-7 w-7 rounded flex items-center justify-center text-slate-400 hover:text-slate-100 hover:bg-slate-800">
              <X class="w-4 h-4" />
            </button>
          </div>
        </header>
        <div class="flex-1 overflow-y-auto p-5">
          <p v-if="viewer.loading" class="text-xs text-slate-500 animate-pulse">Loading…</p>
          <p v-else-if="viewer.error" class="text-xs text-rose-300">{{ viewer.error }}</p>
          <pre v-else-if="viewer.raw" class="text-xs text-slate-300 whitespace-pre-wrap">{{ viewer.content }}</pre>
          <div v-else class="prose prose-invert prose-sm max-w-none text-slate-300" v-html="renderMarkdown(viewer.content)"></div>
        </div>
      </div>
    </div>
  </div>
</template>
