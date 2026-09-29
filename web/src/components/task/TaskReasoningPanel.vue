<script setup lang="ts">
import { computed, ref } from 'vue'
import { useAgentConsole } from '../../composables/useAgentConsole'
import { stageName } from '../../composables/taskLifecycle'
import MarkdownView from '../common/MarkdownView.vue'
import type { ConsoleEntry } from '../../types'
import { BrainCircuit, Wrench, MessageSquareText, AlertTriangle, ChevronDown, ChevronRight, ArrowRightLeft, Loader2 } from 'lucide-vue-next'

const props = defineProps<{ taskId: string }>()
const emit = defineEmits<{ (e: 'open-console'): void }>()

const c = useAgentConsole(() => props.taskId)

type Filter = 'all' | 'thinking' | 'tools' | 'outputs'
const filter = ref<Filter>('all')
const open = ref<Record<string, boolean>>({})

interface Turn {
  id: string
  source: string // "execute" (stage run) or "chat"
  model: string
  startedAt: string
  durationMs?: number
  tokens?: number
  stage?: string
  items: ConsoleEntry[]
  streaming: boolean
}

const KEEP: Record<Filter, (e: ConsoleEntry) => boolean> = {
  all: (e) => ['thinking', 'tool_use', 'assistant', 'error', 'user'].includes(e.kind),
  thinking: (e) => e.kind === 'thinking',
  tools: (e) => e.kind === 'tool_use',
  outputs: (e) => e.kind === 'assistant' || e.kind === 'error',
}

// Group the transcript into agent turns; state changes between turns become dividers.
const timeline = computed(() => {
  const out: ({ type: 'turn'; turn: Turn } | { type: 'state'; entry: ConsoleEntry })[] = []
  const byId = new Map<string, Turn>()
  let stage = ''
  for (const e of c.entries.value) {
    if (e.kind === 'state' && e.state) {
      stage = e.state.stage || stage
      out.push({ type: 'state', entry: e })
      continue
    }
    if (!e.turn_id) continue
    let t = byId.get(e.turn_id)
    if (!t) {
      t = { id: e.turn_id, source: 'chat', model: '', startedAt: e.created_at, stage, items: [], streaming: false }
      byId.set(e.turn_id, t)
      out.push({ type: 'turn', turn: t })
    }
    if (e.kind === 'request' && e.request) {
      t.source = e.request.source || t.source
      t.model = e.request.model || t.model
    } else if (e.kind === 'response' && e.usage) {
      t.durationMs = e.usage.duration_ms
      t.tokens = (e.usage.prompt_tokens || 0) + (e.usage.completion_tokens || 0)
      t.model = e.usage.model || t.model
    }
    if (e.status === 'streaming') t.streaming = true
    if (KEEP[filter.value](e)) t.items.push(e)
  }
  return out.filter((row) => row.type === 'state' || row.turn.items.length > 0)
})

const counts = computed(() => {
  const n = { thinking: 0, tools: 0, outputs: 0 }
  for (const e of c.entries.value) {
    if (e.kind === 'thinking') n.thinking++
    else if (e.kind === 'tool_use') n.tools++
    else if (e.kind === 'assistant') n.outputs++
  }
  return n
})

function toggle(id: string, dflt = false) {
  open.value[id] = !(open.value[id] ?? dflt)
}
const isOpen = (id: string, dflt = false) => open.value[id] ?? dflt

function toolSummary(e: ConsoleEntry): string {
  const input = e.tool?.input || {}
  const key = ['file_path', 'path', 'command', 'pattern', 'url', 'query'].find((k) => typeof input[k] === 'string')
  return key ? String(input[key]) : ''
}

function time(iso: string) {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0">
    <div class="px-4 py-2.5 border-b border-slate-800 flex flex-wrap items-center gap-3 text-xs">
      <div class="flex items-center gap-0.5 p-0.5 rounded-lg bg-slate-950 border border-slate-800" role="group" aria-label="Show">
        <button v-for="f in (['all', 'thinking', 'tools', 'outputs'] as Filter[])" :key="f" type="button"
          @click="filter = f" :aria-pressed="filter === f"
          class="px-2.5 py-1 rounded capitalize"
          :class="filter === f ? 'bg-slate-800 text-slate-100 font-semibold' : 'text-slate-400 hover:text-slate-200'">
          {{ f === 'all' ? 'Everything' : f }}
          <span v-if="f !== 'all'" class="text-slate-500 ml-0.5">{{ counts[f] }}</span>
        </button>
      </div>
      <span v-if="c.busy.value" class="flex items-center gap-1.5 text-sky-300"><Loader2 class="w-3.5 h-3.5 animate-spin" /> Agent is working…</span>
      <span class="ml-auto text-[11px] text-slate-500">Recorded from real agent turns. Model reasoning appears only when the provider returns it.</span>
    </div>

    <div class="flex-1 overflow-y-auto p-4">
      <div v-if="c.loading.value && c.entries.value.length === 0" class="py-12 text-center text-xs text-slate-500 animate-pulse">Loading activity…</div>
      <div v-else-if="timeline.length === 0" class="max-w-md mx-auto py-12 text-center text-xs text-slate-400 space-y-3">
        <BrainCircuit class="w-6 h-6 mx-auto text-slate-600" />
        <p>No agent activity yet. Run the stage or ask the agent a question, and its reasoning, tool calls and results will show up here.</p>
        <button type="button" @click="emit('open-console')" class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200">Open console</button>
      </div>

      <ol v-else class="max-w-4xl mx-auto space-y-4">
        <template v-for="row in timeline" :key="row.type === 'turn' ? row.turn.id : row.entry.id">
          <!-- State change divider -->
          <li v-if="row.type === 'state'" class="flex items-center gap-2 text-[11px] text-slate-500">
            <span class="flex-1 h-px bg-slate-800"></span>
            <ArrowRightLeft class="w-3 h-3" />
            <span>{{ row.entry.state?.from }} → <strong class="text-slate-300">{{ row.entry.state?.to }}</strong>
              <template v-if="row.entry.state?.stage"> · {{ stageName(row.entry.state.stage) }}</template>
              · {{ time(row.entry.created_at) }}</span>
            <span class="flex-1 h-px bg-slate-800"></span>
          </li>

          <!-- Agent turn -->
          <li v-else class="rounded-xl border border-slate-800 bg-slate-900/50 overflow-hidden">
            <header class="px-4 py-2 border-b border-slate-800 flex flex-wrap items-center gap-2 text-[11px]">
              <span class="px-1.5 py-0.5 rounded border font-semibold"
                :class="row.turn.source === 'execute' ? 'bg-emerald-950/60 border-emerald-800/60 text-emerald-300' : 'bg-slate-800 border-slate-700 text-slate-300'">
                {{ row.turn.source === 'execute' ? 'Stage run' : 'Chat' }}
              </span>
              <span v-if="row.turn.stage" class="text-slate-300">{{ stageName(row.turn.stage) }}</span>
              <span v-if="row.turn.model" class="font-mono text-purple-300">{{ row.turn.model }}</span>
              <span class="ml-auto text-slate-500 font-mono">
                {{ time(row.turn.startedAt) }}
                <template v-if="row.turn.durationMs"> · {{ (row.turn.durationMs / 1000).toFixed(1) }}s</template>
                <template v-if="row.turn.tokens"> · {{ row.turn.tokens.toLocaleString() }} tokens</template>
              </span>
              <Loader2 v-if="row.turn.streaming" class="w-3.5 h-3.5 animate-spin text-sky-400" />
            </header>

            <div class="p-3 space-y-2">
              <div v-for="e in row.turn.items" :key="e.id">
                <!-- Operator prompt -->
                <p v-if="e.kind === 'user'" class="text-xs text-slate-400 border-l-2 border-slate-700 pl-3 whitespace-pre-wrap">{{ e.content }}</p>

                <!-- Reasoning -->
                <div v-else-if="e.kind === 'thinking'" class="rounded-lg bg-sky-950/20 border border-sky-900/40">
                  <button type="button" @click="toggle(e.id, true)" :aria-expanded="isOpen(e.id, true)"
                    class="w-full px-3 py-1.5 flex items-center gap-2 text-[11px] text-sky-300">
                    <component :is="isOpen(e.id, true) ? ChevronDown : ChevronRight" class="w-3 h-3" />
                    <BrainCircuit class="w-3.5 h-3.5" /> Reasoning
                  </button>
                  <p v-if="isOpen(e.id, true)" class="px-3 pb-2.5 text-xs text-slate-300 whitespace-pre-wrap leading-relaxed">{{ e.content }}</p>
                </div>

                <!-- Tool call -->
                <div v-else-if="e.kind === 'tool_use'" class="rounded-lg bg-slate-950 border border-slate-800">
                  <button type="button" @click="toggle(e.id)" :aria-expanded="isOpen(e.id)"
                    class="w-full px-3 py-1.5 flex items-center gap-2 text-[11px]">
                    <component :is="isOpen(e.id) ? ChevronDown : ChevronRight" class="w-3 h-3 text-slate-500" />
                    <Wrench class="w-3.5 h-3.5 text-amber-400" />
                    <span class="font-mono text-amber-200">{{ e.tool?.name }}</span>
                    <span class="font-mono text-slate-500 truncate">{{ toolSummary(e) }}</span>
                    <span v-if="e.status === 'error'" class="ml-auto text-rose-400">failed</span>
                  </button>
                  <pre v-if="isOpen(e.id)" class="px-3 pb-2.5 text-[11px] text-slate-400 overflow-x-auto">{{ JSON.stringify(e.tool?.input || {}, null, 2) }}</pre>
                </div>

                <!-- Output / decision -->
                <div v-else-if="e.kind === 'assistant'" class="rounded-lg bg-slate-950/60 border border-slate-800">
                  <button type="button" @click="toggle(e.id, true)" :aria-expanded="isOpen(e.id, true)"
                    class="w-full px-3 py-1.5 flex items-center gap-2 text-[11px] text-emerald-300">
                    <component :is="isOpen(e.id, true) ? ChevronDown : ChevronRight" class="w-3 h-3" />
                    <MessageSquareText class="w-3.5 h-3.5" /> {{ row.turn.source === 'execute' ? 'Stage output' : 'Answer' }}
                  </button>
                  <MarkdownView v-if="isOpen(e.id, true)" class="px-4 pb-3" :source="e.content" variant="compact" />
                </div>

                <!-- Error -->
                <p v-else-if="e.kind === 'error'" class="px-3 py-2 rounded-lg bg-rose-950/30 border border-rose-900/50 text-xs text-rose-200 flex gap-2">
                  <AlertTriangle class="w-3.5 h-3.5 flex-shrink-0 mt-0.5" />{{ e.content }}
                </p>
              </div>
            </div>
          </li>
        </template>
      </ol>
    </div>
  </div>
</template>
