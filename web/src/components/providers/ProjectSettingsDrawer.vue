<script setup lang="ts">
import { ref } from 'vue'
import ProjectOverrideBadge from '../common/ProjectOverrideBadge.vue'
import BtnPrimary from '../common/BtnPrimary.vue'
import { X, Save, FileCode } from 'lucide-vue-next'

defineProps<{
  isOpen: boolean
}>()

defineEmits<{
  (e: 'close'): void
}>()

const isOverride = ref(true)
const configYaml = ref(`version: "1.0.0"
active_sdlc: "general_ai_sdlc"
router:
  strategy: "BEST_PRACTICE"
  max_token_budget_per_task: 50000
providers:
  tier_1_reasoning:
    provider: "claude"
    model: "claude-3-5-sonnet-20241022"
  tier_2_codegen:
    provider: "claude"
    model: "claude-3-5-sonnet-20241022"
  tier_3_log_parsing:
    provider: "antigravity"
    model: "gemini-2.0-flash"
`)
</script>

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-50 overflow-hidden bg-slate-950/70 backdrop-blur-sm"
    @click="$emit('close')"
  >
    <div
      class="absolute inset-y-0 right-0 max-w-xl w-full bg-slate-900 border-l border-slate-800 shadow-2xl flex flex-col p-6 space-y-4"
      @click.stop
    >
      <div class="flex items-center justify-between pb-3 border-b border-slate-800">
        <div class="flex items-center gap-2">
          <FileCode class="w-5 h-5 text-emerald-400" />
          <div>
            <h3 class="text-sm font-bold text-slate-100">Project Configuration Drawer</h3>
            <span class="text-xs text-slate-400 font-mono">.sdlc/config.yaml</span>
          </div>
        </div>

        <div class="flex items-center gap-3">
          <ProjectOverrideBadge :is-override="isOverride" />
          <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
            <X class="w-5 h-5" />
          </button>
        </div>
      </div>

      <div class="flex-1 flex flex-col space-y-2">
        <label class="block text-xs font-medium text-slate-300">Raw YAML Editor</label>
        <textarea
          v-model="configYaml"
          class="flex-1 w-full p-3 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-emerald-300 leading-relaxed focus:outline-none focus:border-emerald-500"
        ></textarea>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-between">
        <button
          type="button"
          @click="isOverride = !isOverride"
          class="text-xs text-slate-400 hover:text-slate-200"
        >
          {{ isOverride ? 'Reset to System Defaults' : 'Eject to Custom Config' }}
        </button>

        <BtnPrimary @click="$emit('close')">
          <Save class="w-3.5 h-3.5" />
          <span>Save Changes</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
