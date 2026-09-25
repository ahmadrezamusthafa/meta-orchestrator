<script setup lang="ts">
import type { WorkflowStage } from '../../types'
import { Lock, ShieldAlert, CheckCircle2 } from 'lucide-vue-next'

defineProps<{
  stages: WorkflowStage[]
}>()
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Live Projected Kanban Board Preview
        </h3>
        <span class="text-[11px] text-slate-400">
          Visual simulation of the columns that will project on the Mission Control board
        </span>
      </div>
      <span class="text-xs font-mono text-emerald-400">{{ stages.length }} Columns</span>
    </div>

    <div class="flex items-center gap-3 overflow-x-auto pb-2">
      <div
        v-for="(stage, idx) in stages"
        :key="stage.id || idx"
        class="w-48 min-w-[192px] h-36 bg-slate-950 rounded-lg border border-slate-800 p-3 flex flex-col justify-between"
      >
        <div class="space-y-1">
          <div class="flex items-center justify-between text-[11px]">
            <span class="text-slate-500 font-mono">Col #{{ idx + 1 }}</span>
            <Lock v-if="stage.write_lock_workspace" class="w-3 h-3 text-amber-400" />
            <ShieldAlert v-else-if="stage.requires_gate" class="w-3 h-3 text-rose-400" />
            <CheckCircle2 v-else class="w-3 h-3 text-emerald-400" />
          </div>
          <h4 class="text-xs font-semibold text-slate-200 line-clamp-2">
            {{ stage.name }}
          </h4>
        </div>

        <div class="pt-2 border-t border-slate-900 text-[10px] font-mono text-slate-500 flex items-center justify-between">
          <span>{{ stage.assigned_role || 'developer' }}</span>
          <span class="text-emerald-400">{{ stage.allowed_methods?.[0] || 'BMAD' }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
