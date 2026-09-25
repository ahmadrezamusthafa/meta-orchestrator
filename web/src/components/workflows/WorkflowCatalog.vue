<script setup lang="ts">
import type { WorkflowDefinition } from '../../types'
import { GitFork, Check, Play } from 'lucide-vue-next'

defineProps<{
  workflows: WorkflowDefinition[]
  activeId: string
}>()

defineEmits<{
  (e: 'select', id: string): void
}>()
</script>

<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
        Installed SDLC Workflows
      </h3>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div
        v-for="wf in workflows"
        :key="wf.id"
        @click="$emit('select', wf.id)"
        class="p-4 rounded-xl border cursor-pointer transition-all duration-150 flex flex-col justify-between"
        :class="activeId === wf.id ? 'bg-slate-900 border-emerald-500 shadow-md ring-1 ring-emerald-500/20' : 'bg-slate-900/60 border-slate-800 hover:border-slate-700'"
      >
        <div>
          <div class="flex items-center justify-between mb-2">
            <div class="flex items-center gap-2">
              <GitFork class="w-4 h-4 text-emerald-400" />
              <h4 class="text-xs font-bold text-slate-100">{{ wf.name }}</h4>
            </div>

            <span
              v-if="activeId === wf.id"
              class="flex items-center gap-1 px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 text-[10px] font-mono border border-emerald-800"
            >
              <Check class="w-3 h-3" />
              <span>Project Default</span>
            </span>
          </div>

          <p class="text-xs text-slate-400 leading-relaxed mb-3">
            {{ wf.description }}
          </p>
        </div>

        <div class="pt-3 border-t border-slate-800/80 flex items-center justify-between text-[11px] font-mono text-slate-400">
          <span>Stages: {{ wf.stages.length }}</span>
          <span>v{{ wf.version }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
