<script setup lang="ts">
import { ref } from 'vue'
import { Bot, ChevronRight, ChevronDown, Wrench, UserCheck } from 'lucide-vue-next'

const props = defineProps<{
  thought: {
    profile?: string
    model?: string
    thought?: string
    tool_call?: {
      name: string
      arguments: any
      output?: any
    }
    is_steered?: boolean
    timestamp: string
  }
}>()

const showToolDetails = ref(false)
</script>

<template>
  <div
    class="p-3 rounded-lg border transition-colors"
    :class="thought.is_steered ? 'bg-sky-950/40 border-sky-800/80' : 'bg-slate-900/80 border-slate-800'"
  >
    <div class="flex items-center justify-between gap-2 mb-2">
      <div class="flex items-center gap-2">
        <div
          class="w-6 h-6 rounded flex items-center justify-center text-xs"
          :class="thought.is_steered ? 'bg-sky-600 text-white' : 'bg-emerald-950 text-emerald-400 border border-emerald-800'"
        >
          <UserCheck v-if="thought.is_steered" class="w-3.5 h-3.5" />
          <Bot v-else class="w-3.5 h-3.5" />
        </div>

        <span class="text-xs font-semibold text-slate-200">
          {{ thought.profile || 'AI Agent' }}
        </span>

        <span class="text-[10px] font-mono text-slate-500">
          {{ thought.model || 'claude-3-5-sonnet' }}
        </span>
      </div>

      <span class="text-[10px] font-mono text-slate-500">
        {{ new Date(thought.timestamp).toLocaleTimeString() }}
      </span>
    </div>

    <div v-if="thought.thought" class="text-xs text-slate-300 leading-relaxed font-sans pl-8">
      {{ thought.thought }}
    </div>

    <div v-if="thought.tool_call" class="mt-2.5 ml-8">
      <div
        @click="showToolDetails = !showToolDetails"
        class="flex items-center justify-between p-2 rounded bg-slate-950 border border-slate-800/80 cursor-pointer hover:border-slate-700 transition-colors"
      >
        <div class="flex items-center gap-2 text-[11px] font-mono text-emerald-400">
          <Wrench class="w-3.5 h-3.5" />
          <span>{{ thought.tool_call.name }}</span>
        </div>

        <button type="button" class="text-slate-500">
          <ChevronDown v-if="showToolDetails" class="w-3.5 h-3.5" />
          <ChevronRight v-else class="w-3.5 h-3.5" />
        </button>
      </div>

      <div
        v-if="showToolDetails"
        class="p-2.5 bg-slate-950 border-x border-b border-slate-800 rounded-b text-[10px] font-mono text-slate-400 overflow-x-auto space-y-2"
      >
        <div>
          <span class="text-slate-500 block mb-0.5">Parameters:</span>
          <pre class="text-slate-300">{{ JSON.stringify(thought.tool_call.arguments, null, 2) }}</pre>
        </div>
        <div v-if="thought.tool_call.output">
          <span class="text-slate-500 block mb-0.5">Output:</span>
          <pre class="text-emerald-400">{{ JSON.stringify(thought.tool_call.output, null, 2) }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>
