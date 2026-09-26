<script setup lang="ts">
import { ref } from 'vue'
import DaemonStatusPill from '../common/DaemonStatusPill.vue'
import { useTaskStore } from '../../stores/tasks'
import { useLayoutStore } from '../../stores/layout'
import { AlertOctagon, Flame, FolderGit2 } from 'lucide-vue-next'

const taskStore = useTaskStore()
const layoutStore = useLayoutStore()

const selectedWorkspace = ref('Default Monorepo (meta-orchestrator)')
const workspaces = [
  'Default Monorepo (meta-orchestrator)',
  'E-Commerce Multi-Repo Stack',
  'Fintech Payment Gateway Suite',
]
</script>

<template>
  <header
    class="fixed top-0 right-0 h-12 bg-slate-900/90 backdrop-blur border-b border-slate-800 px-4 flex items-center justify-between z-30 transition-all duration-200"
    :class="layoutStore.isNavExpanded ? 'left-60' : 'left-16'"
  >
    <div class="flex items-center gap-3">
      <div class="flex items-center gap-2 text-xs text-slate-400">
        <FolderGit2 class="w-4 h-4 text-slate-500" />
        <span class="font-medium text-slate-300">Workspace:</span>
      </div>
      <select
        v-model="selectedWorkspace"
        class="bg-slate-950 border border-slate-800 rounded px-2.5 py-1 text-xs text-slate-200 font-medium focus:outline-none focus:border-emerald-500"
      >
        <option v-for="ws in workspaces" :key="ws" :value="ws">{{ ws }}</option>
      </select>
    </div>

    <div class="flex items-center gap-4">
      <router-link
        v-if="taskStore.frustratedTaskCount > 0"
        to="/"
        class="flex items-center gap-1.5 px-2.5 py-1 bg-rose-950 border border-rose-800/80 text-rose-300 rounded-full text-xs font-mono font-medium animate-pulse-subtle hover:bg-rose-900 transition-colors"
      >
        <AlertOctagon class="w-3.5 h-3.5 text-rose-400" />
        <span>{{ taskStore.frustratedTaskCount }} Task Blocked</span>
      </router-link>

      <div class="flex items-center gap-1.5 px-2.5 py-1 bg-slate-950 border border-slate-800 rounded-full text-xs font-mono text-sky-400">
        <Flame class="w-3.5 h-3.5 text-amber-500" />
        <span>{{ taskStore.totalTokensBurned.toLocaleString() }} tokens</span>
      </div>

      <DaemonStatusPill />
    </div>
  </header>
</template>
