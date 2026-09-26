<script setup lang="ts">
import { ref, computed } from 'vue'
import WorkspaceGraphNode from './WorkspaceGraphNode.vue'
import { Network, Server } from 'lucide-vue-next'

const props = defineProps<{
  taskId?: string
  assignedRepos?: string[]
  currentStageId?: string
  isWriteLocked?: boolean
}>()

const effectiveRepos = computed(() => {
  if (props.assignedRepos && props.assignedRepos.length > 0) {
    return props.assignedRepos
  }
  return ['frontend-portal', 'backend-core']
})

const selectedNode = ref<string>('frontend-portal')

const nodes = computed(() => {
  const branchName = `feat/${props.taskId || 'TASK-LIVE'}`
  const coords: Record<string, { x: number; y: number }> = {
    'frontend-portal': { x: 40, y: 40 },
    'backend-core': { x: 280, y: 40 },
    'api-contracts': { x: 160, y: 160 },
  }

  return effectiveRepos.value.map((repo, idx) => {
    const defaultCoord = coords[repo] || { x: 40 + idx * 120, y: 40 + (idx % 2) * 80 }
    return {
      id: repo,
      name: repo,
      branch: repo === 'api-contracts' ? 'main' : branchName,
      x: defaultCoord.x,
      y: defaultCoord.y,
      isLinked: true,
    }
  })
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Header -->
    <div class="h-9 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <Network class="w-3.5 h-3.5 text-emerald-400" />
        <span class="font-mono text-slate-300 font-medium">MULTI-REPO TOPOLOGY & DOCKER MOUNTS</span>
      </div>
      <span class="text-[11px] font-mono text-emerald-400">Isolated Network: active</span>
    </div>

    <!-- SVG Canvas -->
    <div class="flex-1 relative bg-slate-950 p-4 flex flex-col justify-between">
      <svg class="w-full h-64 overflow-visible">
        <!-- Connecting IO Edges -->
        <!-- frontend-portal to backend-core -->
        <path
          d="M 220 72 L 280 72"
          fill="none"
          stroke="#10b981"
          stroke-width="2"
          class="dash-animated"
        />

        <!-- frontend-portal to api-contracts -->
        <path
          d="M 130 104 L 200 160"
          fill="none"
          stroke="#10b981"
          stroke-width="2"
          class="dash-animated"
        />

        <!-- backend-core to api-contracts -->
        <path
          d="M 330 104 L 260 160"
          fill="none"
          stroke="#10b981"
          stroke-width="2"
          class="dash-animated"
        />

        <!-- Nodes -->
        <WorkspaceGraphNode
          v-for="node in nodes"
          :key="node.id"
          :x="node.x"
          :y="node.y"
          :name="node.name"
          :branch="node.branch"
          :is-linked="node.isLinked"
          :is-selected="selectedNode === node.id"
          @select="selectedNode = node.id"
        />
      </svg>

      <!-- Selected Node Inspection Panel -->
      <div class="p-3 bg-slate-900 border border-slate-800 rounded-lg text-xs space-y-1.5 font-mono">
        <div class="flex items-center justify-between text-slate-200 font-semibold font-sans">
          <div class="flex items-center gap-2">
            <Server class="w-4 h-4 text-emerald-400" />
            <span>Container Inspection: {{ selectedNode }}</span>
          </div>
          <span class="text-[10px] text-slate-400 font-mono">Task ID: {{ taskId || 'LIVE' }}</span>
        </div>
        <div class="grid grid-cols-2 gap-2 text-[11px] text-slate-400">
          <div>Mount: <span class="text-slate-200">/workspaces/{{ taskId || 'TASK-LIVE' }}/{{ selectedNode }}</span></div>
          <div>Network: <span class="text-emerald-400">isolated bridge (172.28.0.4)</span></div>
          <div>Symlinks: <span class="text-emerald-400">node_modules/api-contracts -> ../api-contracts</span></div>
          <div>Write-Lock: <span :class="isWriteLocked ? 'text-amber-400 font-bold' : 'text-emerald-400 font-bold'">{{ isWriteLocked ? 'ENGAGED (RO Red Phase)' : 'DISENGAGED (RW)' }}</span></div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@keyframes dash {
  to {
    stroke-dashoffset: -20;
  }
}
.dash-animated {
  stroke-dasharray: 4, 4;
  animation: dash 1s linear infinite;
}
</style>
