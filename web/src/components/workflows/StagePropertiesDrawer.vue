<script setup lang="ts">
import { X, Sliders, ShieldAlert, Lock, Check } from 'lucide-vue-next'
import type { WorkflowStage } from '../../types'
import BtnPrimary from '../common/BtnPrimary.vue'

const props = defineProps<{
  stage: WorkflowStage | null
  isOpen: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save', updated: WorkflowStage): void
}>()

function save() {
  if (props.stage) {
    emit('save', { ...props.stage })
  }
}
</script>

<template>
  <div
    v-if="isOpen && stage"
    class="fixed inset-0 z-50 overflow-hidden bg-slate-950/70 backdrop-blur-sm"
    @click="$emit('close')"
  >
    <div
      class="absolute inset-y-0 right-0 max-w-md w-full bg-slate-900 border-l border-slate-800 shadow-2xl flex flex-col p-6 space-y-4"
      @click.stop
    >
      <div class="flex items-center justify-between pb-3 border-b border-slate-800">
        <div class="flex items-center gap-2">
          <Sliders class="w-5 h-5 text-emerald-400" />
          <div>
            <h3 class="text-sm font-bold text-slate-100">Stage Properties Editor</h3>
            <span class="text-xs text-slate-400 font-mono">{{ stage.id }}</span>
          </div>
        </div>

        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="flex-1 overflow-y-auto space-y-4 text-xs font-sans">
        <div>
          <label class="block text-slate-400 mb-1 font-medium">Stage Name</label>
          <input
            v-model="stage.name"
            class="w-full h-8 px-2.5 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none focus:border-emerald-500"
          />
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Assigned Persona Role</label>
          <select
            v-model="stage.assigned_role"
            class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none"
          >
            <option value="architect">Architect</option>
            <option value="developer">Developer</option>
            <option value="qa_engineer">QA Automation Specialist</option>
            <option value="product_manager">Product Manager</option>
            <option value="release_manager">Release Manager</option>
          </select>
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Primary Execution Methodology</label>
          <select
            v-model="stage.allowed_methods[0]"
            class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none"
          >
            <option value="BMAD">BMAD Multi-Agent</option>
            <option value="Supervisor">Supervisor</option>
            <option value="ReAct">ReAct</option>
            <option value="Superpower">Superpower</option>
          </select>
        </div>

        <div class="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-3">
          <div class="font-semibold text-slate-300 uppercase tracking-wide text-[11px]">Security & Governance</div>
          
          <label class="flex items-start gap-2.5 cursor-pointer">
            <input
              type="checkbox"
              v-model="stage.write_lock_workspace"
              class="rounded bg-slate-900 border-slate-700 text-amber-500 mt-0.5"
            />
            <div>
              <div class="flex items-center gap-1.5 font-medium text-amber-300">
                <Lock class="w-3.5 h-3.5" />
                <span>Strict Write-Locking Protocol</span>
              </div>
              <div class="text-[11px] text-slate-500 leading-tight mt-0.5">
                Application source code is locked in read-only mode during this stage until red phase passes.
              </div>
            </div>
          </label>

          <label class="flex items-start gap-2.5 cursor-pointer">
            <input
              type="checkbox"
              v-model="stage.requires_gate"
              class="rounded bg-slate-900 border-slate-700 text-rose-500 mt-0.5"
            />
            <div>
              <div class="flex items-center gap-1.5 font-medium text-rose-300">
                <ShieldAlert class="w-3.5 h-3.5" />
                <span>Human Approval Gate (AWAITING_APPROVAL)</span>
              </div>
              <div class="text-[11px] text-slate-500 leading-tight mt-0.5">
                Execution pauses and requires explicit operator sign-off before advancing to the next stage.
              </div>
            </div>
          </label>
        </div>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200"
        >
          Cancel
        </button>

        <BtnPrimary @click="save">
          <Check class="w-3.5 h-3.5" />
          <span>Save Stage Properties</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
