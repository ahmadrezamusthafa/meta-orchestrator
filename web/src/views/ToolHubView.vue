<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { usePackagesStore } from '../stores/packages'
import { useToastStore } from '../stores/toast'
import PackagesSection from '../components/packages/PackagesSection.vue'
import SetupEverythingBanner from '../components/packages/SetupEverythingBanner.vue'
import ToolchainSection from '../components/toolchain/ToolchainSection.vue'
import ConnectorSuggestions from '../components/toolchain/ConnectorSuggestions.vue'
import { Search, X, RefreshCw } from 'lucide-vue-next'

const route = useRoute()
const packagesStore = usePackagesStore()
const toastStore = useToastStore()

const searchQuery = ref('')
const selectedCategory = ref('All')
const isRefreshing = ref(false)

const categories = computed(() => ['All', ...new Set(packagesStore.packages.map((p) => p.category).filter(Boolean))])

let detach: (() => void) | null = null
onMounted(() => {
  detach = packagesStore.attach()
  void packagesStore.fetch()
  // Deep links such as /tools#toolchain ("install <tool> first") land on that section.
  if (route.hash === '#toolchain') void nextTick(() => document.getElementById('toolchain')?.scrollIntoView())
})
onBeforeUnmount(() => detach?.())

/** Re-runs every package check (e.g. after installing something from your own terminal). */
async function handleRefresh() {
  isRefreshing.value = true
  try {
    await packagesStore.recheck()
  } catch (err: any) {
    toastStore.error('Re-check failed', err?.message)
  } finally {
    isRefreshing.value = false
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Sub-header Toolbar -->
    <div class="min-h-14 px-6 py-2 bg-slate-900/50 border-b border-slate-800 flex flex-wrap items-center justify-between gap-4">
      <div class="flex flex-wrap items-center gap-4">
        <h1 class="text-sm font-bold text-slate-100">Tool Hub &amp; Packages</h1>
        <!-- Search -->
        <div class="relative w-64">
          <Search class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search packages..."
            class="w-full h-9 pl-9 pr-8 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
          />
          <button
            v-if="searchQuery"
            @click="searchQuery = ''"
            type="button"
            aria-label="Clear search"
            class="absolute right-2.5 top-2.5 text-slate-500 hover:text-slate-300 transition-colors"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <!-- Category Tabs -->
        <div class="flex flex-wrap items-center gap-1">
          <button
            v-for="cat in categories"
            :key="cat"
            type="button"
            @click="selectedCategory = cat"
            class="h-8 px-3 rounded-lg text-xs font-medium transition-colors"
            :class="selectedCategory === cat ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
          >
            {{ cat }}
          </button>
        </div>
      </div>

      <button
        @click="handleRefresh"
        :disabled="isRefreshing"
        title="Re-check every package"
        aria-label="Re-check every package"
        type="button"
        class="h-9 w-9 flex items-center justify-center rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
      >
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin text-emerald-400': isRefreshing }" />
      </button>
    </div>

    <main class="flex-1 p-6 overflow-y-auto space-y-8">
      <SetupEverythingBanner />

      <!-- The tools the orchestrator itself uses: install, update, pick a version, roll back, uninstall -->
      <div class="rounded-2xl border border-slate-800 bg-slate-900/40 p-5">
        <PackagesSection :search="searchQuery" :category="selectedCategory" />
      </div>

      <!-- Real CLI tools the registered repositories' stacks need on this machine -->
      <div id="toolchain" class="rounded-2xl border border-slate-800 bg-slate-900/40 p-5 scroll-mt-6">
        <ToolchainSection
          :scope="{}"
          title="Stack toolchain"
          empty-text="No registered repository needs command-line tools yet. Register projects in Projects, or show every supported tool."
        />
      </div>

      <div class="rounded-2xl border border-slate-800 bg-slate-900/40 p-5">
        <ConnectorSuggestions :scope="{}" title="Connectors your repositories use" />
      </div>
    </main>
  </div>
</template>
