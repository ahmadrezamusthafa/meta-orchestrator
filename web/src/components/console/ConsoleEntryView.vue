<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ConsoleEntry } from '../../types'
import ToolCallBlock from './ToolCallBlock.vue'
import ToolResultView from './ToolResultView.vue'
import RequestBlock from './RequestBlock.vue'
import { formatTime, responseSummary, stateSummary } from './consoleFormat'
import MarkdownView from '../common/MarkdownView.vue'
import type { ExpandSignal } from './expandSignal'

const props = defineProps<{
  entry: ConsoleEntry
  results?: ConsoleEntry[]
  expandSignal?: ExpandSignal
}>()

const thinkingOpen = ref(false)

const streaming = computed(() => props.entry.status === 'streaming')
const cancelled = computed(() => props.entry.status === 'cancelled')
const title = computed(() => formatTime(props.entry.created_at))

const STATE_COLORS: Record<string, string> = {
  RUNNING: 'text-emerald-400',
  COMPLETED: 'text-sky-400',
  FAILED: 'text-rose-400',
  BLOCKED_FRUSTRATION: 'text-rose-400',
  WAITING_GATE_APPROVAL: 'text-amber-400',
  WAITING_DEPENDENCY: 'text-orange-400',
  SUSPENDED: 'text-amber-400',
  PENDING: 'text-slate-400',
}
const stateColor = computed(() => STATE_COLORS[props.entry.state?.to || ''] || 'text-slate-400')
</script>

<template>
  <div :title="title" :data-kind="entry.kind">
    <!-- user -->
    <div
      v-if="entry.kind === 'user'"
      class="flex gap-2 rounded-md border border-slate-800 bg-slate-900/40 px-2.5 py-1.5 font-mono text-sm"
    >
      <span class="select-none text-slate-500" aria-hidden="true">&gt;</span>
      <span class="sr-only">You:</span>
      <pre class="min-w-0 flex-1 whitespace-pre-wrap break-words font-mono text-slate-300">{{ entry.content }}</pre>
    </div>

    <!-- assistant -->
    <div v-else-if="entry.kind === 'assistant'" class="flex gap-2 text-sm">
      <span class="w-3 flex-shrink-0 select-none pt-px font-mono text-slate-200" aria-hidden="true">⏺</span>
      <div class="min-w-0 flex-1">
        <MarkdownView :source="entry.content" variant="compact" class="text-slate-200" />
        <span v-if="streaming" class="inline-block h-4 w-1.5 translate-y-0.5 animate-pulse bg-slate-400" aria-hidden="true"></span>
        <div v-if="cancelled" class="font-mono text-[13px] text-rose-400/80">⎿  Interrupted by user</div>
      </div>
    </div>

    <!-- thinking -->
    <div v-else-if="entry.kind === 'thinking'" class="font-mono text-sm italic text-slate-500">
      <button
        type="button"
        class="flex items-baseline gap-2 rounded text-left hover:text-slate-400 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
        :aria-expanded="thinkingOpen"
        @click="thinkingOpen = !thinkingOpen"
      >
        <span class="w-3 select-none not-italic" :class="streaming ? 'animate-pulse text-orange-400/80' : ''" aria-hidden="true">✻</span>
        <span>{{ streaming ? 'Thinking…' : 'Thought' }}</span>
        <span class="text-xs not-italic text-slate-700">{{ thinkingOpen ? '▾ collapse' : '▸ expand' }}</span>
      </button>
      <pre
        v-if="thinkingOpen"
        class="ml-5 mt-1 whitespace-pre-wrap break-words border-l border-slate-800 pl-3 font-mono text-[13px] italic text-slate-500"
      >{{ entry.content }}</pre>
    </div>

    <!-- tool_use (+ nested results) -->
    <ToolCallBlock
      v-else-if="entry.kind === 'tool_use'"
      :entry="entry"
      :results="results || []"
      :expand-signal="expandSignal"
    />

    <!-- orphan tool_result -->
    <div v-else-if="entry.kind === 'tool_result'" class="ml-4">
      <ToolResultView :entry="entry" :expand-signal="expandSignal" />
    </div>

    <!-- request -->
    <RequestBlock v-else-if="entry.kind === 'request'" :entry="entry" :expand-signal="expandSignal" />

    <!-- response -->
    <div
      v-else-if="entry.kind === 'response'"
      class="font-mono text-xs text-slate-500"
      :title="entry.usage ? `${entry.usage.provider || ''} ${entry.usage.model || ''}`.trim() || title : title"
    >
      {{ responseSummary(entry) }}
    </div>

    <!-- state -->
    <div v-else-if="entry.kind === 'state'" class="font-mono text-xs">
      <span :class="stateColor">{{ stateSummary(entry) }}</span>
    </div>

    <!-- error -->
    <div v-else-if="entry.kind === 'error'" class="flex gap-2 font-mono text-sm text-rose-400" role="alert">
      <span class="w-3 flex-shrink-0 select-none" aria-hidden="true">⏺</span>
      <pre class="min-w-0 flex-1 whitespace-pre-wrap break-words font-mono">{{ entry.content || 'Unknown error' }}</pre>
    </div>

    <!-- system (and anything unknown) -->
    <pre
      v-else
      class="whitespace-pre-wrap break-words font-mono text-xs text-slate-500"
    >{{ entry.content }}</pre>
  </div>
</template>

