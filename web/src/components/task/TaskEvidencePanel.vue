<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Task, TaskEvidenceDTO, EvidenceCaseDTO, EvidencePartDTO } from '../../types'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import {
  Film, RefreshCw, Loader2, CheckCircle2, XCircle, AlertTriangle, FileText, Route, Clapperboard, Copy, Download, Play,
  MinusCircle, Info, ChevronLeft, ChevronRight, X, GitCommit, Bug, Image as ImageIcon, ListChecks, Presentation,
} from 'lucide-vue-next'

// Test evidence of the Implementation stage, planned from the spec: end-to-end user flows, one
// case per acceptance case and the full suites as regression (recorded by the built-in
// test-evidence-video skill or a library skill). The spec coverage shows every acceptance case and
// the evidence that proves it; each case's steps, with their screenshots, read without the video,
// and a click on a step plays the video from there.
// readOnly: the task has moved past Implementation — its evidence can be watched, not re-recorded.
const props = defineProps<{ task: Task; readOnly?: boolean }>()
const emit = defineEmits<{ (e: 'open-routing'): void; (e: 'open-console'): void }>()
const toast = useToastStore()

const data = ref<TaskEvidenceDTO | null>(null)
const loading = ref(false)
const error = ref('')
const selected = ref('')
const filter = ref<'all' | 'failed'>('all')
const log = ref<{ path: string; content: string } | null>(null)
const logError = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.getTaskEvidence(props.task.id)
    const ids = items.value.map((c) => c.id)
    if (!ids.includes(selected.value)) selected.value = (items.value.find((c) => c.status !== 'passed') ?? items.value[0])?.id ?? ''
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
const manifest = computed(() => report.value?.manifest)
const cases = computed(() => manifest.value?.cases ?? [])
// Videos no case names (a library skill without evidence.json, or extra takes) are listed as their own items.
const items = computed<EvidenceCaseDTO[]>(() => {
  const named = new Set([...cases.value.map((c) => c.video), manifest.value?.summary?.video].filter(Boolean))
  const loose = (report.value?.videos ?? []).filter((v) => !named.has(v))
    .map((v) => ({ id: v, title: 'Video without case results', status: 'unknown', video: v }))
  return [...cases.value, ...loose]
})
const failedItems = computed(() => items.value.filter((c) => c.status !== 'passed'))
const shown = computed(() => (filter.value === 'failed' ? failedItems.value : items.value))
// The case list in the order a reviewer reads it: the journeys, the acceptance cases, then regression.
const kindOf = (c: EvidenceCaseDTO) => c.kind || (/^suite/i.test(c.id) ? 'suite' : /^flow/i.test(c.id) ? 'flow' : c.status === 'unknown' ? 'other' : 'acceptance')
const GROUPS = [
  { kind: 'flow', label: 'End-to-end flows' }, { kind: 'acceptance', label: 'Acceptance cases' },
  { kind: 'suite', label: 'Regression suites' }, { kind: 'other', label: 'Other videos' },
]
const groups = computed(() => GROUPS.map((g) => ({ ...g, cases: shown.value.filter((c) => kindOf(c) === g.kind) })).filter((g) => g.cases.length))

// ---- the presentation: the whole evidence in one video, with chapters ----
const summary = computed(() => {
  const sm = manifest.value?.summary
  return sm?.video && !sm.video_missing ? sm : null
})
const summaryEl = ref<HTMLVideoElement | null>(null)
const summaryNow = ref(0)
const activeChapter = computed(() => {
  let idx = -1
  summary.value?.chapters?.forEach((c, i) => { if (c.at <= summaryNow.value + 0.05) idx = i })
  return idx
})
function playChapter(at: number) {
  const v = summaryEl.value
  if (!v) return
  v.currentTime = at
  v.play().catch(() => {})
}
// The presentation is older than a case recorded after it: say so, so the reviewer re-records it.
const summaryStale = computed(() => {
  const at = summary.value?.recorded_at
  return !!at && cases.value.some((c) => c.recorded_at && c.recorded_at > at)
})

// ---- spec coverage: every acceptance case and the evidence that proves it ----
const spec = computed(() => data.value?.spec)
const specCases = computed(() => spec.value?.cases ?? [])
const gapsOnly = ref(false)
const specShown = computed(() => (gapsOnly.value ? specCases.value.filter((c) => c.status !== 'passed') : specCases.value))
const specGaps = computed(() => specCases.value.filter((c) => c.status !== 'passed').length)
const COVER: Record<string, { cls: string; label: string }> = {
  passed: { cls: 'bg-emerald-950 text-emerald-300 border-emerald-800', label: 'Proven' },
  failed: { cls: 'bg-rose-950 text-rose-300 border-rose-800', label: 'Failed' },
  blocked: { cls: 'bg-amber-950 text-amber-300 border-amber-800', label: 'Blocked' },
  missing: { cls: 'bg-slate-800 text-slate-300 border-slate-600 border-dashed', label: 'No evidence' },
}
function openCase(id: string) {
  filter.value = 'all'
  selected.value = id
  document.getElementById('evidence-case')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
const current = computed(() => items.value.find((c) => c.id === selected.value) ?? null)
const base = computed(() => data.value?.base_path ?? 'evidence/')
const url = (p?: string) => (p ? api.artifactURL(props.task.id, base.value + p) : '')

// Totals for the summary strip. Note cards are not checks, so they are not counted as steps.
const checked = (c: EvidenceCaseDTO) => (c.parts ?? []).filter((p) => p.status !== 'info')
const stats = computed(() => {
  const all = cases.value
  const steps = all.flatMap(checked)
  const suites = manifest.value?.suites ?? []
  return {
    cases: all.length, casesPassed: all.filter((c) => c.status === 'passed').length,
    steps: steps.length, stepsPassed: steps.filter((p) => p.status === 'passed').length,
    suites: suites.length, suitesPassed: suites.filter((s) => s.status === 'passed').length,
    videos: report.value?.videos.length ?? 0,
  }
})

const TONE: Record<string, { cls: string; label: string }> = {
  passed: { cls: 'bg-emerald-950 text-emerald-300 border-emerald-800', label: 'Passed' },
  failed: { cls: 'bg-rose-950 text-rose-300 border-rose-800', label: 'Failed' },
  blocked: { cls: 'bg-amber-950 text-amber-300 border-amber-800', label: 'Blocked' },
  unknown: { cls: 'bg-slate-800 text-slate-300 border-slate-700', label: 'No result' },
}
const tone = (s?: string) => TONE[s || 'unknown'] ?? TONE.unknown

// What kind of step it is, for the badge in the step list.
function kind(p: EvidencePartDTO): { label: string; cls: string } {
  if (p.action === 'run') return { label: 'Terminal', cls: 'text-sky-300 bg-sky-950/60 border-sky-900' }
  if (p.action === 'start' || p.action === 'stop') return { label: 'Process', cls: 'text-cyan-300 bg-cyan-950/60 border-cyan-900' }
  if (p.action === 'watch' || p.action === 'expect_log') return { label: 'Log check', cls: 'text-emerald-300 bg-emerald-950/60 border-emerald-900' }
  if (p.action === 'api') return { label: 'API', cls: 'text-indigo-300 bg-indigo-950/60 border-indigo-900' }
  if (p.action === 'log') return { label: 'Test log', cls: 'text-sky-300 bg-sky-950/60 border-sky-900' }
  if (p.action === 'card') return { label: 'Note', cls: 'text-slate-300 bg-slate-800 border-slate-700' }
  if (p.action === 'annotate') return { label: 'Look here', cls: 'text-purple-300 bg-purple-950/60 border-purple-900' }
  if (p.verified || p.action.startsWith('expect')) return { label: 'Check', cls: 'text-emerald-300 bg-emerald-950/60 border-emerald-900' }
  if (p.action === 'goto') return { label: 'Open', cls: 'text-amber-300 bg-amber-950/40 border-amber-900' }
  return { label: 'Action', cls: 'text-amber-300 bg-amber-950/40 border-amber-900' }
}

const clock = (s?: number) => {
  if (s == null) return ''
  const t = Math.max(0, Math.floor(s))
  return `${Math.floor(t / 60)}:${String(t % 60).padStart(2, '0')}`
}
function ago(iso?: string) {
  if (!iso) return ''
  const s = Math.round((Date.now() - new Date(iso).getTime()) / 1000)
  if (Number.isNaN(s)) return ''
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  return new Date(iso).toLocaleString()
}

// ---- video: a step plays from its time; the step on screen is highlighted while it plays ----
const videoEl = ref<HTMLVideoElement | null>(null)
const now = ref(0)
const speed = ref(1)
const SPEEDS = [0.5, 1, 1.5, 2]
const timed = computed(() => !!current.value?.parts?.some((p) => p.at != null))
const activeStep = computed(() => {
  const parts = current.value?.parts ?? []
  if (!timed.value || now.value <= 0) return -1
  if (current.value?.result_at != null && now.value >= current.value.result_at) return -1
  let idx = -1
  parts.forEach((p, i) => { if (p.at != null && p.at <= now.value + 0.05) idx = i })
  return idx
})
function seek(p: EvidencePartDTO) {
  const v = videoEl.value
  if (!v || p.at == null) return
  v.currentTime = p.at
  v.play().catch(() => {})
  v.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
}
watch(speed, (s) => { if (videoEl.value) videoEl.value.playbackRate = s })
function onLoaded() { if (videoEl.value) videoEl.value.playbackRate = speed.value }

// ---- screenshots: a lightbox with the step's caption, arrows to walk through the steps ----
const shots = computed(() => (current.value?.parts ?? []).map((p, i) => ({ p, i })).filter((x) => x.p.shot))
const lightbox = ref<number | null>(null) // index into shots
const lightShot = computed(() => (lightbox.value == null ? null : shots.value[lightbox.value] ?? null))
function openShot(i: number) { lightbox.value = shots.value.findIndex((x) => x.i === i) }
function step(d: number) {
  if (lightbox.value == null) return
  lightbox.value = (lightbox.value + d + shots.value.length) % shots.value.length
}
function onKey(e: KeyboardEvent) {
  if (lightbox.value == null) return
  if (e.key === 'Escape') lightbox.value = null
  else if (e.key === 'ArrowRight') step(1)
  else if (e.key === 'ArrowLeft') step(-1)
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

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
const logLines = computed(() => (log.value?.content ?? '').split('\n').map((l) => ({
  text: l,
  cls: /^#/.test(l) ? 'text-slate-500'
    : /\b(fail(ed|ure)?|error|panic)\b|✗/i.test(l) && !/\b0 (failed|failures?)\b/i.test(l) ? 'text-rose-300'
    : /\b(pass(ed)?|ok)\b|✓/i.test(l) ? 'text-emerald-300' : 'text-slate-300',
})))

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

// A Markdown summary of the evidence, to paste into a PR, a ticket or a review message.
async function copyReport() {
  const icon = (s?: string) => (s === 'passed' ? '✅ Passed' : s === 'failed' ? '❌ Failed' : s === 'blocked' ? '⚠️ Blocked' : '—')
  const cell = (t?: string | number) => String(t ?? '').replace(/\|/g, '\\|').replace(/\n/g, ' ')
  const lines = [`## Test evidence — ${props.task.id}${manifest.value?.title ? ` · ${manifest.value.title.replace(/^\S+ · /, '')}` : ''}`, '']
  lines.push(`${stats.value.casesPassed} of ${stats.value.cases} cases passed · ${stats.value.stepsPassed} of ${stats.value.steps} steps passed`, '')
  if (specCases.value.length) {
    const cov = (s: string) => (s === 'passed' ? '✅ Proven' : s === 'failed' ? '❌ Failed' : s === 'blocked' ? '⚠️ Blocked' : '⬜ No evidence')
    lines.push(`### Spec coverage — ${specCases.value.length - specGaps.value} of ${specCases.value.length} acceptance cases proven`, '',
      '| Acceptance case | What it proves | Evidence | Result |', '|---|---|---|---|')
    for (const c of specCases.value) lines.push(`| ${cell(c.id)} | ${cell(c.title)} | ${c.evidence.map((e) => e.id).join(', ') || '—'} | ${cov(c.status)} |`)
    lines.push('', '### Evidence')
  }
  lines.push('| Case | Kind | Title | Result | Steps | Video |', '|---|---|---|---|---|---|')
  for (const c of items.value) {
    const ps = checked(c)
    lines.push(`| ${cell(c.id)} | ${kindOf(c)} | ${cell(c.title)} | ${icon(c.status)} | ${ps.length ? `${ps.filter((p) => p.status === 'passed').length}/${ps.length}` : ''} | ${cell(c.video)} |`)
  }
  const suites = manifest.value?.suites ?? []
  if (suites.length) {
    lines.push('', '### Full test suites', '', '| Repository | Command | Passed | Failed | Result |', '|---|---|---|---|---|')
    for (const s of suites) lines.push(`| ${cell(s.repository)} | \`${cell(s.command)}\` | ${s.passed ?? '—'} | ${s.failed ?? '—'} | ${icon(s.status)} |`)
  }
  if (summary.value?.video) lines.push('', `▶ Presentation: ${summary.value.video}${summary.value.duration_s ? ` (${clock(summary.value.duration_s)})` : ''}`)
  const bad = cases.value.filter((c) => c.status !== 'passed')
  if (bad.length) {
    lines.push('', '### Not passing')
    for (const c of bad) {
      lines.push(`- **${c.id}** ${c.title ?? ''}`)
      for (const p of c.parts ?? []) if (p.status === 'failed') lines.push(`  - ✗ ${p.caption ?? p.action}${p.error ? ` — ${p.error}` : ''}`)
      if (c.notes) lines.push(`  - ${c.notes}`)
    }
  }
  try {
    await navigator.clipboard.writeText(lines.join('\n') + '\n')
    toast.success('Copied', 'Test evidence summary copied as Markdown')
  } catch {
    toast.error('Could not copy', 'The browser blocked clipboard access')
  }
}

watch(selected, () => { log.value = null; logError.value = ''; now.value = 0; lightbox.value = null })
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="max-w-7xl mx-auto p-5 space-y-4">
      <!-- Summary -->
      <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
        <div class="flex flex-wrap items-start gap-4">
          <div class="flex-1 min-w-[240px] space-y-1">
            <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
              <Film class="w-4 h-4 text-purple-400" /> Test evidence
              <span v-if="manifest?.updated_at" class="text-[11px] font-normal text-slate-500" :title="manifest.updated_at">· updated {{ ago(manifest.updated_at) }}</span>
            </h3>
            <p class="text-xs text-slate-400 leading-relaxed">
              Proof that every flow the change touches works end to end, as specified: recorded in the running system step by step —
              the UI, API calls, live terminals, and jobs or workers started with their logs watched — every action and check annotated.
              Unit-test suites are regression only. Recorded with
              <span class="font-mono text-slate-200">{{ data?.skill || 'test-evidence-video' }}</span>.
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button v-if="items.length" type="button" @click="copyReport" title="Copy a Markdown summary for a PR, ticket or review"
              class="h-8 px-3 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5">
              <Copy class="w-3.5 h-3.5" /> Copy summary
            </button>
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

        <dl v-if="stats.cases || specCases.length" class="grid grid-cols-2 sm:grid-cols-4 gap-2">
          <div class="rounded-lg border px-3 py-2" :class="!specCases.length ? 'border-slate-800 bg-slate-950/40' : specGaps ? 'border-amber-900 bg-amber-950/30' : 'border-emerald-900 bg-emerald-950/30'">
            <dt class="text-[10px] uppercase tracking-wider text-slate-500">Spec coverage</dt>
            <dd class="text-lg font-semibold" :class="!specCases.length ? 'text-slate-500' : specGaps ? 'text-amber-300' : 'text-emerald-300'">
              <template v-if="specCases.length">{{ specCases.length - specGaps }}<span class="text-sm text-slate-500"> / {{ specCases.length }} proven</span></template>
              <span v-else class="text-sm">no acceptance cases</span></dd>
          </div>
          <div class="rounded-lg border px-3 py-2" :class="stats.casesPassed === stats.cases ? 'border-emerald-900 bg-emerald-950/30' : 'border-rose-900 bg-rose-950/30'">
            <dt class="text-[10px] uppercase tracking-wider text-slate-500">Test cases</dt>
            <dd class="text-lg font-semibold" :class="stats.casesPassed === stats.cases ? 'text-emerald-300' : 'text-rose-300'">
              {{ stats.casesPassed }}<span class="text-sm text-slate-500"> / {{ stats.cases }} passed</span></dd>
          </div>
          <div class="rounded-lg border border-slate-800 bg-slate-950/40 px-3 py-2">
            <dt class="text-[10px] uppercase tracking-wider text-slate-500">Steps</dt>
            <dd class="text-lg font-semibold text-slate-100">{{ stats.stepsPassed }}<span class="text-sm text-slate-500"> / {{ stats.steps }} passed</span></dd>
          </div>
          <div class="rounded-lg border px-3 py-2" :class="!stats.suites ? 'border-slate-800 bg-slate-950/40' : stats.suitesPassed === stats.suites ? 'border-emerald-900 bg-emerald-950/30' : 'border-rose-900 bg-rose-950/30'">
            <dt class="text-[10px] uppercase tracking-wider text-slate-500">Full suites</dt>
            <dd class="text-lg font-semibold" :class="!stats.suites ? 'text-slate-500' : stats.suitesPassed === stats.suites ? 'text-emerald-300' : 'text-rose-300'">
              <template v-if="stats.suites">{{ stats.suitesPassed }}<span class="text-sm text-slate-500"> / {{ stats.suites }} passed</span></template>
              <span v-else class="text-sm">none recorded</span></dd>
          </div>
        </dl>

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

      <section v-if="summary" class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden">
        <div class="px-4 py-2.5 border-b border-slate-800 flex flex-wrap items-center gap-2">
          <h4 class="text-xs font-semibold text-slate-200 flex items-center gap-1.5"><Presentation class="w-3.5 h-3.5 text-purple-400" /> Evidence presentation</h4>
          <span class="text-[11px] text-slate-500">The whole evidence in one video<template v-if="summary.duration_s"> · {{ clock(summary.duration_s) }}</template>
            <template v-if="summary.recorded_at"> · recorded {{ ago(summary.recorded_at) }}</template></span>
          <span v-if="summaryStale" class="text-[11px] text-amber-300">· a case was re-recorded after it — record again to refresh it</span>
          <span class="flex-1" />
          <a :href="url(summary.video)" :download="summary.video" class="inline-flex items-center gap-1 text-[11px] text-sky-300 hover:underline"><Download class="w-3 h-3" /> Download</a>
        </div>
        <div class="grid lg:grid-cols-[1fr_16rem]">
          <div class="bg-black">
            <video ref="summaryEl" :key="summary.video" :src="url(summary.video)" :poster="url(summary.poster)" controls preload="metadata"
              class="w-full max-h-[62vh] mx-auto" aria-label="Test evidence presentation"
              @timeupdate="summaryNow = ($event.target as HTMLVideoElement).currentTime" />
          </div>
          <nav v-if="summary.chapters?.length" aria-label="Presentation chapters" class="border-t lg:border-t-0 lg:border-l border-slate-800 max-h-[62vh] overflow-y-auto">
            <button v-for="(c, i) in summary.chapters" :key="c.id + i" type="button" @click="playChapter(c.at)"
              class="w-full text-left px-3 py-2 border-b border-slate-800/60 flex items-start gap-2 text-xs border-l-2"
              :class="activeChapter === i ? 'bg-purple-950/30 border-l-purple-500 text-slate-100' : 'border-l-transparent text-slate-300 hover:bg-slate-800/50'">
              <span class="font-mono text-[11px] text-slate-500 w-9 flex-shrink-0">{{ clock(c.at) }}</span>
              <span class="min-w-0 leading-snug">{{ c.title }}</span>
            </button>
          </nav>
        </div>
      </section>
      <p v-else-if="cases.length && data?.skill === 'test-evidence-video'" class="text-xs text-slate-400 bg-slate-900 border border-dashed border-slate-700 rounded-xl px-4 py-3 flex items-center gap-2">
        <Presentation class="w-4 h-4 text-slate-500" /> No presentation video yet — it is recorded after the cases.
        <template v-if="!readOnly">Record again to make it.</template>
      </p>

      <section v-if="specCases.length" class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden">
        <div class="px-4 py-2.5 border-b border-slate-800 flex flex-wrap items-center gap-2">
          <h4 class="text-xs font-semibold text-slate-200 flex items-center gap-1.5"><ListChecks class="w-3.5 h-3.5 text-purple-400" /> Spec coverage</h4>
          <span class="text-[11px] text-slate-500">
            {{ specCases.length }} acceptance case{{ specCases.length === 1 ? '' : 's' }}<template v-if="spec?.source"> from {{ spec.source }}</template>
            <template v-if="spec?.docs?.length"> · spec: <span class="font-mono">{{ spec.docs.join(', ') }}</span></template>
          </span>
          <span class="flex-1" />
          <label v-if="specGaps" class="flex items-center gap-1.5 text-[11px] text-slate-300 cursor-pointer">
            <input v-model="gapsOnly" type="checkbox" class="accent-purple-500" /> Only not proven ({{ specGaps }})
          </label>
        </div>
        <div class="overflow-x-auto max-h-80 overflow-y-auto">
          <table class="w-full text-xs">
            <thead class="text-[10px] uppercase tracking-wider text-slate-500 sticky top-0 bg-slate-900">
              <tr><th class="text-left px-4 py-2">Case</th><th class="text-left px-4 py-2">What it proves</th>
                <th class="text-left px-4 py-2">Evidence</th><th class="text-left px-4 py-2">Result</th></tr>
            </thead>
            <tbody>
              <tr v-for="c in specShown" :key="c.id" class="border-t border-slate-800 align-top">
                <td class="px-4 py-2 whitespace-nowrap">
                  <span class="font-mono text-slate-200">{{ c.id }}</span>
                  <span v-for="t in [c.priority, c.platform, c.kind].filter(Boolean)" :key="t" class="ml-1 px-1 rounded bg-slate-800 text-[10px] text-slate-400">{{ t }}</span>
                </td>
                <td class="px-4 py-2 text-slate-300">{{ c.title }}</td>
                <td class="px-4 py-2">
                  <span v-if="!c.evidence.length" class="text-slate-500">—</span>
                  <button v-for="e in c.evidence" :key="e.id" type="button" @click="openCase(e.id)"
                    class="mr-1 mb-1 inline-flex items-center gap-1 px-1.5 py-0.5 rounded border font-mono text-[11px] hover:brightness-125" :class="tone(e.status).cls">
                    <Play class="w-2.5 h-2.5" />{{ e.id }}</button>
                </td>
                <td class="px-4 py-2"><span class="px-1.5 py-0.5 rounded border text-[11px] whitespace-nowrap" :class="(COVER[c.status] ?? COVER.missing).cls">{{ (COVER[c.status] ?? COVER.missing).label }}</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="data && !items.length" class="p-8 rounded-xl bg-slate-900 border border-dashed border-slate-700 text-center space-y-1">
        <Film class="w-6 h-6 text-slate-600 mx-auto" />
        <p class="text-sm text-slate-300">No test evidence yet</p>
        <p class="text-xs text-slate-500">The Implementation stage records it when it runs.
          <template v-if="!blocked">Implementation already done? <strong class="text-slate-300">Record evidence</strong> tests it and records the
            videos now, without changing the stage's output.</template>
          <template v-else>{{ blocked }}.</template></p>
      </section>

      <div v-if="items.length" class="grid gap-4 lg:grid-cols-[17rem_1fr]">
        <!-- Cases -->
        <nav class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden self-start lg:sticky lg:top-0" aria-label="Test cases">
          <div v-if="failedItems.length" class="flex gap-1 p-1.5 border-b border-slate-800" role="tablist" aria-label="Filter test cases">
            <button v-for="f in (['all', 'failed'] as const)" :key="f" type="button" role="tab" :aria-selected="filter === f" @click="filter = f"
              class="flex-1 h-7 rounded-md text-[11px] font-semibold"
              :class="filter === f ? 'bg-slate-700 text-slate-100' : 'text-slate-400 hover:text-slate-200'">
              {{ f === 'all' ? `All ${items.length}` : `Not passing ${failedItems.length}` }}
            </button>
          </div>
          <template v-for="g in groups" :key="g.kind">
          <div class="px-3 pt-2.5 pb-1 text-[10px] font-semibold uppercase tracking-wider text-slate-500 border-b border-slate-800/60">{{ g.label }} · {{ g.cases.length }}</div>
          <button v-for="c in g.cases" :key="c.id" type="button" @click="selected = c.id" :aria-current="selected === c.id"
            class="w-full text-left px-3 py-2.5 border-b border-slate-800 last:border-b-0 flex items-start gap-2 border-l-2"
            :class="selected === c.id ? 'bg-slate-800 border-l-purple-500' : 'hover:bg-slate-800/50 border-l-transparent'">
            <CheckCircle2 v-if="c.status === 'passed'" class="w-4 h-4 mt-0.5 text-emerald-400 flex-shrink-0" />
            <XCircle v-else-if="c.status === 'failed'" class="w-4 h-4 mt-0.5 text-rose-400 flex-shrink-0" />
            <AlertTriangle v-else class="w-4 h-4 mt-0.5 text-amber-400 flex-shrink-0" />
            <span class="min-w-0 flex-1">
              <span class="block text-[11px] font-mono text-slate-400 truncate">{{ c.id }}</span>
              <span class="block text-xs text-slate-200 leading-snug line-clamp-2">{{ c.title || c.video }}</span>
              <span v-if="c.parts?.length" class="block text-[10px] text-slate-500 mt-0.5">
                {{ checked(c).filter((p) => p.status === 'passed').length }}/{{ checked(c).length }} steps<template v-if="c.duration_s"> · {{ clock(c.duration_s) }}</template>
              </span>
            </span>
          </button>
          </template>
        </nav>

        <!-- Selected case -->
        <article v-if="current" id="evidence-case" class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden min-w-0">
          <header class="px-4 py-3 border-b border-slate-800 space-y-2">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-xs font-mono text-slate-400">{{ current.id }}</span>
              <span class="px-1.5 py-px rounded bg-slate-800 text-[10px] text-slate-400">{{ GROUPS.find((g) => g.kind === kindOf(current!))?.label.replace(/s$/, '') }}</span>
              <h4 class="text-sm font-semibold text-slate-100 flex-1 min-w-0">{{ current.title }}</h4>
              <span class="px-2 py-0.5 rounded border text-[11px] font-semibold" :class="tone(current.status).cls">{{ tone(current.status).label }}</span>
            </div>
            <dl v-if="current.given || current.expected || current.covers?.length" class="grid grid-cols-[5.5rem_1fr] gap-x-3 gap-y-1 text-xs">
              <template v-if="current.given"><dt class="text-slate-500 uppercase tracking-wider text-[10px] pt-0.5">Given</dt><dd class="text-slate-300">{{ current.given }}</dd></template>
              <template v-if="current.expected"><dt class="text-slate-500 uppercase tracking-wider text-[10px] pt-0.5">Expected</dt><dd class="text-slate-200">{{ current.expected }}</dd></template>
              <template v-if="current.covers?.length"><dt class="text-slate-500 uppercase tracking-wider text-[10px] pt-0.5">Covers</dt>
                <dd><span v-for="c in current.covers" :key="c" class="mr-1 px-1.5 py-0.5 rounded bg-slate-800 font-mono text-slate-300">{{ c }}</span></dd></template>
            </dl>
            <p v-if="current.notes" class="text-xs text-amber-200/90 bg-amber-950/30 border border-amber-900/60 rounded-lg px-3 py-2 whitespace-pre-wrap">{{ current.notes }}</p>
          </header>

          <div class="bg-black">
            <video v-if="current.video && !current.video_missing" ref="videoEl" :key="current.video" :src="url(current.video)" :poster="url(current.poster)"
              controls preload="metadata" class="w-full max-h-[62vh] mx-auto" :aria-label="`Test evidence video for ${current.id}`"
              @timeupdate="now = ($event.target as HTMLVideoElement).currentTime" @loadedmetadata="onLoaded" />
            <p v-else class="px-4 py-10 text-center text-xs text-slate-400">
              {{ current.video ? `evidence.json names ${current.video}, but it is not in the evidence folder.` : 'This case has no video.' }}
            </p>
          </div>
          <div v-if="current.video && !current.video_missing" class="px-4 py-2 border-b border-slate-800 flex flex-wrap items-center gap-3 text-[11px] text-slate-400">
            <span class="flex items-center gap-1" role="group" aria-label="Playback speed">Speed
              <button v-for="s in SPEEDS" :key="s" type="button" @click="speed = s" :aria-pressed="speed === s"
                class="px-1.5 py-0.5 rounded" :class="speed === s ? 'bg-slate-700 text-slate-100' : 'hover:text-slate-200'">{{ s }}×</button>
            </span>
            <span v-if="timed" class="text-slate-500">Click a step to play it.</span>
            <span class="flex-1" />
            <span v-if="current.recorded_at" :title="current.recorded_at">Recorded {{ ago(current.recorded_at) }}</span>
            <a :href="url(current.video)" :download="current.video" class="inline-flex items-center gap-1 text-sky-300 hover:underline">
              <Download class="w-3 h-3" /> Download</a>
          </div>

          <!-- Steps -->
          <div class="p-4 space-y-3">
            <h5 v-if="current.parts?.length" class="text-[11px] font-semibold uppercase tracking-wider text-slate-500">
              Steps · {{ checked(current).filter((p) => p.status === 'passed').length }} of {{ checked(current).length }} passed
            </h5>
            <ol v-if="current.parts?.length" class="space-y-2">
              <li v-for="(p, i) in current.parts" :key="i"
                class="rounded-lg border transition-colors"
                :class="[activeStep === i ? 'border-purple-500 bg-purple-950/20' : p.status === 'failed' ? 'border-rose-900 bg-rose-950/20' : 'border-slate-800 bg-slate-950/30']">
                <div class="flex gap-3 p-2.5">
                  <div class="flex flex-col items-center gap-1 pt-0.5">
                    <CheckCircle2 v-if="p.status === 'passed'" class="w-4 h-4 text-emerald-400" aria-label="passed" />
                    <XCircle v-else-if="p.status === 'failed'" class="w-4 h-4 text-rose-400" aria-label="failed" />
                    <MinusCircle v-else-if="p.status === 'skipped'" class="w-4 h-4 text-slate-500" aria-label="not run" />
                    <Info v-else class="w-4 h-4 text-slate-400" aria-label="note" />
                    <span class="text-[10px] font-mono text-slate-500">{{ i + 1 }}</span>
                  </div>
                  <div class="flex-1 min-w-0 space-y-1 text-xs">
                    <div class="flex flex-wrap items-center gap-1.5">
                      <span class="px-1.5 py-px rounded border text-[10px] font-semibold" :class="kind(p).cls">{{ kind(p).label }}</span>
                      <span class="text-slate-100 font-medium" :class="p.status === 'skipped' ? 'text-slate-500' : ''">{{ p.caption || p.action }}</span>
                    </div>
                    <p v-if="p.note" class="text-slate-400 leading-relaxed">{{ p.note }}</p>
                    <div v-if="p.command" class="font-mono text-[11px] text-slate-500 break-all">$ {{ p.command }}</div>
                    <div v-if="p.method" class="font-mono text-[11px] text-slate-500 break-all">{{ p.method }} {{ p.url }}</div>
                    <div v-else-if="p.url && !p.command && p.url !== current.parts?.[i - 1]?.url" class="font-mono text-[11px] text-slate-500 break-all">{{ p.url }}</div>
                    <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px]">
                      <span v-if="p.action === 'run'" class="font-mono" :class="p.exit_code === 0 ? 'text-emerald-300' : 'text-rose-300'">exit {{ p.exit_code ?? '—' }}</span>
                      <span v-if="p.summary" class="text-slate-300">{{ p.summary }}</span>
                      <span v-if="p.seconds" class="text-slate-500">{{ clock(p.seconds) }} run</span>
                      <button v-if="p.log" type="button" @click="openLog(p.log)" class="inline-flex items-center gap-1 text-sky-300 hover:underline">
                        <FileText class="w-3 h-3" /> {{ p.action === 'api' ? 'request & response' : ['start', 'watch', 'expect_log', 'stop'].includes(p.action) ? 'process log' : 'full log' }}</button>
                      <button v-if="p.at != null && current.video && !current.video_missing" type="button" @click="seek(p)"
                        class="inline-flex items-center gap-1 text-purple-300 hover:underline" :aria-label="`Play step ${i + 1} in the video`">
                        <Play class="w-3 h-3" /> {{ clock(p.at) }}</button>
                    </div>
                    <p v-if="p.error" class="text-rose-300" :class="p.status === 'skipped' ? 'text-slate-500' : ''">{{ p.error }}</p>
                  </div>
                  <button v-if="p.shot" type="button" @click="openShot(i)" :aria-label="`Screenshot of step ${i + 1}`"
                    class="flex-shrink-0 w-36 sm:w-44 aspect-video rounded-md overflow-hidden border border-slate-700 hover:border-purple-400 bg-slate-950">
                    <img :src="url(p.shot)" :alt="`Step ${i + 1}: ${p.caption || p.action}`" loading="lazy" class="w-full h-full object-cover object-top" />
                  </button>
                </div>
              </li>
            </ol>

            <div v-if="current.environment?.repos?.length || current.environment?.browser" class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-slate-500">
              <span v-for="r in current.environment?.repos ?? []" :key="r.name" class="inline-flex items-center gap-1 font-mono">
                <GitCommit class="w-3 h-3" />{{ r.name }}@{{ r.commit }}<span v-if="r.uncommitted" class="text-amber-400" title="Recorded with uncommitted changes">+changes</span>
                <span v-if="r.branch" class="text-slate-600">({{ r.branch }})</span>
              </span>
              <span v-if="current.environment?.browser">{{ current.environment.browser }} · {{ current.environment.viewport }}</span>
            </div>

            <details v-if="current.issues?.length" class="rounded-lg border border-amber-900/60 bg-amber-950/20">
              <summary class="px-3 py-2 text-xs text-amber-200 cursor-pointer flex items-center gap-1.5">
                <Bug class="w-3.5 h-3.5" /> {{ current.issues.length }} browser error{{ current.issues.length === 1 ? '' : 's' }} during this case
              </summary>
              <ul class="px-3 pb-2 space-y-1 font-mono text-[11px] text-amber-100/80">
                <li v-for="(m, i) in current.issues" :key="i" class="break-all">{{ m }}</li>
              </ul>
            </details>

            <p v-if="logError" class="text-xs text-rose-300">{{ logError }}</p>
            <div v-if="log" class="rounded-lg border border-slate-800 bg-slate-950">
              <div class="px-3 py-1.5 border-b border-slate-800 flex items-center gap-3 text-[11px] font-mono text-slate-400">
                <span class="flex-1 truncate">{{ log.path }}</span>
                <a :href="url(log.path)" :download="log.path.split('/').pop()" class="text-sky-300 hover:underline">download</a>
                <button type="button" class="text-slate-500 hover:text-slate-200" @click="log = null">close</button>
              </div>
              <pre class="max-h-96 overflow-auto p-3 text-[11px] leading-relaxed whitespace-pre-wrap break-words"><span v-for="(l, i) in logLines" :key="i" :class="l.cls">{{ l.text }}
</span></pre>
            </div>
          </div>
        </article>
      </div>

      <section v-if="manifest?.suites?.length" class="rounded-xl bg-slate-900 border border-slate-800 overflow-hidden">
        <h4 class="px-4 py-2.5 border-b border-slate-800 text-xs font-semibold text-slate-200">Full test suites</h4>
        <div class="overflow-x-auto">
          <table class="w-full text-xs">
            <thead class="text-[10px] uppercase tracking-wider text-slate-500">
              <tr><th class="text-left px-4 py-2">Repository</th><th class="text-left px-4 py-2">Command</th>
                <th class="text-right px-4 py-2">Passed</th><th class="text-right px-4 py-2">Failed</th><th class="text-right px-4 py-2">Skipped</th>
                <th class="text-right px-4 py-2">Time</th><th class="text-left px-4 py-2">Result</th><th class="px-4 py-2"><span class="sr-only">Evidence</span></th></tr>
            </thead>
            <tbody>
              <tr v-for="s in manifest.suites" :key="s.repository + s.command" class="border-t border-slate-800">
                <td class="px-4 py-2 font-mono text-slate-200">{{ s.repository }}</td>
                <td class="px-4 py-2 font-mono text-slate-400 break-all">{{ s.command }}</td>
                <td class="px-4 py-2 text-right text-slate-300">{{ s.passed ?? '—' }}</td>
                <td class="px-4 py-2 text-right" :class="s.failed ? 'text-rose-300 font-semibold' : 'text-slate-300'">{{ s.failed ?? '—' }}</td>
                <td class="px-4 py-2 text-right text-slate-400">{{ s.skipped ?? '—' }}</td>
                <td class="px-4 py-2 text-right text-slate-400">{{ s.seconds ? clock(s.seconds) : '—' }}</td>
                <td class="px-4 py-2"><span class="px-1.5 py-0.5 rounded border text-[11px]" :class="tone(s.status).cls">{{ tone(s.status).label }}</span></td>
                <td class="px-4 py-2 text-right whitespace-nowrap">
                  <button v-if="s.case && items.some((c) => c.id === s.case)" type="button" @click="selected = s.case!; filter = 'all'"
                    class="text-purple-300 hover:underline">watch</button>
                  <button v-if="s.log" type="button" @click="selected = s.case || selected; openLog(s.log)" class="ml-3 text-sky-300 hover:underline">log</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <!-- Step screenshot -->
    <div v-if="lightShot" class="fixed inset-0 z-50 bg-black/85 flex items-center justify-center p-4" @click.self="lightbox = null">
      <div role="dialog" aria-modal="true" :aria-label="`Screenshot of step ${lightShot.i + 1}`" class="w-full max-w-6xl space-y-2">
        <div class="flex items-center gap-3 text-xs text-slate-300">
          <ImageIcon class="w-4 h-4 text-purple-300" />
          <span class="font-mono text-slate-500">{{ current?.id }} · step {{ lightShot.i + 1 }} of {{ current?.parts?.length }}</span>
          <span class="flex-1 truncate text-slate-100 font-medium">{{ lightShot.p.caption }}</span>
          <button v-if="lightShot.p.at != null && current?.video" type="button" @click="seek(lightShot.p); lightbox = null"
            class="inline-flex items-center gap-1 text-purple-300 hover:underline"><Play class="w-3 h-3" /> play from here</button>
          <button type="button" @click="lightbox = null" aria-label="Close" class="p-1 rounded hover:bg-slate-800"><X class="w-4 h-4" /></button>
        </div>
        <div class="relative">
          <img :src="url(lightShot.p.shot)" :alt="lightShot.p.caption || ''" class="w-full max-h-[80vh] object-contain rounded-lg border border-slate-700 bg-slate-950" />
          <button v-if="shots.length > 1" type="button" @click="step(-1)" aria-label="Previous step"
            class="absolute left-2 top-1/2 -translate-y-1/2 p-2 rounded-full bg-slate-900/80 hover:bg-slate-800 text-slate-100"><ChevronLeft class="w-5 h-5" /></button>
          <button v-if="shots.length > 1" type="button" @click="step(1)" aria-label="Next step"
            class="absolute right-2 top-1/2 -translate-y-1/2 p-2 rounded-full bg-slate-900/80 hover:bg-slate-800 text-slate-100"><ChevronRight class="w-5 h-5" /></button>
        </div>
        <p v-if="lightShot.p.note || lightShot.p.error" class="text-xs" :class="lightShot.p.error ? 'text-rose-300' : 'text-slate-400'">{{ lightShot.p.error || lightShot.p.note }}</p>
        <p class="text-[11px] text-slate-500">← → to walk through the steps · Esc to close</p>
      </div>
    </div>
  </div>
</template>
