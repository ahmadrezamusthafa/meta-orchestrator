<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { Task } from '../../types'
import { getStageGuidance } from '../../composables/useStageGuidance'
import RoutingExplainerPill from '../common/RoutingExplainerPill.vue'
import { Layers, Flame, Clock, Lock, AlertTriangle, ArrowRight, ExternalLink, FileText } from 'lucide-vue-next'

const props = defineProps<{
  task: Task
}>()

const router = useRouter()
const isDragging = ref(false)

const guidance = computed(() => getStageGuidance(props.task))
const isBlocked = computed(() => props.task.state === 'BLOCKED_FRUSTRATION')
const isWriteLocked = computed(() => {
  return props.task.current_stage_id === 'atdd_creation' || props.task.metadata?.write_lock === 'ACTIVE'
})

const jiraKey = computed(() => {
  if (props.task.metadata?.jira_key) return props.task.metadata.jira_key
  const match = props.task.title.match(/\b([A-Z]{2,10}-\d+)\b/)
  return match ? match[1] : null
})

const jiraUrl = computed(() => {
  if (props.task.metadata?.jira_url) return props.task.metadata.jira_url
  if (jiraKey.value) return `https://jira.atlassian.net/browse/${jiraKey.value}`
  return '#'
})

const confluenceUrl = computed(() => {
  return props.task.metadata?.confluence_page_url || null
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

function handleDragStart(e: DragEvent) {
  isDragging.value = true
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', props.task.id)
  }
}

function handleDragEnd() {
  isDragging.value = false
}
</script>

<template>
  <div
    draggable="true"
    @dragstart="handleDragStart"
    @dragend="handleDragEnd"
    @click="navigateToTask"
    class="p-3.5 rounded-lg bg-slate-900/90 border border-slate-800 hover:border-slate-700 hover:-translate-y-0.5 transition-all duration-150 cursor-grab active:cursor-grabbing shadow-sm relative group flex flex-col justify-between select-none"
    :class="{
      'opacity-40 scale-95 border-emerald-500/80': isDragging,
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

      <h4 class="text-xs font-medium text-slate-200 line-clamp-2 leading-relaxed mb-2">
        {{ task.title }}
      </h4>

      <!-- External Third-Party Connector Links (JIRA & Confluence) -->
      <div v-if="jiraKey || confluenceUrl" class="flex flex-wrap items-center gap-1.5 mb-2.5">
        <a
          v-if="jiraKey"
          :href="jiraUrl"
          target="_blank"
          @click.stop
          title="Open linked JIRA ticket in new tab"
          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-blue-950/80 text-blue-300 border border-blue-800/80 hover:bg-blue-900 hover:text-blue-100 hover:border-blue-600 transition-colors text-[10px] font-mono font-medium shadow-sm group/jira"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-blue-400"></span>
          <span>{{ jiraKey }}</span>
          <ExternalLink class="w-2.5 h-2.5 opacity-70 group-hover/jira:opacity-100" />
        </a>

        <a
          v-if="confluenceUrl"
          :href="confluenceUrl"
          target="_blank"
          @click.stop
          title="Open published Technical RFC on Confluence"
          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-indigo-950/80 text-indigo-300 border border-indigo-800/80 hover:bg-indigo-900 hover:text-indigo-100 hover:border-indigo-600 transition-colors text-[10px] font-mono font-medium shadow-sm group/conf"
        >
          <FileText class="w-2.5 h-2.5 text-indigo-400" />
          <span>Confluence RFC</span>
          <ExternalLink class="w-2.5 h-2.5 opacity-70 group-hover/conf:opacity-100" />
        </a>
      </div>

      <!-- Real Status & Step Information Pill -->
      <div class="mb-3 p-2 rounded bg-slate-950/90 border border-slate-800/80 space-y-1 text-[10px] font-mono">
        <div class="flex items-center justify-between">
          <span class="text-slate-400 font-medium">Step {{ guidance.stageNumber }}/{{ guidance.totalStages }}</span>
          <span
            v-if="guidance.actionType === 'blocked_steer'"
            class="text-rose-400 font-semibold"
          >
            ● Steer Needed
          </span>
          <span
            v-else-if="guidance.actionType === 'gate_approval'"
            class="text-amber-400 font-semibold"
          >
            ● Gate Review
          </span>
          <span
            v-else
            class="text-emerald-400"
          >
            ● {{ guidance.isWriteLocked ? 'Locked (Red)' : 'Active' }}
          </span>
        </div>
        <div class="text-slate-400 truncate flex items-center gap-1">
          <span class="text-slate-500">Next:</span>
          <span class="text-slate-300 truncate">{{ guidance.nextStageName }}</span>
        </div>
      </div>
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
