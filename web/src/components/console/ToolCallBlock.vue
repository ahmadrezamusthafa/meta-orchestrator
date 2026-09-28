<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ConsoleEntry } from '../../types'
import ToolResultView from './ToolResultView.vue'
import { prettyJson, toolArgSummary } from './consoleFormat'
import type { ExpandSignal } from './expandSignal'

const props = defineProps<{
  entry: ConsoleEntry
  results: ConsoleEntry[]
  expandSignal?: ExpandSignal
}>()

const showInput = ref(false)

const name = computed(() => props.entry.tool?.name || 'Tool')
const summary = computed(() => toolArgSummary(props.entry.tool?.name, props.entry.tool?.input))
const hasError = computed(() => props.results.some(r => r.tool?.is_error || r.status === 'error'))
const pending = computed(() => !props.results.length && props.entry.status !== 'cancelled' && props.entry.status !== 'error')
const bulletClass = computed(() => {
  if (hasError.value || props.entry.status === 'error') return 'text-rose-400'
  if (pending.value) return 'text-slate-500 animate-pulse'
  return 'text-emerald-400'
})
</script>

<template>
  <div class="space-y-0.5">
    <div class="flex items-baseline gap-2 font-mono text-sm">
      <span class="w-3 flex-shrink-0 select-none" :class="bulletClass" aria-hidden="true">⏺</span>
      <button
        type="button"
        class="min-w-0 truncate rounded text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
        :title="showInput ? 'Hide tool input' : 'Show tool input'"
        :aria-expanded="showInput"
        @click="showInput = !showInput"
      >
        <span class="font-semibold text-slate-100">{{ name }}</span><span class="text-slate-400">({{ summary }})</span>
      </button>
    </div>
    <pre
      v-if="showInput && entry.tool?.input"
      class="ml-5 max-h-72 overflow-auto rounded border border-slate-800 bg-slate-900/50 p-2 font-mono text-xs text-slate-400"
    >{{ prettyJson(entry.tool.input) }}</pre>
    <div class="ml-4 space-y-0.5">
      <ToolResultView v-for="r in results" :key="r.id" :entry="r" :expand-signal="expandSignal" />
      <div v-if="entry.status === 'cancelled'" class="flex gap-2 font-mono text-[13px] text-rose-400/80">
        <span class="select-none pl-1 text-slate-600">⎿</span><span>Interrupted by user</span>
      </div>
    </div>
  </div>
</template>
