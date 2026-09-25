<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  source?: string
  rationale?: string
}>()

const showPopover = ref(false)
</script>

<template>
  <div class="relative inline-block" @mouseenter="showPopover = true" @mouseleave="showPopover = false">
    <button
      type="button"
      class="px-1.5 py-0.5 rounded text-[10px] font-mono tracking-tight font-medium cursor-help transition-colors"
      :class="{
        'bg-sky-950/80 text-sky-300 border border-sky-800/80 hover:bg-sky-900': source === 'BP' || !source,
        'bg-purple-950/80 text-purple-300 border border-purple-800/80 hover:bg-purple-900': source === 'RULE',
      }"
    >
      {{ source === 'RULE' ? '[RULE]' : '[BP]' }}
    </button>

    <div
      v-if="showPopover"
      class="absolute bottom-full left-0 mb-1 z-50 w-64 p-2.5 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl text-[11px] text-slate-300 leading-relaxed pointer-events-none"
    >
      <div class="font-semibold text-slate-100 mb-1 flex items-center gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full" :class="source === 'RULE' ? 'bg-purple-400' : 'bg-sky-400'"></span>
        {{ source === 'RULE' ? 'Custom Rule Match' : 'Best Practice Heuristic' }}
      </div>
      <div>{{ rationale || 'Evaluated task complexity, AST radius, and SDLC stage requirements.' }}</div>
    </div>
  </div>
</template>
