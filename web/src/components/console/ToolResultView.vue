<script setup lang="ts">
import { computed } from 'vue'
import type { ConsoleEntry } from '../../types'
import { splitLines } from './consoleFormat'
import { useExpandable } from './expandSignal'
import type { ExpandSignal } from './expandSignal'

const props = defineProps<{
  entry: ConsoleEntry
  expandSignal?: ExpandSignal
}>()

const PREVIEW_LINES = 4
const expanded = useExpandable(() => props.expandSignal)

const lines = computed(() => splitLines(props.entry.content))
const isError = computed(() => !!props.entry.tool?.is_error || props.entry.status === 'error')
const hidden = computed(() => Math.max(0, lines.value.length - PREVIEW_LINES))
const shown = computed(() => (expanded.value ? lines.value : lines.value.slice(0, PREVIEW_LINES)))
</script>

<template>
  <div class="flex gap-2 font-mono text-[13px] leading-5">
    <span class="select-none pl-1 text-slate-600" aria-hidden="true">⎿</span>
    <div class="min-w-0 flex-1">
      <div v-if="!lines.length" class="text-slate-500">
        {{ entry.status === 'streaming' ? 'Running…' : '(no output)' }}
      </div>
      <pre
        v-else
        class="whitespace-pre-wrap break-words font-mono"
        :class="isError ? 'text-rose-400' : 'text-slate-400'"
      >{{ shown.join('\n') }}</pre>
      <button
        v-if="hidden > 0"
        type="button"
        class="rounded text-slate-500 hover:text-slate-300 focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-500"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        <template v-if="expanded">(click to collapse)</template>
        <template v-else>… +{{ hidden }} line{{ hidden === 1 ? '' : 's' }} (click to expand)</template>
      </button>
    </div>
  </div>
</template>
