<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useToolsStore } from '../stores/tools'
import type { ToolDTO } from '../types'
import ToolLifecycleCard from '../components/tools/ToolLifecycleCard.vue'
import InstallWizardModal from '../components/tools/InstallWizardModal.vue'
import RollbackConfirmModal from '../components/tools/RollbackConfirmModal.vue'
import BestFitMatrixModal from '../components/tools/BestFitMatrixModal.vue'
import { Search, Sparkles, Wrench } from 'lucide-vue-next'

const toolsStore = useToolsStore()

const searchQuery = ref('')
const selectedCategory = ref('All')
const activeInstallTool = ref<ToolDTO | null>(null)
const activeRollbackTool = ref<ToolDTO | null>(null)
const showMatrixModal = ref(false)

const categories = ['All', 'Methodologies', 'Parsers', 'Runtimes', 'UI Kits']

onMounted(async () => {
  await toolsStore.fetchTools()
})

const filteredTools = computed(() => {
  return toolsStore.tools.filter((t) => {
    if (selectedCategory.value !== 'All' && t.category !== selectedCategory.value) {
      return false
    }
    if (searchQuery.value) {
      const q = searchQuery.value.toLowerCase()
      if (!t.name.toLowerCase().includes(q) && !t.id.toLowerCase().includes(q)) {
        return false
      }
    }
    return true
  })
})

async function handleRollbackConfirm(version: string) {
  if (activeRollbackTool.value) {
    await toolsStore.rollbackTool(activeRollbackTool.value.id, version)
    activeRollbackTool.value = null
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Sub-header Toolbar -->
    <div class="h-14 px-6 bg-slate-900/50 border-b border-slate-800 flex items-center justify-between gap-4">
      <div class="flex items-center gap-4">
        <!-- Search -->
        <div class="relative w-64">
          <Search class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search tools & runtimes..."
            class="w-full h-9 pl-9 pr-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
          />
        </div>

        <!-- Category Tabs -->
        <div class="flex items-center gap-1">
          <button
            v-for="cat in categories"
            :key="cat"
            @click="selectedCategory = cat"
            class="h-8 px-3 rounded-lg text-xs font-medium transition-colors"
            :class="selectedCategory === cat ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
          >
            {{ cat }}
          </button>
        </div>
      </div>

      <!-- Auto-Resolve Best Fit Matrix Action -->
      <button
        @click="showMatrixModal = true"
        type="button"
        class="h-9 px-3.5 rounded-lg bg-emerald-950/80 hover:bg-emerald-900 border border-emerald-800 text-emerald-300 text-xs font-semibold uppercase tracking-wider flex items-center gap-2 transition-colors shadow-sm"
      >
        <Sparkles class="w-4 h-4 text-emerald-400" />
        <span>Auto-Resolve Best Fit Matrix</span>
      </button>
    </div>

    <!-- Catalog Grid -->
    <main class="flex-1 p-6 overflow-y-auto">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <ToolLifecycleCard
          v-for="tool in filteredTools"
          :key="tool.id"
          :tool="tool"
          @install="activeInstallTool = tool"
          @rollback="activeRollbackTool = tool"
        />
      </div>
    </main>

    <!-- Guided Install Wizard Modal -->
    <InstallWizardModal
      v-if="activeInstallTool"
      :tool="activeInstallTool"
      @close="activeInstallTool = null"
      @completed="activeInstallTool = null; toolsStore.fetchTools()"
    />

    <!-- Rollback Confirm Modal -->
    <RollbackConfirmModal
      v-if="activeRollbackTool"
      :tool="activeRollbackTool"
      @close="activeRollbackTool = null"
      @confirm="handleRollbackConfirm"
    />

    <!-- Best Fit Matrix Modal -->
    <BestFitMatrixModal
      v-if="showMatrixModal"
      :tools="toolsStore.tools"
      @close="showMatrixModal = false"
      @apply="showMatrixModal = false; toolsStore.fetchTools()"
    />
  </div>
</template>
