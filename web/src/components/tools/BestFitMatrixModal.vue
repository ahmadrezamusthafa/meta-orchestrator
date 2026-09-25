<script setup lang="ts">
import { ref } from 'vue'
import { Sparkles, X, CheckCircle2 } from 'lucide-vue-next'
import BtnPrimary from '../common/BtnPrimary.vue'

defineProps<{
  tools: any[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'apply'): void
}>()

const isApplying = ref(false)

function onApply() {
  isApplying.value = true
  setTimeout(() => {
    emit('apply')
  }, 500)
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div class="p-2 rounded bg-emerald-950 border border-emerald-800 text-emerald-400">
            <Sparkles class="w-5 h-5" />
          </div>
          <div>
            <h3 class="text-sm font-bold text-slate-100">Best-Fit Matrix Evaluator</h3>
            <span class="text-xs text-slate-400">Host Environment: Darwin ARM64</span>
          </div>
        </div>
        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <p class="text-xs text-slate-300 leading-relaxed">
        The orchestrator analyzed host CPU architecture, Node.js version, Go toolchain, and cross-tool lockfiles to compute the optimal conflict-free version matrix:
      </p>

      <div class="border border-slate-800 rounded-lg overflow-hidden">
        <table class="w-full text-left text-xs font-mono">
          <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
            <tr>
              <th class="p-2.5">Tool</th>
              <th class="p-2.5">Installed</th>
              <th class="p-2.5">Best-Fit</th>
              <th class="p-2.5">Status</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/80 text-slate-300">
            <tr v-for="t in tools" :key="t.id" class="hover:bg-slate-900/40">
              <td class="p-2.5 font-sans font-medium text-slate-200">{{ t.name }}</td>
              <td class="p-2.5 text-slate-400">{{ t.current_version }}</td>
              <td class="p-2.5 text-emerald-400">{{ t.best_fit_version }}</td>
              <td class="p-2.5">
                <span class="px-1.5 py-0.5 rounded bg-emerald-950 text-emerald-300 text-[10px] border border-emerald-800">
                  Optimal
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200"
        >
          Dismiss
        </button>

        <BtnPrimary
          :loading="isApplying"
          @click="onApply"
        >
          <CheckCircle2 class="w-3.5 h-3.5" />
          <span>Apply Recommended Matrix</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
