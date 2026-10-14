<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Task, TaskEvidenceDTO, EvidenceCaseDTO } from '../../types'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { Film, RefreshCw, Loader2, CheckCircle2, XCircle, AlertTriangle, FileText, Route, Clapperboard } from 'lucide-vue-next'

// Test evidence of the Implementation stage: a video per test case (recorded by the built-in
// test-evidence-video skill or a library skill), the full suite runs, and each case's parts.
// readOnly: the task has moved past Implementation — its evidence can be watched, not re-recorded.
const props = defineProps<{ task: Task; readOnly?: boolean }>()
const emit = defineEmits<{ (e: 'open-routing'): void; (e: 'open-console'): void }>()
const toast = useToastStore()

const data = ref<TaskEvidenceDTO | null>(null)
const loading = ref(false)
const error = ref('')
const selected = ref('')
const log = ref<{ path: string; content: string } | null>(null)
const logError = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.getTaskEvidence(props.task.id)
    const ids = items.value.map((c) => c.id)
    if (!ids.includes(selected.value)) selected.value = ids[0] ?? ''
  } catch (e: any) {
    error.value = e?.message || 'Failed to load test evidence'
  } finally {
    loading.value = false
  }
}
onMounted(load)
// The stage reports its evidence when it finishes; reload then.
watch(() => [props.task.metadata?.evidence_status, props.task.metadata?.evidence_videos, props.task.state], load)

const report = computed(() => data.value?.report)
const cases = computed(() => report.value?.manifest?.cases ?? [])
// Videos no case names (a library skill without evidence.json, or extra takes) are listed as their own items.
const items = computed<EvidenceCaseDTO[]>(() => {
  const named = new Set(cases.value.map((c) => c.video).filter(Boolean))
  const loose = (report.value?.videos ?? []).filter((v) => !named.has(v))
    .map((v) => ({ id: v, title: 'Video without case results', status: 'unknown', video: v }))
  return [...cases.value, ...loose]
})
const current = computed(() => items.value.find((c) => c.id === selected.value) ?? null)
const base = computed(() => data.value?.base_path ?? 'evidence/')
const url = (p?: string) => (p ? api.artifactURL(props.task.id, base.value + p) : '')

const TONE: Record<string, { cls: string; label: string }> = {
  passed: { cls: 'bg-emerald-950 text-emerald-300 border-emerald-800', label: 'Passed' },
  failed: { cls: 'bg-rose-950 text-rose-300 border-rose-800', label: 'Failed' },
  blocked: { cls: 'bg-amber-950 text-amber-300 border-amber-800', label: 'Blocked' },
  unknown: { cls: 'bg-slate-800 text-slate-300 border-slate-700', label: 'No result' },
}
const tone = (s?: string) => TONE[s || 'unknown'] ?? TONE.unknown

async function openLog(path: string) {
  logError.value = ''
  try {
    const a = await api.getArtifact(props.task.id, base.value + path)
    log.value = { path, content: a.content }
  } catch (e: any) {
    log.value = null
    logError.value = e?.message || `Could not open ${path}`
  }
}
// Record evidence runs an agent turn; its commands ask for approval in the Console, so go there.
const recording = ref(false)
const blocked = computed(() => data.value?.record_blocked ?? '')
async function record() {
  recording.value = true
  try {
    await api.recordTaskEvidence(props.task.id)
    toast.info('Recording test evidence', 'The agent tests the change and records the videos. Approve its commands in the Console.')
    emit('open-console')
  } catch (e: any) {
    toast.error('Could not record evidence', e?.message || '')
  } finally {
    recording.value = false
  }
}

watch(selected, () => { log.value = null; logError.value = '' })
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="max-w-6xl mx-auto p-5 space-y-4">
      <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
        <div class="flex flex-wrap items-start gap-4">
          <div class="flex-1 min-w-[240px] space-y-1">
            <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
              <Film class="w-4 h-4 text-purple-400" /> Test evidence
            </h3>
            <p class="text-xs text-slate-400 leading-relaxed">
              After implementing, the agent runs the test suites completely and records a video per test case — the test commands with their
              real output and exit code, and walkthroughs of the running app. Recorded with
              <span class="font-mono text-slate-200">{{ data?.skill || 'test-evidence-video' }}</span>.
            </p>
          </div>
          <div class="flex items-center gap-2">
            <span v-if="report && report.videos.length" class="text-xs text-slate-400">
              <span class="text-emerald-300 font-semibold">{{ report.passed }}</span> passed ·
              <span :class="report.failed ? 'text-rose-300 font-semibold' : ''">{{ report.failed }}</span> failed or blocked
            </span>
            <template v-if="!readOnly">
              <button type="button" @click="emit('open-routing')"
                class="h-8 px-3 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5">
                <Route class="w-3.5 h-3.5" /> Evidence settings
              </button>
              <button type="button" @click="record" :disabled="!!blocked || recording" :title="blocked || 'Test the implementation and record a video per test case'"
                class="h-8 px-3 rounded-lg bg-purple-600 hover:bg-purple-500 text-white text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed">
                <Loader2 v-if="recording" class="w-3.5 h-3.5 animate-spin" /><Clapperboard v-else class="w-3.5 h-3.5" />
                {{ items.length ? 'Record again' : 'Record evidence' }}
              </button>
            </template>
            <button type="button" @click="load" :disabled="loading" aria-label="Refresh test evidence"
              class="h-8 px-3 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5 disabled:opacity-50">
              <Loader2 v-if="loading" class="w-3.5 h-3.5 animate-spin" /><RefreshCw v-else class="w-3.5 h-3.5" /> Refresh
            </button>
          </div>
        </div>
        <p v-if="error" class="text-xs text-rose-300">{{ error }}</p>
        <p v-if="readOnly" class="text-xs text-slate-400 bg-slate-800/60 border border-slate-700 rounded-lg px-3 py-2">
          Recorded during the Implementation stage. The task has moved on, so this evidence is read-only.
        </p>
        <p v-if="data && !data.enabled && !readOnly" class="text-xs text-amber-200 bg-amber-950/40 border border-amber-900 rounded-lg px-3 py-2">
          Test evidence is off for this task. Turn it on for the Implementation stage under Evidence settings.
        </p>
        <p v-if="report?.manifest_error" class="text-xs text-amber-200 bg-amber-950/40 border border-amber-900 rounded-lg px-3 py-2">
          {{ report.manifest_error }} — the videos are listed without case results.
        </p>
      </section>

      <section v-if="data && !items.length" class="p-8 rounded-xl bg-slate-900 border border-dashed border-slate-700 text-center space-y-1">
        <Film class="w-6 h-6 text-slate-600 mx-auto" />
        <p class="text-sm text-slate-300">No test evidence yet</p>
        <p class="text-xs text-slate-500">The Implementation stage records it when it runs.
          <template v-if="!blocked">Implementation already done? <strong class="text-slate-300">Record evidence</strong> tests it and records the
            videos now, without changing the stage's output.</template>
          <template v-else>{{ blocked }}.</template></p>
      </section>

      <div v-if="items.length" class="grid gap-4 lg:grid-cols-[18rem_1fr]">
        <nav class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden self-start" aria-label="Test cases">
          <button v-for="c in items" :key="c.id" type="button" @click="selected = c.id" :aria-current="selected === c.id"
            class="w-full text-left px-3 py-2.5 border-b border-slate-800 last:border-b-0 flex items-start gap-2"
            :class="selected === c.id ? 'bg-slate-800' : 'hover:bg-slate-800/50'">
            <CheckCircle2 v-if="c.status === 'passed'" class="w-4 h-4 mt-0.5 text-emerald-400 flex-shrink-0" />
            <XCircle v-else-if="c.status === 'failed'" class="w-4 h-4 mt-0.5 text-rose-400 flex-shrink-0" />
            <AlertTriangle v-else class="w-4 h-4 mt-0.5 text-amber-400 flex-shrink-0" />
            <span class="min-w-0">
              <span class="block text-[11px] font-mono text-slate-400 truncate">{{ c.id }}</span>
              <span class="block text-xs text-slate-200 leading-snug line-clamp-2">{{ c.title || c.video }}</span>
            </span>
          </button>
        </nav>

        <article v-if="current" class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden min-w-0">
          <div class="px-4 py-3 border-b border-slate-800 flex flex-wrap items-center gap-2">
            <span class="text-xs font-mono text-slate-400">{{ current.id }}</span>
            <span class="text-sm font-semibold text-slate-100 flex-1 min-w-0 truncate">{{ current.title }}</span>
            <span class="px-2 py-0.5 rounded border text-[11px] font-semibold" :class="tone(current.status).cls">{{ tone(current.status).label }}</span>
          </div>
          <div class="bg-black">
            <video v-if="current.video && !current.video_missing" :key="current.video" :src="url(current.video)" :poster="url(current.poster)"
              controls preload="metadata" class="w-full max-h-[60vh] mx-auto" :aria-label="`Test evidence video for ${current.id}`" />
            <p v-else class="px-4 py-10 text-center text-xs text-slate-400">
              {{ current.video ? `evidence.json names ${current.video}, but it is not in the evidence folder.` : 'This case has no video.' }}
            </p>
          </div>
          <div class="p-4 space-y-3 text-xs">
            <p v-if="current.expected" class="text-slate-300"><span class="text-slate-500">Expected:</span> {{ current.expected }}</p>
            <p v-if="current.covers?.length" class="text-slate-400">Covers
              <span v-for="c in current.covers" :key="c" class="ml-1 px-1.5 py-0.5 rounded bg-slate-800 font-mono text-slate-300">{{ c }}</span>
            </p>
            <p v-if="current.notes" class="text-slate-300 whitespace-pre-wrap">{{ current.notes }}</p>
            <ol v-if="current.parts?.length" class="space-y-1.5">
              <li v-for="(p, i) in current.parts" :key="i" class="flex items-start gap-2">
                <CheckCircle2 v-if="p.status === 'passed'" class="w-3.5 h-3.5 mt-0.5 text-emerald-400 flex-shrink-0" />
                <XCircle v-else class="w-3.5 h-3.5 mt-0.5 text-rose-400 flex-shrink-0" />
                <div class="min-w-0">
                  <span class="text-slate-200">{{ i + 1 }}. {{ p.caption || p.action }}</span>
                  <span v-if="p.action === 'run'" class="ml-1 text-slate-500 font-mono">
                    exit {{ p.exit_code ?? '—' }}<template v-if="p.seconds"> · {{ p.seconds }}s</template>
                  </span>
                  <button v-if="p.log" type="button" @click="openLog(p.log)"
                    class="ml-2 inline-flex items-center gap-1 text-sky-300 hover:underline"><FileText class="w-3 h-3" /> log</button>
                  <div v-if="p.command" class="font-mono text-[11px] text-slate-500 break-all">$ {{ p.command }}</div>
                  <div v-if="p.error" class="text-rose-300">{{ p.error }}</div>
                </div>
              </li>
            </ol>
            <p v-if="logError" class="text-rose-300">{{ logError }}</p>
            <div v-if="log" class="rounded-lg border border-slate-800 bg-slate-950">
              <div class="px-3 py-1.5 border-b border-slate-800 flex items-center justify-between text-[11px] font-mono text-slate-400">
                {{ log.path }}<button type="button" class="text-slate-500 hover:text-slate-200" @click="log = null">close</button>
              </div>
              <pre class="max-h-80 overflow-auto p-3 text-[11px] leading-relaxed text-slate-300 whitespace-pre-wrap break-words">{{ log.content }}</pre>
            </div>
          </div>
        </article>
      </div>

      <section v-if="report?.manifest?.suites?.length" class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden">
        <h4 class="px-4 py-2.5 border-b border-slate-800 text-xs font-semibold text-slate-200">Full test suites</h4>
        <table class="w-full text-xs">
          <thead class="text-[10px] uppercase tracking-wider text-slate-500">
            <tr><th class="text-left px-4 py-2">Repository</th><th class="text-left px-4 py-2">Command</th>
              <th class="text-right px-4 py-2">Passed</th><th class="text-right px-4 py-2">Failed</th><th class="text-left px-4 py-2">Result</th></tr>
          </thead>
          <tbody>
            <tr v-for="s in report.manifest.suites" :key="s.repository + s.command" class="border-t border-slate-800">
              <td class="px-4 py-2 font-mono text-slate-200">{{ s.repository }}</td>
              <td class="px-4 py-2 font-mono text-slate-400 break-all">{{ s.command }}</td>
              <td class="px-4 py-2 text-right text-slate-300">{{ s.passed ?? '—' }}</td>
              <td class="px-4 py-2 text-right" :class="s.failed ? 'text-rose-300' : 'text-slate-300'">{{ s.failed ?? '—' }}</td>
              <td class="px-4 py-2"><span class="px-1.5 py-0.5 rounded border text-[11px]" :class="tone(s.status).cls">{{ tone(s.status).label }}</span></td>
            </tr>
          </tbody>
        </table>
      </section>
    </div>
  </div>
</template>
