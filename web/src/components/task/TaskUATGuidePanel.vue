<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { useTaskStore } from '../../stores/tasks'
import MarkdownView from '../common/MarkdownView.vue'
import type { Task, UATGuideStatusDTO } from '../../types'
import {
  ClipboardCheck, Download, Loader2, RefreshCw, AlertCircle, Camera, Settings2, ListChecks, CheckCircle2, MessageSquareWarning,
  Plus, X, Lock,
} from 'lucide-vue-next'

const props = defineProps<{ task: Task }>()
const toast = useToastStore()
const taskStore = useTaskStore()

const status = ref<UATGuideStatusDTO | null>(null)
const guide = ref('')
const loading = ref(false)
const starting = ref(false)
const requesting = ref(false)
const showSettings = ref(false)
const showScope = ref(false)
const envs = ref<Record<string, { url: string; storage_state: string }>>({})
const atddPath = ref('')
// Test-data values for the plan's ${NAME} placeholders; removed holds names to clear on save.
const vars = ref<Record<string, string>>({})
const removedVars = ref<string[]>([])
const newVarName = ref('')
const VAR_NAME = /^[A-Za-z][A-Za-z0-9_]{0,63}$/
const ignoreHTTPS = ref(false)
let poll: ReturnType<typeof setInterval> | null = null

const generating = computed(() => status.value?.status === 'GENERATING')
const htmlURL = computed(() => status.value?.html_path
  ? `/api/v1/artifacts/${encodeURIComponent(props.task.id)}/${status.value.html_path}` : '')
const webApps = computed(() => (status.value?.apps || []).filter((a) => a.kind !== 'api'))
const appName = (id: string) => status.value?.apps.find((a) => a.id === id)?.name || id
const missing = computed(() => status.value?.coverage?.missing?.filter(Boolean) || [])
const scope = computed(() => status.value?.atdd)
const missingEnv = computed(() => webApps.value.filter((a) => !a.base_url).map((a) => a.name))
const variables = computed(() => status.value?.variables || [])
const testDataVars = computed(() => variables.value.filter((v) => !v.secret))
const secretVars = computed(() => variables.value.filter((v) => v.secret && v.used))
const missingVars = computed(() => variables.value.filter((v) => v.used && v.source === 'missing').map((v) => v.name))
const extraVarNames = computed(() => Object.keys(vars.value).filter((n) => !testDataVars.value.some((v) => v.name === n)))
const placeholderOf = (name: string) => '${' + name + '}'

function addVar() {
  const name = newVarName.value.trim().replace(/^\$\{|\}$/g, '')
  if (!VAR_NAME.test(name)) {
    toast.error('Invalid name', 'Use letters, digits and _ only, e.g. UAT_PI_ID_PAID.')
    return
  }
  if (!(name in vars.value)) vars.value[name] = ''
  removedVars.value = removedVars.value.filter((n) => n !== name)
  newVarName.value = ''
}

function removeVar(name: string) {
  delete vars.value[name]
  if (!removedVars.value.includes(name)) removedVars.value.push(name)
}
const KIND_BADGE: Record<string, string> = {
  Sanity: 'bg-amber-950/60 border-amber-800/70 text-amber-200',
  UAT: 'bg-sky-950/60 border-sky-800/70 text-sky-200',
  'Sanity + UAT': 'bg-violet-950/60 border-violet-800/70 text-violet-200',
}

function syncPolling() {
  if (generating.value && !poll) poll = setInterval(load, 2500)
  if (!generating.value && poll) {
    clearInterval(poll)
    poll = null
  }
}

function adoptSettings(s: UATGuideStatusDTO) {
  envs.value = Object.fromEntries((s.apps || []).map((a) => [a.id, { url: a.base_url || '', storage_state: a.storage_state || '' }]))
  atddPath.value = s.atdd?.path || ''
  vars.value = Object.fromEntries((s.variables || []).filter((v) => !v.secret).map((v) => [v.name, v.value || '']))
  removedVars.value = []
  ignoreHTTPS.value = s.ignore_https_errors
}

async function load() {
  loading.value = !status.value
  try {
    const s = await api.getUATGuide(props.task.id)
    const wasGenerating = generating.value
    status.value = s
    if (!showSettings.value) adoptSettings(s)
    if (s.guide_path && (wasGenerating || !guide.value || s.status === 'READY')) {
      guide.value = (await api.getArtifact(props.task.id, s.guide_path)).content
    }
  } catch {
    status.value = null
  } finally {
    loading.value = false
    syncPolling()
  }
}

async function submit(saveOnly: boolean) {
  if (starting.value || generating.value) return
  starting.value = true
  try {
    status.value = await api.generateUATGuide(props.task.id, {
      apps: Object.fromEntries(Object.entries(envs.value).map(([id, e]) => [id, { url: e.url.trim(), storage_state: e.storage_state.trim() }])),
      atdd_path: atddPath.value.trim(),
      variables: {
        ...Object.fromEntries(removedVars.value.map((n) => [n, ''])),
        ...Object.fromEntries(Object.entries(vars.value).map(([n, v]) => [n, v.trim()])),
      },
      ignore_https_errors: ignoreHTTPS.value,
      save_only: saveOnly,
    })
    showSettings.value = false
    if (saveOnly) toast.success('UAT settings saved', 'They apply the next time the guide is generated.')
    else toast.info('Generating UAT guide', 'Replaying each scenario in its application to capture screenshots.')
  } catch (err: any) {
    toast.error(saveOnly ? 'Could not save the UAT settings' : 'Could not generate the UAT guide', err?.message || 'Unknown error')
  } finally {
    starting.value = false
    syncPolling()
  }
}

// Send the stage back to the agent with the exact cases its plan left out.
async function requestMissing() {
  if (!missing.value.length || requesting.value) return
  requesting.value = true
  try {
    const titles = missing.value.map((id) => {
      const c = scope.value?.cases.find((x) => x.id === id)
      return c ? `- ${id} (${c.kind}, ${appName(c.app)}): ${c.title}` : `- ${id}`
    })
    await api.gateApproval(props.task.id, false,
      `The UAT plan does not walk through these ATDD cases marked UAT or Sanity. Add a scenario covering each one ` +
      `(put its id in "covers"), with concrete screen steps, "where", and the expected result for every step:\n${titles.join('\n')}`)
    await taskStore.fetchTask(props.task.id)
    toast.info('Sent back to the agent', `The UAT stage re-runs to cover ${missing.value.length} missing case(s).`)
  } catch (err: any) {
    toast.error('Could not send the stage back', err?.message || 'Unknown error')
  } finally {
    requesting.value = false
  }
}

watch(() => props.task.metadata?.uat_guide_status, load)
watch(() => props.task.state, load)
onMounted(load)
onBeforeUnmount(() => poll && clearInterval(poll))
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="max-w-5xl mx-auto p-5 space-y-4">
      <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
        <div class="flex flex-wrap items-start gap-4">
          <div class="flex-1 min-w-[240px] space-y-1">
            <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
              <ClipboardCheck class="w-4 h-4 text-emerald-400" /> UAT guide
            </h3>
            <p class="text-xs text-slate-400 leading-relaxed">
              A step-by-step guide for business testers covering every ATDD case marked <strong class="text-slate-200">UAT</strong> or
              <strong class="text-slate-200">Sanity</strong>, organized by the application each case runs in. Each scenario is replayed in its
              application with a screenshot after every step. Use staging environments with test data only.
            </p>
          </div>
          <div class="flex items-center gap-2">
            <a v-if="htmlURL && !generating" :href="htmlURL" download
              class="h-8 px-3 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5">
              <Download class="w-3.5 h-3.5" /> Download for testers
            </a>
            <button type="button" @click="showSettings = !showSettings" :aria-expanded="showSettings"
              class="h-8 px-3 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5">
              <Settings2 class="w-3.5 h-3.5" /> Environments
            </button>
            <button type="button" @click="submit(false)" :disabled="!status?.stage_ready || starting || generating"
              :title="status?.stage_ready ? '' : 'Run the uat_verification stage first'"
              class="h-8 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed">
              <Loader2 v-if="starting || generating" class="w-3.5 h-3.5 animate-spin" /><RefreshCw v-else class="w-3.5 h-3.5" />
              {{ generating ? 'Capturing…' : status?.guide_path ? 'Regenerate' : 'Generate guide' }}
            </button>
          </div>
        </div>

        <form v-if="showSettings" class="space-y-4 pt-3 border-t border-slate-800" @submit.prevent="submit(false)">
          <div v-for="app in webApps" :key="app.id" class="grid gap-2 sm:grid-cols-2">
            <div class="sm:col-span-2 text-xs font-semibold text-slate-200">
              {{ app.name }} <span v-if="app.repos?.length" class="font-normal text-slate-500">· {{ app.repos.join(', ') }}</span>
            </div>
            <label class="block space-y-1">
              <span class="text-[11px] uppercase tracking-wide text-slate-500">Environment URL</span>
              <input v-model="envs[app.id].url" type="url" placeholder="https://staging…"
                class="w-full h-9 px-3 rounded-lg bg-slate-950 border border-slate-700 text-sm text-slate-100 focus:outline-none focus:border-emerald-500" />
            </label>
            <label class="block space-y-1">
              <span class="text-[11px] uppercase tracking-wide text-slate-500">Logged-in session file (optional)</span>
              <input v-model="envs[app.id].storage_state" type="text" placeholder="/path/to/storage-state.json"
                class="w-full h-9 px-3 rounded-lg bg-slate-950 border border-slate-700 font-mono text-xs text-slate-100 focus:outline-none focus:border-emerald-500" />
            </label>
          </div>
          <p class="text-[11px] text-slate-500 leading-relaxed">
            The session file is a Playwright storage state for a UAT test account, so pages behind login can be captured. Login steps may use
            <code class="font-mono">${UAT_USERNAME}</code> / <code class="font-mono">${UAT_PASSWORD}</code>, read from the daemon's environment —
            never written into the guide.
          </p>
          <fieldset class="space-y-2">
            <legend class="text-xs font-semibold text-slate-200">Test data</legend>
            <p class="text-[11px] text-slate-500 leading-relaxed">
              Values for the <code class="font-mono">${…}</code> placeholders in the plan — the staging records each scenario opens
              (e.g. a proforma invoice id). They are saved with this task and shown to testers in the guide, so use test records only.
            </p>
            <div v-if="!testDataVars.length && !extraVarNames.length" class="text-[11px] text-slate-500">The current plan uses no test-data placeholders.</div>
            <div v-for="v in testDataVars.filter((x) => !removedVars.includes(x.name))" :key="v.name" class="grid gap-2 sm:grid-cols-[minmax(0,16rem)_1fr_auto] items-center">
              <code class="font-mono text-xs text-slate-300 truncate" :title="placeholderOf(v.name)">{{ placeholderOf(v.name) }}</code>
              <input v-model="vars[v.name]" type="text" :placeholder="v.source === 'environment' ? 'set in the orchestrator environment' : 'test record value'"
                :aria-label="`Value for ${v.name}`"
                class="w-full h-9 px-3 rounded-lg bg-slate-950 border text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                :class="v.used && !vars[v.name] && v.source !== 'environment' ? 'border-amber-700' : 'border-slate-700'" />
              <span class="text-[11px] whitespace-nowrap" :class="v.used ? 'text-slate-500' : 'text-slate-600'">
                {{ v.used ? (v.source === 'environment' && !vars[v.name] ? 'from environment' : 'used by plan') : 'not in plan' }}
                <button v-if="!v.used" type="button" class="ml-1 text-slate-500 hover:text-rose-300" :aria-label="`Remove ${v.name}`" @click="removeVar(v.name)">
                  <X class="w-3.5 h-3.5 inline" />
                </button>
              </span>
            </div>
            <div v-for="name in extraVarNames" :key="name" class="grid gap-2 sm:grid-cols-[minmax(0,16rem)_1fr_auto] items-center">
              <code class="font-mono text-xs text-slate-300 truncate">{{ placeholderOf(name) }}</code>
              <input v-model="vars[name]" type="text" placeholder="test record value" :aria-label="`Value for ${name}`"
                class="w-full h-9 px-3 rounded-lg bg-slate-950 border border-slate-700 text-sm text-slate-100 focus:outline-none focus:border-emerald-500" />
              <button type="button" class="text-slate-500 hover:text-rose-300" :aria-label="`Remove ${name}`" @click="removeVar(name)"><X class="w-3.5 h-3.5" /></button>
            </div>
            <div class="flex items-center gap-2">
              <input v-model="newVarName" type="text" placeholder="UAT_PI_ID_PAID" aria-label="New test data name" @keydown.enter.prevent="addVar"
                class="w-64 h-8 px-3 rounded-lg bg-slate-950 border border-slate-700 font-mono text-xs text-slate-100 focus:outline-none focus:border-emerald-500" />
              <button type="button" @click="addVar" class="h-8 px-2.5 rounded-lg border border-slate-700 text-xs text-slate-200 hover:bg-slate-800 flex items-center gap-1">
                <Plus class="w-3.5 h-3.5" /> Add
              </button>
            </div>
            <p v-if="secretVars.length" class="text-[11px] text-slate-500 flex items-start gap-1.5">
              <Lock class="w-3.5 h-3.5 flex-shrink-0 mt-px" />
              <span>
                Credentials (<code class="font-mono">{{ secretVars.map((v) => v.name).join(', ') }}</code>) are never stored with the task — set them in the
                orchestrator's environment before starting it<template v-if="secretVars.some((v) => v.source === 'missing')">; missing:
                  <span class="text-amber-300 font-mono">{{ secretVars.filter((v) => v.source === 'missing').map((v) => v.name).join(', ') }}</span></template>.
              </span>
            </p>
          </fieldset>
          <label class="block space-y-1">
            <span class="text-[11px] uppercase tracking-wide text-slate-500">ATDD sheet (optional)</span>
            <input v-model="atddPath" type="text" placeholder="/path/to/initiative-atdd.csv"
              class="w-full h-9 px-3 rounded-lg bg-slate-950 border border-slate-700 font-mono text-xs text-slate-100 focus:outline-none focus:border-emerald-500" />
            <span class="block text-[11px] text-slate-500">
              Leave empty to use the newest ATDD CSV in the task's documents or the worktree's <code class="font-mono">_bmad-output/</code>, then the
              ATDD stage output.
            </span>
          </label>
          <div class="flex flex-wrap items-center gap-3">
            <label class="flex items-center gap-2 text-xs text-slate-300">
              <input v-model="ignoreHTTPS" type="checkbox" class="rounded border-slate-600 bg-slate-950" /> Accept self-signed certificates
            </label>
            <button type="button" @click="submit(true)" :disabled="starting" class="ml-auto h-8 px-3 rounded-lg border border-slate-700 text-xs text-slate-200 hover:bg-slate-800">
              Save only
            </button>
            <button type="submit" :disabled="!status?.stage_ready || starting || generating"
              class="h-8 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50">
              <Camera class="w-3.5 h-3.5" /> Save & capture
            </button>
          </div>
        </form>

        <div class="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-slate-500">
          <span v-for="app in webApps" :key="app.id">{{ app.name }}: <span class="text-slate-300">{{ app.base_url || 'URL not set' }}</span></span>
          <span v-if="status?.generated_at">Generated {{ new Date(status.generated_at).toLocaleString() }}</span>
        </div>
      </section>

      <!-- Coverage: the guide's contract with the ATDD sheet -->
      <section v-if="scope" class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
        <div class="flex flex-wrap items-center gap-3">
          <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2"><ListChecks class="w-4 h-4 text-sky-400" /> Test case coverage</h3>
          <span v-if="scope.error" class="text-xs text-rose-300">{{ scope.error }}</span>
          <span v-else-if="!scope.total" class="text-xs text-slate-400">No ATDD sheet found — the guide uses the UAT plan only.</span>
          <span v-else class="text-xs text-slate-400">
            {{ scope.in_scope }} of {{ scope.total }} cases in scope ({{ scope.sanity }} Sanity) · from {{ scope.source }}
          </span>
          <button v-if="scope.in_scope" type="button" @click="showScope = !showScope" class="ml-auto text-xs text-slate-400 hover:text-slate-100">
            {{ showScope ? 'Hide cases' : 'Show cases' }}
          </button>
        </div>
        <div v-if="status?.guide_path && scope.in_scope" class="text-xs flex flex-wrap items-center gap-2"
          :class="missing.length ? 'text-amber-200' : 'text-emerald-300'">
          <template v-if="missing.length">
            <AlertCircle class="w-4 h-4" />
            {{ missing.length }} case(s) were not in the agent's plan and are written from the ATDD sheet without screenshots:
            <span class="font-mono">{{ missing.join(', ') }}</span>
            <button v-if="status?.can_request_changes" type="button" @click="requestMissing" :disabled="requesting"
              class="ml-auto h-7 px-2.5 rounded-lg border border-amber-700 bg-amber-950/40 hover:bg-amber-900/40 text-amber-100 flex items-center gap-1.5 disabled:opacity-50">
              <Loader2 v-if="requesting" class="w-3.5 h-3.5 animate-spin" /><MessageSquareWarning v-else class="w-3.5 h-3.5" />
              Ask the agent to cover them
            </button>
          </template>
          <template v-else><CheckCircle2 class="w-4 h-4" /> Every UAT and Sanity case has a walkthrough with screenshots.</template>
        </div>
        <div v-if="showScope" class="overflow-x-auto">
          <table class="w-full text-xs">
            <thead class="text-slate-500 text-left">
              <tr><th class="py-1.5 pr-3 font-medium">Case</th><th class="pr-3 font-medium">Type</th><th class="pr-3 font-medium">Priority</th>
                <th class="pr-3 font-medium">Application</th><th class="font-medium">Title</th></tr>
            </thead>
            <tbody class="divide-y divide-slate-800">
              <tr v-for="c in scope.cases" :key="c.id" :class="missing.includes(c.id) && 'bg-amber-950/20'">
                <td class="py-1.5 pr-3 font-mono text-slate-200 whitespace-nowrap">{{ c.id }}</td>
                <td class="pr-3"><span class="px-1.5 py-0.5 rounded border text-[10px] whitespace-nowrap" :class="KIND_BADGE[c.kind]">{{ c.kind }}</span></td>
                <td class="pr-3 text-slate-400">{{ c.priority || '—' }}</td>
                <td class="pr-3 text-slate-300 whitespace-nowrap">{{ appName(c.app) }}</td>
                <td class="text-slate-300">{{ c.title }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div v-if="status?.error" class="p-3 rounded-lg border text-xs flex items-start gap-2"
        :class="status.status === 'FAILED' ? 'border-rose-800/70 bg-rose-950/40 text-rose-200' : 'border-amber-800/60 bg-amber-950/30 text-amber-200'">
        <AlertCircle class="w-4 h-4 flex-shrink-0 mt-px" /> {{ status.error }}
      </div>
      <div v-else-if="missingEnv.length && status?.stage_ready" class="p-3 rounded-lg border border-slate-800 text-xs text-slate-400 flex items-start gap-2">
        <AlertCircle class="w-4 h-4 flex-shrink-0 mt-px" /> Set the environment URL for {{ missingEnv.join(', ') }} to capture its screenshots.
      </div>
      <div v-if="missingVars.length && status?.stage_ready" class="p-3 rounded-lg border border-amber-800/60 bg-amber-950/30 text-xs text-amber-200 flex flex-wrap items-center gap-2">
        <AlertCircle class="w-4 h-4 flex-shrink-0" />
        The plan needs {{ missingVars.length }} test-data value(s):
        <span class="font-mono">{{ missingVars.map(placeholderOf).join(', ') }}</span>
        <button type="button" class="ml-auto underline hover:text-amber-100" @click="showSettings = true">Fill them in</button>
      </div>

      <div v-if="loading" class="text-xs text-slate-500 animate-pulse">Loading UAT guide…</div>
      <div v-else-if="generating && !guide" class="p-6 rounded-xl border border-slate-800 text-center text-xs text-slate-400 flex items-center justify-center gap-2">
        <Loader2 class="w-4 h-4 animate-spin" /> Replaying the scenarios and capturing screenshots…
      </div>
      <div v-else-if="!guide" class="p-6 rounded-xl border border-dashed border-slate-800 text-center text-xs text-slate-500">
        {{ status?.stage_ready ? 'Set the environments and generate the guide.' : 'The guide is generated automatically when the uat_verification stage finishes.' }}
      </div>
      <section v-else class="p-5 rounded-xl bg-slate-900 border border-slate-800 uat-guide" :class="generating && 'opacity-60'">
        <MarkdownView :source="guide" :toc="true" />
      </section>
    </div>
  </div>
</template>

<style scoped>
.uat-guide :deep(img) {
  max-width: 100%;
  border: 1px solid rgb(51 65 85);
  border-radius: 0.5rem;
  margin: 0.5rem 0 0.75rem;
}
</style>
