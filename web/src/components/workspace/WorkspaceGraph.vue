<script setup lang="ts">
import { ref, computed } from 'vue'
import WorkspaceGraphNode from './WorkspaceGraphNode.vue'
import { Network, Server, ShieldCheck, Cpu, HardDrive } from 'lucide-vue-next'

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

const selectedNode = ref<string>(effectiveRepos.value[0] || 'frontend-portal')

const nodes = computed(() => {
  const branchName = `feat/${props.taskId || 'TASK-LIVE'}`
  const coords: Record<string, { x: number; y: number }> = {
    'frontend-portal': { x: 40, y: 40 },
    'backend-core': { x: 280, y: 40 },
    'api-contracts': { x: 160, y: 160 },
  }

  return effectiveRepos.value.map((repo, idx) => {
    const defaultCoord = coords[repo] || { x: 40 + (idx % 2) * 240, y: 40 + Math.floor(idx / 2) * 120 }
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

const edges = computed(() => {
  const list: Array<{ id: string; d: string }> = []
  const hasFrontend = effectiveRepos.value.includes('frontend-portal')
  const hasBackend = effectiveRepos.value.includes('backend-core')
  const hasContracts = effectiveRepos.value.includes('api-contracts')

  if (hasFrontend && hasBackend) {
    list.push({ id: 'fe-be', d: 'M 220 72 L 280 72' })
  }
  if (hasFrontend && hasContracts) {
    list.push({ id: 'fe-contracts', d: 'M 130 104 L 200 160' })
  }
  if (hasBackend && hasContracts) {
    list.push({ id: 'be-contracts', d: 'M 330 104 L 260 160' })
  }

  return list
})

const selectedRepoSymlinks = computed(() => {
  switch (selectedNode.value) {
    case 'frontend-portal':
      return 'package.json: "contracts": "file:../api-contracts" (symlinked to node_modules/@factory/contracts)'
    case 'backend-core':
      return 'go.mod / composer.json: replace github.com/ahmadrezamusthafa/api-contracts => ../api-contracts'
    case 'api-contracts':
      return 'Shared Protobuf & OpenAPI specs source repository (symlinked into consumer trees)'
    default:
      return 'Auto-symlinked via internal/symlink/resolver.go'
  }
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden select-none">
    <!-- Header -->
    <div class="h-10 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <Network class="w-3.5 h-3.5 text-emerald-400" />
        <span class="font-mono text-slate-300 font-medium">MULTI-REPO EPHEMERAL TOPOLOGY</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 text-[10px] font-mono border border-emerald-800 flex items-center gap-1">
          <ShieldCheck class="w-3 h-3 text-emerald-400" />
          <span>Zero Collision Verified</span>
        </span>
        <span class="text-[11px] font-mono text-emerald-400 hidden sm:inline">Bridge 172.28.0.0/16</span>
      </div>
    </div>

    <!-- SVG Canvas -->
    <div class="flex-1 relative bg-slate-950 p-4 flex flex-col justify-between">
      <svg class="w-full h-64 overflow-visible">
        <!-- Dynamically Computed Connecting IO Edges -->
        <path
          v-for="edge in edges"
          :key="edge.id"
          :d="edge.d"
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
      <div class="p-3.5 bg-slate-900 border border-slate-800 rounded-xl text-xs space-y-2 font-mono shadow-lg">
        <div class="flex items-center justify-between text-slate-200 font-semibold font-sans">
          <div class="flex items-center gap-2">
            <Server class="w-4 h-4 text-emerald-400" />
            <span>Container Inspection: {{ selectedNode }}</span>
          </div>
          <span class="text-[10px] text-slate-400 font-mono">Sandbox: /workspaces/{{ taskId || 'TASK-LIVE' }}</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px] text-slate-400">
          <div class="flex items-center gap-1.5">
            <HardDrive class="w-3.5 h-3.5 text-slate-500 flex-shrink-0" />
            <span class="truncate">Mount: <strong class="text-slate-200">/workspaces/{{ taskId || 'TASK-LIVE' }}/{{ selectedNode }}</strong></span>
          </div>

          <div class="flex items-center gap-1.5">
            <Cpu class="w-3.5 h-3.5 text-slate-500 flex-shrink-0" />
            <span>Isolation: <strong class="text-emerald-400">Docker Bridge (172.28.0.4)</strong></span>
          </div>

          <div class="flex items-center gap-1.5 col-span-1 md:col-span-2">
            <span class="text-slate-500">Symlinks:</span>
            <span class="text-emerald-300 truncate">{{ selectedRepoSymlinks }}</span>
          </div>

          <div class="flex items-center gap-1.5">
            <span class="text-slate-500">Write-Lock Status:</span>
            <span :class="isWriteLocked ? 'text-amber-400 font-bold' : 'text-emerald-400 font-bold'">
              {{ isWriteLocked ? 'ENGAGED (RO Red Phase)' : 'DISENGAGED (RW Green Phase)' }}
            </span>
          </div>

          <div class="flex items-center gap-1.5">
            <span class="text-slate-500">Backing Services:</span>
            <span class="text-slate-300">PostgreSQL (5432), Redis (6379)</span>
          </div>
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
