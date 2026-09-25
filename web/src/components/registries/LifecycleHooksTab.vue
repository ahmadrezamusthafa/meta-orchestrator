<script setup lang="ts">
import { ref } from 'vue'
import { Plus } from 'lucide-vue-next'
import HookEditorDrawer from './HookEditorDrawer.vue'

const props = defineProps<{
  hooks: Array<{
    id: string
    event: string
    type: string
    command: string
    policy: string
  }>
}>()

const emit = defineEmits<{
  (e: 'add-hook', hook: any): void
}>()

const showDrawer = ref(false)

function handleSaveHook(newHook: any) {
  emit('add-hook', newHook)
  showDrawer.value = false
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Lifecycle Event Interceptor Hooks Engine
        </h3>
        <span class="text-[11px] text-slate-400">
          Hooks fire on pre-stage, on-gate, pre-commit, and failure boundaries with blocking policies
        </span>
      </div>

      <div class="flex items-center gap-2">
        <button
          @click="showDrawer = true"
          type="button"
          class="h-7 px-2.5 rounded bg-purple-950 border border-purple-800 text-purple-300 hover:bg-purple-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>Add Lifecycle Hook</span>
        </button>

        <span class="text-xs font-mono text-purple-400 px-2 py-0.5 rounded bg-slate-900 border border-slate-800">
          {{ hooks.length }} Hooks Active
        </span>
      </div>
    </div>

    <div class="border border-slate-800 rounded-lg overflow-hidden font-mono text-xs">
      <table class="w-full text-left">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
          <tr>
            <th class="p-3">Hook ID</th>
            <th class="p-3">Trigger Event</th>
            <th class="p-3">Execution Engine</th>
            <th class="p-3">Command / URL</th>
            <th class="p-3">Policy</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="h in hooks" :key="h.id" class="hover:bg-slate-900/40">
            <td class="p-3 font-semibold text-slate-200">{{ h.id }}</td>
            <td class="p-3 text-amber-400">{{ h.event }}</td>
            <td class="p-3 text-sky-400">{{ h.type }}</td>
            <td class="p-3 text-slate-300 truncate max-w-xs">{{ h.command }}</td>
            <td class="p-3">
              <span
                class="px-2 py-0.5 rounded text-[10px]"
                :class="h.policy === 'BLOCK' ? 'bg-rose-950 text-rose-300 border border-rose-800' : 'bg-amber-950 text-amber-300 border border-amber-800'"
              >
                {{ h.policy }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <HookEditorDrawer
      :is-open="showDrawer"
      @close="showDrawer = false"
      @save="handleSaveHook"
    />
  </div>
</template>
