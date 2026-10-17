<script setup lang="ts">
import type { ToolDTO } from '../../types'
import { Wrench, Download } from 'lucide-vue-next'

defineProps<{
  tool: ToolDTO
}>()

defineEmits<{
  (e: 'install', tool: ToolDTO): void
}>()

const label: Record<ToolDTO['status'], string> = {
  INSTALLED: 'Installed',
  NOT_INSTALLED: 'Not installed',
  ERROR: 'Check failed',
}
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
        <span
          class="px-2 py-0.5 rounded text-[10px] font-mono font-medium border"
          :class="{
            'bg-emerald-950/80 text-emerald-300 border-emerald-800': tool.status === 'INSTALLED',
            'bg-slate-950 text-slate-400 border-slate-700': tool.status === 'NOT_INSTALLED',
            'bg-rose-950/80 text-rose-300 border-rose-800': tool.status === 'ERROR',
          }"
        >
          {{ label[tool.status] }}
        </span>
      </div>

      <p class="text-xs text-slate-400 leading-relaxed line-clamp-2 mb-3">{{ tool.description }}</p>

      <div class="space-y-1 text-[11px] font-mono">
        <div class="flex items-center gap-2">
          <span class="text-slate-500">Version:</span>
          <span class="text-slate-200">{{ tool.installed_version || (tool.status === 'INSTALLED' ? 'unknown' : '—') }}</span>
        </div>
        <div class="text-slate-500 truncate" :title="tool.health_check_cmd">$ {{ tool.health_check_cmd }}</div>
        <div v-if="tool.problem" class="text-rose-300 break-words">{{ tool.problem }}</div>
      </div>
    </div>

    <div v-if="tool.status !== 'INSTALLED'" class="pt-3 border-t border-slate-800/80 flex items-center justify-end">
      <button
        @click="$emit('install', tool)"
        type="button"
        class="h-8 px-3 rounded text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5 transition-colors bg-emerald-600 hover:bg-emerald-500 text-white"
      >
        <Download class="w-3.5 h-3.5" />
        <span>{{ tool.toolchain_id ? 'Install' : 'How to install' }}</span>
      </button>
    </div>
  </div>
</template>
