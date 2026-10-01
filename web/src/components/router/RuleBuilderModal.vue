<script setup lang="ts">
import { ref } from 'vue'
import { X, Check } from 'lucide-vue-next'
import BtnPrimary from '../common/BtnPrimary.vue'

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save', rule: any): void
}>()

const stage = ref('task_implementation')
const complexity = ref('HIGH')
const method = ref('Supervisor')
const provider = ref('Anthropic Claude')
const model = ref('claude-sonnet-5-5')

function handleSave() {
  emit('save', {
    condition: `Stage == "${stage.value}" && Complexity == "${complexity.value}"`,
    method: method.value,
    provider: provider.value,
    model: model.value,
  })
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-slate-800">
        <h3 class="text-sm font-bold text-slate-100">Add Custom Routing Rule</h3>
        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-3 text-xs">
        <div>
          <label class="block text-slate-400 mb-1">Target Stage</label>
          <select v-model="stage" class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none">
            <option value="prd_discovery">Stage 1: PRD Discovery</option>
            <option value="atdd_creation">Stage 2: ATDD Creation</option>
            <option value="techdoc_rfc">Stage 3: Tech Doc RFC</option>
            <option value="task_implementation">Stage 5: Task Implementation</option>
            <option value="e2e_validation">Stage 6: E2E Validation</option>
          </select>
        </div>

        <div>
          <label class="block text-slate-400 mb-1">Complexity Strata</label>
          <select v-model="complexity" class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none">
            <option value="LOW">LOW</option>
            <option value="MEDIUM">MEDIUM</option>
            <option value="HIGH">HIGH</option>
            <option value="SYSTEM">SYSTEM</option>
          </select>
        </div>

        <div>
          <label class="block text-slate-400 mb-1">Assign Methodology</label>
          <select v-model="method" class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none">
            <option value="BMAD">BMAD Multi-Agent</option>
            <option value="Supervisor">Supervisor</option>
            <option value="ReAct">ReAct</option>
            <option value="Superpower">Superpower</option>
          </select>
        </div>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button @click="$emit('close')" class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200">
          Cancel
        </button>
        <BtnPrimary @click="handleSave">
          <Check class="w-3.5 h-3.5" />
          <span>Save Rule</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
