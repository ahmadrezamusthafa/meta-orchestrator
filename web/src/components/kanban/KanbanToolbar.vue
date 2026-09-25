<script setup lang="ts">
import { useTaskStore } from '../../stores/tasks'
import BtnPrimary from '../common/BtnPrimary.vue'
import { Search, Plus } from 'lucide-vue-next'

defineEmits<{
  (e: 'open-new-task'): void
}>()

const taskStore = useTaskStore()
</script>

<template>
  <div class="h-14 px-6 py-2.5 bg-slate-900/50 border-b border-slate-800 flex items-center justify-between gap-4">
    <div class="flex items-center gap-3">
      <div class="relative w-72">
        <Search class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
        <input
          v-model="taskStore.searchQuery"
          type="text"
          placeholder="Filter by title or ID (e.g. TASK-8942)..."
          class="w-full h-9 pl-9 pr-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500 transition-colors"
        />
      </div>

      <div class="flex items-center gap-2">
        <select
          v-model="taskStore.selectedMethod"
          class="h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-300 focus:outline-none focus:border-emerald-500"
        >
          <option v-for="m in taskStore.availableMethods" :key="m" :value="m">
            {{ m === 'All' ? 'All Methods' : m }}
          </option>
        </select>
      </div>

      <div class="flex items-center gap-2">
        <select
          v-model="taskStore.selectedRepo"
          class="h-9 px-3 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-300 focus:outline-none focus:border-emerald-500"
        >
          <option v-for="r in taskStore.availableRepos" :key="r" :value="r">
            {{ r === 'All' ? 'All Repositories' : r }}
          </option>
        </select>
      </div>
    </div>

    <BtnPrimary @click="$emit('open-new-task')">
      <Plus class="w-4 h-4" />
      <span>New Task</span>
    </BtnPrimary>
  </div>
</template>
