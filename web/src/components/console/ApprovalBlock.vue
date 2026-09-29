<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { ConsoleEntry } from '../../types'
import { api } from '../../services/api'
import { ShieldAlert, ShieldCheck, ShieldX, Clock } from 'lucide-vue-next'

const props = defineProps<{ entry: ConsoleEntry }>()

const a = computed(() => props.entry.approval!)
const pending = computed(() => a.value.decision === 'pending')
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

const OUTCOME: Record<string, { text: string; cls: string }> = {
  allowed: { text: 'Allowed once', cls: 'text-emerald-400' },
  always: { text: 'Allowed — saved as a rule for this task', cls: 'text-emerald-400' },
  auto: { text: 'Auto-approved by your rule', cls: 'text-emerald-400/80' },
  denied: { text: 'Denied', cls: 'text-rose-400' },
  expired: { text: 'Expired — no answer in time, so it was denied', cls: 'text-amber-400' },
  cancelled: { text: 'Cancelled — the run stopped first', cls: 'text-slate-500' },
}

async function decide(decision: 'allow' | 'always' | 'deny') {
  if (sending.value) return
  sending.value = true
  error.value = ''
  try {
    await api.decideApproval(props.entry.task_id, a.value.id, decision, decision === 'deny' ? note.value.trim() : undefined)
  } catch (err: any) {
    error.value = err?.message || 'Could not send the decision'
  } finally {
    sending.value = false
  }
}

onMounted(() => {
  clock = setInterval(() => (now.value = Date.now()), 1000)
  if (pending.value) allowBtn.value?.focus({ preventScroll: true })
})
onUnmounted(() => clearInterval(clock))
</script>

<template>
  <!-- Auto-approved: one quiet line -->
  <div v-if="a.decision === 'auto'" class="flex gap-2 font-mono text-xs text-slate-500">
    <ShieldCheck class="h-3.5 w-3.5 flex-shrink-0 text-emerald-500/70" />
    <span class="truncate">Auto-approved <span class="text-slate-400">{{ a.tool_name }}</span> · <span class="text-slate-300">{{ a.summary }}</span></span>
  </div>

  <section
    v-else
    class="rounded-lg border font-mono text-sm"
    :class="pending ? 'border-amber-600/70 bg-amber-950/20 shadow-[0_0_0_1px_rgba(217,119,6,0.15)]' : 'border-slate-800 bg-slate-900/40'"
    :role="pending ? 'alertdialog' : undefined"
    :aria-label="pending ? `The agent asks permission to use ${a.tool_name}` : undefined"
  >
    <header class="flex items-center gap-2 px-3 pt-2.5 text-xs">
      <ShieldAlert v-if="pending" class="h-4 w-4 text-amber-400" />
      <ShieldX v-else-if="a.decision !== 'allowed' && a.decision !== 'always'" class="h-4 w-4 text-rose-400/80" />
      <ShieldCheck v-else class="h-4 w-4 text-emerald-400" />
      <span :class="pending ? 'font-semibold text-amber-200' : 'text-slate-300'">
        {{ pending ? 'Permission needed' : 'Permission request' }} · {{ a.tool_name }}
      </span>
      <span v-if="pending" class="ml-auto flex items-center gap-1 text-amber-300/80"><Clock class="h-3 w-3" /> {{ remaining }}</span>
    </header>

    <div class="space-y-2 px-3 py-2">
      <p v-if="entry.content" class="font-sans text-xs text-slate-400">{{ entry.content }}</p>
      <pre class="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded border border-slate-800 bg-slate-950 px-2.5 py-1.5 text-[12px] text-slate-100">{{ a.summary }}</pre>
      <p v-if="a.blocked_path" class="text-[11px] text-slate-500">Outside the workspace: {{ a.blocked_path }}</p>
    </div>

    <!-- Decision -->
    <div v-if="pending" class="space-y-2 border-t border-amber-900/40 px-3 py-2.5">
      <div v-if="!denying" class="flex flex-wrap items-center gap-2">
        <button ref="allowBtn" type="button" :disabled="sending" @click="decide('allow')"
          class="h-7 rounded border border-emerald-500 bg-emerald-600 px-3 text-xs font-semibold text-white hover:bg-emerald-500 disabled:opacity-50">
          Allow once
        </button>
        <button v-if="a.rule_label" type="button" :disabled="sending" @click="decide('always')" :title="`Don't ask again in this task for ${a.rule_label}`"
          class="h-7 rounded border border-emerald-800 bg-emerald-950/60 px-3 text-xs text-emerald-200 hover:bg-emerald-900/60 disabled:opacity-50">
          Always allow {{ a.rule_label }}
        </button>
        <button type="button" :disabled="sending" @click="denying = true"
          class="h-7 rounded border border-slate-700 bg-slate-800 px-3 text-xs text-slate-200 hover:bg-slate-700 disabled:opacity-50">
          Deny…
        </button>
        <span class="font-sans text-[11px] text-slate-500">The agent is paused until you decide.</span>
      </div>
      <form v-else class="flex flex-wrap items-center gap-2" @submit.prevent="decide('deny')">
        <input v-model="note" type="text" autofocus placeholder="Optional: tell the agent what to do instead"
          aria-label="Reason for denying"
          class="h-7 min-w-[240px] flex-1 rounded border border-slate-700 bg-slate-950 px-2 font-sans text-xs text-slate-200 placeholder-slate-600 focus:border-rose-500 focus:outline-none" />
        <button type="submit" :disabled="sending"
          class="h-7 rounded border border-rose-600 bg-rose-700 px-3 text-xs font-semibold text-white hover:bg-rose-600 disabled:opacity-50">Deny</button>
        <button type="button" @click="denying = false" class="h-7 px-2 text-xs text-slate-400 hover:text-slate-200">Back</button>
      </form>
      <p v-if="error" class="font-sans text-[11px] text-rose-300" role="alert">{{ error }}</p>
    </div>
    <div v-else class="border-t border-slate-800 px-3 py-1.5 text-xs" :class="OUTCOME[a.decision]?.cls">
      {{ OUTCOME[a.decision]?.text || a.decision }}<template v-if="a.decision === 'always' && a.rule_label"> ({{ a.rule_label }})</template>
      <span v-if="a.message && a.decision === 'denied'" class="text-slate-400"> — “{{ a.message }}”</span>
    </div>
  </section>
</template>
