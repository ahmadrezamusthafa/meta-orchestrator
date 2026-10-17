<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { ToolMatrix } from '../../types'
import { useToolsStore } from '../../stores/tools'
import { Sparkles, X, Loader2 } from 'lucide-vue-next'

const emit = defineEmits<{
  (e: 'close'): void
}>()

const toolsStore = useToolsStore()
const matrix = ref<ToolMatrix | null>(null)
const error = ref('')

onMounted(async () => {
  try {
    matrix.value = await toolsStore.resolveBestFitMatrix()
  } catch (err: any) {
    error.value = err?.message || 'Could not check compatibility'
  }
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div class="p-2 rounded bg-emerald-950 border border-emerald-800 text-emerald-400">
            <Sparkles class="w-5 h-5" />
          </div>
          <div>
            <h3 class="text-sm font-bold text-slate-100">Compatibility with this machine</h3>
            <span v-if="matrix" class="text-xs text-slate-400 font-mono">{{ matrix.host_os }}/{{ matrix.host_arch }}</span>
          </div>
        </div>
        <button @click="emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <p v-if="error" class="text-xs text-rose-300">{{ error }}</p>
      <div v-else-if="!matrix" class="flex items-center gap-2 text-xs text-slate-400">
        <Loader2 class="w-4 h-4 animate-spin" /> Checking installed runtimes…
      </div>
      <template v-else>
        <p class="text-xs text-slate-300 leading-relaxed">
          Each tool's required runtimes, checked against the versions installed here.
        </p>
        <div class="border border-slate-800 rounded-lg overflow-hidden">
          <table class="w-full text-left text-xs">
            <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
              <tr>
                <th class="p-2.5">Tool</th>
                <th class="p-2.5">Installed</th>
                <th class="p-2.5">Runtimes</th>
                <th class="p-2.5">Status</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/80 text-slate-300">
              <tr v-for="t in matrix.tools" :key="t.tool_id">
                <td class="p-2.5 font-medium text-slate-200">{{ t.name }}</td>
                <td class="p-2.5 font-mono text-slate-400">{{ t.installed_version || (t.status === 'INSTALLED' ? 'yes' : '—') }}</td>
                <td class="p-2.5 font-mono">
                  <div v-if="!t.runtimes.length" class="text-slate-500">none</div>
                  <div v-for="r in t.runtimes" :key="r.runtime" :class="r.satisfied ? 'text-slate-300' : 'text-amber-300'">
                    {{ r.runtime }} {{ r.constraint }} → {{ r.found || r.problem }}
                  </div>
                </td>
                <td class="p-2.5">
                  <span class="px-1.5 py-0.5 rounded text-[10px] border"
                    :class="t.compatible ? 'bg-emerald-950 text-emerald-300 border-emerald-800' : 'bg-amber-950 text-amber-300 border-amber-800'">
                    {{ t.compatible ? 'Compatible' : 'Runtime missing' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end">
        <button @click="emit('close')" type="button" class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200">
          Close
        </button>
      </div>
    </div>
  </div>
</template>
