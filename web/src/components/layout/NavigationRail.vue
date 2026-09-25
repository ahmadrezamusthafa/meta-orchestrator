<script setup lang="ts">
import { ref } from 'vue'
import NavRailItem from './NavRailItem.vue'
import {
  Kanban,
  Wrench,
  Cpu,
  GitFork,
  BookOpen,
  ChevronLeft,
  ChevronRight,
  ShieldCheck,
} from 'lucide-vue-next'

const isExpanded = ref(false)

function toggleExpand() {
  isExpanded.value = !isExpanded.value
}
</script>

<template>
  <aside
    class="fixed top-0 left-0 h-screen z-40 bg-slate-900 border-r border-slate-800 flex flex-col justify-between transition-all duration-200 ease-in-out"
    :class="isExpanded ? 'w-60' : 'w-16'"
  >
    <div>
      <div class="h-12 flex items-center px-4 border-b border-slate-800 gap-3">
        <div class="w-8 h-8 rounded-lg bg-emerald-600 flex items-center justify-center flex-shrink-0 text-white font-bold text-sm shadow-md">
          Ω
        </div>
        <div v-if="isExpanded" class="flex flex-col overflow-hidden">
          <span class="text-xs font-bold tracking-wider text-slate-100 uppercase">Meta-Orch</span>
          <span class="text-[10px] text-emerald-400 font-mono">Zero-Trust Factory</span>
        </div>
      </div>

      <nav class="p-2 space-y-1.5 mt-2">
        <NavRailItem to="/" label="Mission Control" :is-expanded="isExpanded">
          <Kanban class="w-5 h-5" />
        </NavRailItem>

        <NavRailItem to="/tools" label="Tool Lifecycle Hub" :is-expanded="isExpanded">
          <Wrench class="w-5 h-5" />
        </NavRailItem>

        <NavRailItem to="/settings/providers" label="AI Providers & Matrix" :is-expanded="isExpanded">
          <Cpu class="w-5 h-5" />
        </NavRailItem>

        <NavRailItem to="/settings/workflows" label="Custom SDLC Builder" :is-expanded="isExpanded">
          <GitFork class="w-5 h-5" />
        </NavRailItem>

        <NavRailItem to="/settings/registries" label="Multi-Source Registries" :is-expanded="isExpanded">
          <BookOpen class="w-5 h-5" />
        </NavRailItem>
      </nav>
    </div>

    <div class="p-2 border-t border-slate-800 space-y-2">
      <div
        v-if="isExpanded"
        class="px-3 py-2 rounded bg-slate-950/60 border border-slate-800 flex items-center gap-2 text-[11px] text-slate-300"
      >
        <ShieldCheck class="w-4 h-4 text-emerald-400 flex-shrink-0" />
        <div class="truncate">
          <div class="font-medium text-slate-200">Zero-Trust Guard</div>
          <div class="text-[10px] text-slate-500 font-mono">Write-Locks Active</div>
        </div>
      </div>

      <button
        @click="toggleExpand"
        type="button"
        class="w-full flex items-center justify-center p-2 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
      >
        <ChevronRight v-if="!isExpanded" class="w-5 h-5" />
        <div v-else class="flex items-center gap-2 text-xs">
          <ChevronLeft class="w-4 h-4" />
          <span>Collapse Sidebar</span>
        </div>
      </button>
    </div>
  </aside>
</template>
