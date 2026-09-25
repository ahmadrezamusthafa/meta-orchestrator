<script setup lang="ts">
import type { ToolDTO } from '../../types'
import { Wrench, CheckCircle2, ArrowUpCircle, RotateCcw } from 'lucide-vue-next'

defineProps<{
  tool: ToolDTO
}>()

defineEmits<{
  (e: 'install', tool: ToolDTO): void
  (e: 'rollback', tool: ToolDTO): void
}>()
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3 flex flex-col justify-between hover:border-slate-700 transition-colors shadow-sm">
    <div>
      <div class="flex items-center justify-between mb-2">
        <div class="flex items-center gap-2.5">
          <div class="w-8 h-8 rounded-lg bg-slate-950 border border-slate-800 flex items-center justify-center text-emerald-400">
            <Wrench class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-xs font-bold text-slate-100">{{ tool.name }}</h3>
            <span class="text-[10px] font-mono text-slate-500">{{ tool.category }}</span>
          </div>
        </div>

        <div class="flex items-center gap-1.5">
          <span
            class="w-2 h-2 rounded-full"
            :class="{
              'bg-emerald-400': tool.status === 'HEALTHY',
              'bg-amber-400': tool.status === 'UPDATE_AVAILABLE',
              'bg-rose-400': tool.status === 'ERROR' || tool.status === 'NOT_INSTALLED',
            }"
          ></span>
          <span
            class="px-2 py-0.5 rounded text-[10px] font-mono font-medium"
            :class="{
              'bg-emerald-950/80 text-emerald-300 border border-emerald-800': tool.status === 'HEALTHY',
              'bg-amber-950/80 text-amber-300 border border-amber-800': tool.status === 'UPDATE_AVAILABLE',
              'bg-rose-950/80 text-rose-300 border border-rose-800': tool.status === 'ERROR',
            }"
          >
            {{ tool.status.replace('_', ' ') }}
          </span>
        </div>
      </div>

      <p class="text-xs text-slate-400 leading-relaxed line-clamp-2 mb-3">
        {{ tool.description }}
      </p>

      <div class="flex items-center gap-2 text-[11px] font-mono">
        <span class="text-slate-500">Active:</span>
        <span class="px-1.5 py-0.5 rounded bg-slate-950 border border-slate-800 text-slate-200">
          {{ tool.current_version }}
        </span>

        <span class="text-slate-500 ml-1">Latest:</span>
        <span class="px-1.5 py-0.5 rounded bg-slate-950 border border-slate-800 text-slate-400">
          {{ tool.latest_version }}
        </span>
      </div>
    </div>

    <div class="pt-3 border-t border-slate-800/80 flex items-center justify-between gap-2">
      <button
        @click="$emit('rollback', tool)"
        type="button"
        class="h-8 px-2.5 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-300 hover:text-white text-xs font-mono flex items-center gap-1.5 transition-colors"
      >
        <RotateCcw class="w-3 h-3 text-slate-400" />
        <span>Rollback</span>
      </button>

      <button
        @click="$emit('install', tool)"
        type="button"
        class="h-8 px-3 rounded text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5 transition-colors"
        :class="tool.status === 'UPDATE_AVAILABLE' ? 'bg-amber-600 hover:bg-amber-500 text-white' : 'bg-emerald-600 hover:bg-emerald-500 text-white'"
      >
        <ArrowUpCircle v-if="tool.status === 'UPDATE_AVAILABLE'" class="w-3.5 h-3.5" />
        <CheckCircle2 v-else class="w-3.5 h-3.5" />
        <span>{{ tool.status === 'UPDATE_AVAILABLE' ? `Update to ${tool.latest_version}` : 'Guided Install' }}</span>
      </button>
    </div>
  </div>
</template>
