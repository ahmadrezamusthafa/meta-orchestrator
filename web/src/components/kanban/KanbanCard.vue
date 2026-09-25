<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { Task } from '../../types'
import RoutingExplainerPill from '../common/RoutingExplainerPill.vue'
import { Layers, Flame, Clock, Lock, AlertTriangle } from 'lucide-vue-next'

const props = defineProps<{
  task: Task
}>()

const router = useRouter()

const isBlocked = computed(() => props.task.state === 'BLOCKED_FRUSTRATION')
const isWriteLocked = computed(() => {
  return props.task.current_stage_id === 'atdd_creation' || props.task.metadata?.write_lock === 'ACTIVE'
})

const timeElapsed = computed(() => {
  const created = new Date(props.task.created_at).getTime()
  const now = new Date().getTime()
  const diffMins = Math.floor((now - created) / 60000)
  if (diffMins < 60) return `${diffMins}m`
  const hours = Math.floor(diffMins / 60)
  return `${hours}h ${diffMins % 60}m`
})

function navigateToTask() {
  router.push(`/tasks/${props.task.id}`)
}
</script>

<template>
  <div
    @click="navigateToTask"
    class="p-3.5 rounded-lg bg-slate-900/90 border border-slate-800 hover:border-slate-700 hover:-translate-y-0.5 transition-all duration-150 cursor-pointer shadow-sm relative group flex flex-col justify-between"
    :class="{
      'border-l-4 !border-l-rose-500 shadow-rose-950/20 animate-pulse-subtle': isBlocked,
      'border-l-4 !border-l-amber-500': isWriteLocked && !isBlocked,
      'border-l-4 !border-l-emerald-500': !isWriteLocked && !isBlocked,
    }"
  >
    <div>
      <div class="flex items-center justify-between gap-2 mb-2">
        <span class="font-mono text-xs font-semibold text-slate-400 group-hover:text-emerald-400 transition-colors">
          {{ task.id }}
        </span>

        <div class="flex items-center gap-1.5">
          <span
            v-if="isWriteLocked"
            title="Source write-locking active (Red Phase)"
            class="p-1 rounded bg-amber-950/60 border border-amber-800/80 text-amber-400"
          >
            <Lock class="w-3 h-3" />
          </span>

          <span
            v-if="isBlocked"
            title="Blocked in frustration loop"
            class="p-1 rounded bg-rose-950/60 border border-rose-800/80 text-rose-400"
          >
            <AlertTriangle class="w-3 h-3" />
          </span>

          <RoutingExplainerPill
            :source="task.metadata?.router_source"
            :rationale="task.metadata?.router_rationale"
          />
        </div>
      </div>

      <h4 class="text-xs font-medium text-slate-200 line-clamp-2 leading-relaxed mb-3">
        {{ task.title }}
      </h4>
    </div>

    <div class="pt-2.5 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-400">
      <div class="flex items-center gap-2">
        <span
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium"
          :class="{
            'bg-emerald-950/60 text-emerald-300 border border-emerald-800/60': task.selected_method === 'BMAD',
            'bg-sky-950/60 text-sky-300 border border-sky-800/60': task.selected_method === 'Supervisor',
            'bg-purple-950/60 text-purple-300 border border-purple-800/60': task.selected_method === 'ReAct',
            'bg-amber-950/60 text-amber-300 border border-amber-800/60': task.selected_method === 'Superpower',
          }"
        >
          {{ task.selected_method }}
        </span>

        <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400">
          <Layers class="w-3 h-3" />
          <span>{{ task.assigned_repos?.length || 1 }}</span>
        </div>
      </div>

      <div class="flex items-center gap-2.5">
        <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400">
          <Flame class="w-3 h-3 text-amber-500" />
          <span>{{ Math.round((task.token_usage?.total_tokens || 0) / 1000) }}k</span>
        </div>

        <div class="flex items-center gap-1 text-[10px] font-mono text-slate-400">
          <Clock class="w-3 h-3" />
          <span>{{ timeElapsed }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
