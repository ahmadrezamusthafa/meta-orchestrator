<script setup lang="ts">
import { ref } from 'vue'
import type { ConsoleEntry } from '../../types'
import { requestSummary, splitLines } from './consoleFormat'
import { useExpandable } from './expandSignal'
import type { ExpandSignal } from './expandSignal'

const props = defineProps<{
  entry: ConsoleEntry
  expandSignal?: ExpandSignal
}>()

const expanded = useExpandable(() => props.expandSignal)
const openMessages = ref<Set<number>>(new Set())

function toggleMessage(i: number) {
  const next = new Set(openMessages.value)
  if (next.has(i)) next.delete(i)
  else next.add(i)
  openMessages.value = next
}

function firstLine(s: string | undefined): string {
  const l = splitLines(s)
  const head = (l[0] || '').trim()
  return head.length > 120 ? `${head.slice(0, 119)}…` : head
}

const roleClass: Record<string, string> = {
  system: 'text-violet-400',
  user: 'text-sky-400',
  assistant: 'text-emerald-400',
  tool: 'text-amber-400',
}
</script>

<template>
  <div class="font-mono text-xs">
    <button
      type="button"
      class="flex max-w-full items-baseline gap-2 rounded text-left text-slate-500 hover:text-slate-300 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <span class="truncate">{{ requestSummary(entry) }}</span>
      <span v-if="entry.request?.source" class="flex-shrink-0 text-slate-600">[{{ entry.request.source }}]</span>
      <span class="flex-shrink-0 text-slate-700">{{ expanded ? '▾' : '▸' }}</span>
    </button>
    <div v-if="expanded && entry.request" class="ml-3 mt-1 space-y-1 border-l border-slate-800 pl-3">
      <div class="text-slate-500">
        <span class="text-slate-600">model</span> {{ entry.request.model || '—' }}
        <span class="text-slate-700">·</span>
        <span class="text-slate-600">method</span> {{ entry.request.method || '—' }}
        <span class="text-slate-700">·</span>
        <span class="text-slate-600">strategy</span> {{ entry.request.strategy || '—' }}
        <template v-if="entry.request.session_id">
          <span class="text-slate-700">·</span>
          <span class="text-slate-600">session</span> {{ entry.request.session_id }}
        </template>
      </div>
      <div v-if="entry.request.fallback_chain?.length" class="text-slate-500">
        <span class="text-slate-600">fallback</span>
        <template v-for="(m, i) in entry.request.fallback_chain" :key="i">
          <span v-if="i" class="text-slate-700"> → </span>
          <span :class="m === entry.request.model ? 'text-slate-300' : ''">{{ m }}</span>
        </template>
      </div>
      <div v-for="(msg, i) in entry.request.messages || []" :key="i">
        <button
          type="button"
          class="flex max-w-full items-baseline gap-2 rounded text-left hover:text-slate-300 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
          :aria-expanded="openMessages.has(i)"
          @click="toggleMessage(i)"
        >
          <span class="flex-shrink-0 text-slate-700">{{ openMessages.has(i) ? '▾' : '▸' }}</span>
          <span class="flex-shrink-0" :class="roleClass[msg.role] || 'text-slate-400'">{{ msg.role }}</span>
          <span class="truncate text-slate-500">{{ firstLine(msg.content) }}</span>
          <span class="flex-shrink-0 text-slate-700">{{ (msg.content || '').length.toLocaleString() }} chars</span>
        </button>
        <pre
          v-if="openMessages.has(i)"
          class="ml-4 mt-0.5 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded border border-slate-800 bg-slate-900/40 p-2 text-slate-400"
        >{{ msg.content }}</pre>
      </div>
    </div>
  </div>
</template>
