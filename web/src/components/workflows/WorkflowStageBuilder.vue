<script setup lang="ts">
import { ref } from 'vue'
import type { WorkflowStage } from '../../types'
import StagePropertiesDrawer from './StagePropertiesDrawer.vue'
import { Plus, Trash2, Settings, Lock, ShieldAlert, CheckCircle2 } from 'lucide-vue-next'

const props = defineProps<{
  stages: WorkflowStage[]
}>()

const emit = defineEmits<{
  (e: 'add'): void
  (e: 'remove', index: number): void
  (e: 'update', updated: WorkflowStage[]): void
}>()

const activeEditingStage = ref<WorkflowStage | null>(null)
const isDrawerOpen = ref(false)

function openEditor(stage: WorkflowStage) {
  activeEditingStage.value = stage
  isDrawerOpen.value = true
}

function handleSaveStage(updated: WorkflowStage) {
  const list = [...props.stages]
  const idx = list.findIndex(s => s.id === updated.id)
  if (idx !== -1) {
    list[idx] = updated
    emit('update', list)
  }
  isDrawerOpen.value = false
  activeEditingStage.value = null
}
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-4">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Sequential Stage Pipeline Designer
        </h3>
        <span class="text-[11px] text-slate-400">
          Configure write-locking rules, allowed methods, and approval gates per stage
        </span>
      </div>

      <button
        @click="$emit('add')"
        type="button"
        class="h-7 px-2.5 rounded bg-emerald-950 border border-emerald-800 text-emerald-300 hover:bg-emerald-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
      >
        <Plus class="w-3.5 h-3.5" />
        <span>Add Pipeline Stage</span>
      </button>
    </div>

    <div class="space-y-3">
      <div
        v-for="(st, idx) in stages"
        :key="st.id || idx"
        class="p-3 bg-slate-950 border border-slate-800 rounded-lg flex items-center justify-between gap-4 text-xs font-mono"
      >
        <div class="flex items-center gap-3">
          <span class="text-slate-500 font-bold">#{{ idx + 1 }}</span>
          <input
            v-model="st.name"
            class="h-7 px-2 bg-slate-900 border border-slate-700 rounded text-slate-200 text-xs font-sans font-medium focus:outline-none focus:border-emerald-500 w-56"
          />
        </div>

        <!-- Role & Method Selection -->
        <div class="flex items-center gap-3">
          <span class="px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-[11px] text-slate-300">
            {{ st.assigned_role }}
          </span>

          <span class="px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-[11px] text-emerald-400">
            {{ st.allowed_methods?.[0] || 'BMAD' }}
          </span>

          <!-- Write Lock Indicator -->
          <span
            v-if="st.write_lock_workspace"
            title="Write-Locked Stage"
            class="p-1 rounded bg-amber-950/80 border border-amber-800 text-amber-300"
          >
            <Lock class="w-3.5 h-3.5" />
          </span>

          <!-- Human Gate Indicator -->
          <span
            v-if="st.requires_gate"
            title="Human Gate Approval Required"
            class="p-1 rounded bg-rose-950/80 border border-rose-800 text-rose-300"
          >
            <ShieldAlert class="w-3.5 h-3.5" />
          </span>

          <span
            v-if="!st.write_lock_workspace && !st.requires_gate"
            class="p-1 rounded bg-slate-900 border border-slate-800 text-slate-500"
          >
            <CheckCircle2 class="w-3.5 h-3.5" />
          </span>
        </div>

        <div class="flex items-center gap-1.5">
          <button
            @click="openEditor(st)"
            title="Configure stage properties"
            class="p-1.5 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-900 transition-colors"
          >
            <Settings class="w-4 h-4" />
          </button>

          <button
            @click="$emit('remove', idx)"
            title="Remove stage"
            class="p-1.5 rounded text-slate-500 hover:text-rose-400 hover:bg-slate-900 transition-colors"
          >
            <Trash2 class="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>

    <!-- Slide-over properties editor -->
    <StagePropertiesDrawer
      :stage="activeEditingStage"
      :is-open="isDrawerOpen"
      @close="isDrawerOpen = false"
      @save="handleSaveStage"
    />
  </div>
</template>
