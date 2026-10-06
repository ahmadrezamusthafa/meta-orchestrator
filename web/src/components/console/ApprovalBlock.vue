<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import type { ConsoleEntry } from '../../types'
import { api } from '../../services/api'
import { ShieldAlert, ShieldCheck, ShieldX, Clock, ClipboardList, MessageCircleQuestion } from 'lucide-vue-next'
import MarkdownView from '../common/MarkdownView.vue'

const props = defineProps<{ entry: ConsoleEntry }>()

const a = computed(() => props.entry.approval!)
const kind = computed(() => a.value.kind || 'tool')
const pending = computed(() => a.value.decision === 'pending')
const allowed = computed(() => ['allowed', 'always', 'all', 'auto'].includes(a.value.decision))
const sending = ref(false)
const error = ref('')
const denying = ref(false)
const note = ref('')
const allowBtn = ref<HTMLButtonElement | null>(null)
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined

const remaining = computed(() => {
  const ms = new Date(a.value.expires_at).getTime() - now.value
  if (ms <= 0) return 'expiring'
  const m = Math.floor(ms / 60000)
  return m >= 1 ? `${m} min left` : `${Math.ceil(ms / 1000)}s left`
})

const TITLE = { tool: 'Permission needed', plan: 'Plan ready for your approval', question: 'The agent has a question' }
const DONE_TITLE = { tool: 'Permission request', plan: 'Plan', question: 'Question' }

const OUTCOME: Record<string, { text: string; cls: string }> = {
  allowed: { text: 'Allowed once', cls: 'text-emerald-400' },
  always: { text: 'Allowed — saved as a rule for this task', cls: 'text-emerald-400' },
  all: { text: 'Allowed — every later request in this task is allowed too', cls: 'text-amber-300' },
  auto: { text: 'Auto-approved by your rule', cls: 'text-emerald-400/80' },
  denied: { text: 'Denied', cls: 'text-rose-400' },
  expired: { text: 'Expired — no answer in time, so it was denied', cls: 'text-amber-400' },
  cancelled: { text: 'Cancelled — the run stopped first', cls: 'text-slate-500' },
}
const PLAN_OUTCOME: Record<string, string> = {
  allowed: 'Plan approved — the agent may act',
  all: 'Plan approved — every later request in this task is allowed too',
  auto: 'Plan approved automatically (allow all)',
  denied: 'Sent back — the agent keeps planning',
}

const outcome = computed(() => {
  const base = OUTCOME[a.value.decision] || { text: a.value.decision, cls: 'text-slate-400' }
  if (kind.value === 'plan' && PLAN_OUTCOME[a.value.decision]) return { ...base, text: PLAN_OUTCOME[a.value.decision] }
  if (kind.value === 'question' && a.value.decision === 'allowed') return { ...base, text: 'Answered' }
  if (kind.value === 'question' && a.value.decision === 'denied') return { ...base, text: 'Skipped — the agent continues on its own judgement' }
  return base
})

const planGrant = computed(() =>
  a.value.mode === 'acceptEdits'
    ? 'Approving lets the agent edit files in the task worktree; commands, pushes and other actions still ask you here.'
    : 'Approving lets the agent act; every change it makes still asks you here first.',
)

// ---------------------------------------------------------------------------
// Questions: one pick (or several) per question, or a free-text "Other" answer
// ---------------------------------------------------------------------------

const picks = reactive<Record<string, string[]>>({})
const other = reactive<Record<string, string>>({})
const questions = computed(() => a.value.questions || [])

function toggle(q: string, label: string, multi?: boolean) {
  const cur = picks[q] || []
  if (multi) picks[q] = cur.includes(label) ? cur.filter(l => l !== label) : [...cur, label]
  else picks[q] = cur[0] === label ? [] : [label]
  if (!multi) other[q] = ''
}

// A typed answer replaces the pick of a single-choice question.
function onOther(q: string, multi?: boolean) {
  if (!multi && (other[q] || '').trim()) picks[q] = []
}

function answerFor(q: string): string {
  const parts = [...(picks[q] || [])]
  const free = (other[q] || '').trim()
  if (free) parts.push(free)
  return parts.join(', ')
}

const answers = computed(() => Object.fromEntries(questions.value.map(q => [q.question, answerFor(q.question)])))
const allAnswered = computed(() => questions.value.length > 0 && questions.value.every(q => answerFor(q.question) !== ''))

async function decide(decision: 'allow' | 'always' | 'all' | 'deny') {
  if (sending.value) return
  if (kind.value === 'question' && decision === 'allow' && !allAnswered.value) {
    error.value = 'Answer every question first.'
    return
  }
  sending.value = true
  error.value = ''
  try {
    await api.decideApproval(props.entry.task_id, a.value.id, decision, decision === 'deny' ? note.value.trim() : undefined,
      kind.value === 'question' && decision === 'allow' ? answers.value : undefined)
  } catch (err: any) {
    error.value = err?.message || 'Could not send the decision'
  } finally {
    sending.value = false
  }
}

onMounted(() => {
  clock = setInterval(() => (now.value = Date.now()), 1000)
  if (pending.value && kind.value !== 'question') allowBtn.value?.focus({ preventScroll: true })
})
onUnmounted(() => clearInterval(clock))
</script>

<template>
  <!-- Auto-approved: one quiet line -->
  <div v-if="a.decision === 'auto'" class="flex gap-2 font-mono text-xs text-slate-500">
    <ShieldCheck class="h-3.5 w-3.5 flex-shrink-0 text-emerald-500/70" />
    <span class="truncate">
      <template v-if="kind === 'plan'">Plan approved automatically · <span class="text-slate-400">{{ a.rule_label }}</span></template>
      <template v-else>Auto-approved <span class="text-slate-400">{{ a.tool_name }}</span> · <span class="text-slate-300">{{ a.summary }}</span></template>
    </span>
  </div>

  <section
    v-else
    class="rounded-lg border font-mono text-sm"
    :class="pending ? 'border-amber-600/70 bg-amber-950/20 shadow-[0_0_0_1px_rgba(217,119,6,0.15)]' : 'border-slate-800 bg-slate-900/40'"
    :data-approval-id="a.id"
    :data-approval-pending="pending ? 'true' : undefined"
    :role="pending ? 'alertdialog' : undefined"
    :aria-label="pending ? TITLE[kind] : undefined"
  >
    <header class="flex items-center gap-2 px-3 pt-2.5 text-xs">
      <ClipboardList v-if="kind === 'plan'" class="h-4 w-4" :class="pending ? 'text-amber-400' : allowed ? 'text-emerald-400' : 'text-slate-500'" />
      <MessageCircleQuestion v-else-if="kind === 'question'" class="h-4 w-4" :class="pending ? 'text-amber-400' : 'text-sky-400'" />
      <ShieldAlert v-else-if="pending" class="h-4 w-4 text-amber-400" />
      <ShieldX v-else-if="!allowed" class="h-4 w-4 text-rose-400/80" />
      <ShieldCheck v-else class="h-4 w-4 text-emerald-400" />
      <span :class="pending ? 'font-semibold text-amber-200' : 'text-slate-300'">
        {{ pending ? TITLE[kind] : DONE_TITLE[kind] }}<template v-if="kind === 'tool'"> · {{ a.tool_name }}</template>
      </span>
      <span v-if="pending" class="ml-auto flex items-center gap-1 text-amber-300/80"><Clock class="h-3 w-3" /> {{ remaining }}</span>
    </header>

    <!-- Body -->
    <div class="space-y-2 px-3 py-2">
      <template v-if="kind === 'plan'">
        <div class="max-h-80 overflow-auto rounded border border-slate-800 bg-slate-950 px-3 py-2 font-sans">
          <MarkdownView :source="a.summary" variant="compact" />
        </div>
        <p v-if="pending" class="font-sans text-[11px] text-slate-400">{{ planGrant }}</p>
      </template>

      <template v-else-if="kind === 'question'">
        <fieldset v-for="q in questions" :key="q.question" class="space-y-1.5" :disabled="!pending || sending">
          <legend class="font-sans text-xs text-slate-200">
            <span v-if="q.header" class="mr-1.5 rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] uppercase text-slate-400">{{ q.header }}</span>
            {{ q.question }}
            <span v-if="q.multi_select && pending" class="text-[11px] text-slate-500">(pick any)</span>
          </legend>
          <template v-if="pending">
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="o in q.options || []"
                :key="o.label"
                type="button"
                :title="o.description"
                :aria-pressed="(picks[q.question] || []).includes(o.label)"
                class="rounded border px-2.5 py-1 text-left font-sans text-xs"
                :class="(picks[q.question] || []).includes(o.label)
                  ? 'border-sky-500 bg-sky-900/50 text-sky-100'
                  : 'border-slate-700 bg-slate-900 text-slate-200 hover:border-slate-500'"
                @click="toggle(q.question, o.label, q.multi_select)"
              >
                <span class="font-medium">{{ o.label }}</span>
                <span v-if="o.description" class="block text-[11px] text-slate-400">{{ o.description }}</span>
              </button>
            </div>
            <input
              v-model="other[q.question]"
              type="text"
              placeholder="Other — type your own answer"
              :aria-label="`Other answer to: ${q.question}`"
              class="h-7 w-full rounded border border-slate-700 bg-slate-950 px-2 font-sans text-xs text-slate-200 placeholder-slate-600 focus:border-sky-500 focus:outline-none"
              @input="onOther(q.question, q.multi_select)"
              @keydown.enter.prevent="decide('allow')"
            />
          </template>
          <p v-else-if="a.answers?.[q.question]" class="font-sans text-xs text-sky-300">→ {{ a.answers[q.question] }}</p>
        </fieldset>
      </template>

      <template v-else>
        <p v-if="entry.content" class="font-sans text-xs text-slate-400">{{ entry.content }}</p>
        <pre class="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded border border-slate-800 bg-slate-950 px-2.5 py-1.5 text-[12px] text-slate-100">{{ a.summary }}</pre>
        <p v-if="a.blocked_path" class="text-[11px] text-slate-500">Outside the workspace: {{ a.blocked_path }}</p>
      </template>
    </div>

    <!-- Decision -->
    <div v-if="pending" class="space-y-2 border-t border-amber-900/40 px-3 py-2.5">
      <div v-if="!denying" class="flex flex-wrap items-center gap-2">
        <template v-if="kind === 'question'">
          <button type="button" :disabled="sending || !allAnswered" @click="decide('allow')"
            class="h-7 rounded border border-sky-500 bg-sky-600 px-3 text-xs font-semibold text-white hover:bg-sky-500 disabled:opacity-50">
            Send answer
          </button>
          <button type="button" :disabled="sending" @click="denying = true"
            class="h-7 rounded border border-slate-700 bg-slate-800 px-3 text-xs text-slate-200 hover:bg-slate-700 disabled:opacity-50">
            Skip…
          </button>
        </template>
        <template v-else>
          <button ref="allowBtn" type="button" :disabled="sending" @click="decide('allow')"
            class="h-7 rounded border border-emerald-500 bg-emerald-600 px-3 text-xs font-semibold text-white hover:bg-emerald-500 disabled:opacity-50">
            {{ kind === 'plan' ? 'Approve plan' : 'Allow once' }}
          </button>
          <button v-if="kind === 'tool' && a.rule_label" type="button" :disabled="sending" @click="decide('always')" :title="`Don't ask again in this task for ${a.rule_label}`"
            class="h-7 rounded border border-emerald-800 bg-emerald-950/60 px-3 text-xs text-emerald-200 hover:bg-emerald-900/60 disabled:opacity-50">
            Always allow {{ a.rule_label }}
          </button>
          <button type="button" :disabled="sending" @click="decide('all')"
            :title="kind === 'plan'
              ? 'Approve the plan and allow every later action in this task without asking. Turn it off from the console footer.'
              : 'Allow this and every later request in this task without asking. Turn it off from the console footer.'"
            class="h-7 rounded border border-amber-700 bg-amber-950/60 px-3 text-xs text-amber-200 hover:bg-amber-900/60 disabled:opacity-50">
            {{ kind === 'plan' ? 'Approve & allow all' : 'Allow all' }}
          </button>
          <button type="button" :disabled="sending" @click="denying = true"
            class="h-7 rounded border border-slate-700 bg-slate-800 px-3 text-xs text-slate-200 hover:bg-slate-700 disabled:opacity-50">
            {{ kind === 'plan' ? 'Keep planning…' : 'Deny…' }}
          </button>
        </template>
        <span class="font-sans text-[11px] text-slate-500">The agent is paused until you decide.</span>
      </div>
      <form v-else class="flex flex-wrap items-center gap-2" @submit.prevent="decide('deny')">
        <input v-model="note" type="text" autofocus
          :placeholder="kind === 'plan' ? 'Optional: what should change in the plan?' : kind === 'question' ? 'Optional: guidance instead of an answer' : 'Optional: tell the agent what to do instead'"
          :aria-label="kind === 'plan' ? 'Plan feedback' : kind === 'question' ? 'Guidance instead of an answer' : 'Reason for denying'"
          class="h-7 min-w-[240px] flex-1 rounded border border-slate-700 bg-slate-950 px-2 font-sans text-xs text-slate-200 placeholder-slate-600 focus:border-rose-500 focus:outline-none" />
        <button type="submit" :disabled="sending"
          class="h-7 rounded border border-rose-600 bg-rose-700 px-3 text-xs font-semibold text-white hover:bg-rose-600 disabled:opacity-50">
          {{ kind === 'plan' ? 'Send back' : kind === 'question' ? 'Skip' : 'Deny' }}
        </button>
        <button type="button" @click="denying = false" class="h-7 px-2 text-xs text-slate-400 hover:text-slate-200">Back</button>
      </form>
      <p v-if="error" class="font-sans text-[11px] text-rose-300" role="alert">{{ error }}</p>
    </div>
    <div v-else class="border-t border-slate-800 px-3 py-1.5 text-xs" :class="outcome.cls">
      {{ outcome.text }}<template v-if="a.decision === 'always' && a.rule_label"> ({{ a.rule_label }})</template>
      <span v-if="a.message && a.decision === 'denied'" class="text-slate-400"> — “{{ a.message }}”</span>
    </div>
  </section>
</template>
