<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { UniversalSkillDTO, ProfileItemDTO, SkillRunResult } from '../../types'
import { api } from '../../services/api'
import BtnPrimary from '../common/BtnPrimary.vue'
import { Play, X, Terminal } from 'lucide-vue-next'

const props = defineProps<{
  skill: UniversalSkillDTO
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const profiles = ref<ProfileItemDTO[]>([])
const role = ref('')
const running = ref(false)
const result = ref<SkillRunResult | null>(null)
const error = ref('')

onMounted(async () => {
  try {
    profiles.value = (await api.getRegistries()).profiles || []
    role.value = profiles.value.find((p) => props.skill.required_roles?.includes(p.role))?.role || profiles.value[0]?.role || ''
  } catch (err: any) {
    error.value = err?.message || 'Could not load the roles'
  }
})

async function run() {
  running.value = true
  error.value = ''
  result.value = null
  try {
    result.value = await api.runSkill(props.skill.name, role.value)
  } catch (err: any) {
    error.value = err?.message || 'The run failed'
  } finally {
    running.value = false
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-center justify-between pb-3 border-b border-slate-800">
        <div>
          <h3 class="text-sm font-bold text-slate-100">Test run: {{ skill.name }}</h3>
          <span class="text-xs text-slate-400 font-mono">$ {{ skill.command }} {{ (skill.args || []).join(' ') }}</span>
        </div>
        <button @click="emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <p class="text-xs text-slate-400">
        Runs the skill's command once in the project root (at most 60 seconds), as the selected stage role.
      </p>

      <label class="block text-xs text-slate-300">
        Role
        <select v-model="role" :disabled="!profiles.length"
          class="mt-1 w-full h-9 px-3 bg-slate-950 border border-slate-700 rounded text-xs text-slate-200 font-mono">
          <option v-for="p in profiles" :key="p.role" :value="p.role">{{ p.name }} ({{ p.role }})</option>
        </select>
      </label>
      <p v-if="!profiles.length && !error" class="text-xs text-amber-300">No roles are configured in configs/profiles.json.</p>

      <p v-if="error" class="text-xs text-rose-300">{{ error }}</p>

      <div v-if="result" class="space-y-1.5">
        <div class="flex items-center gap-1.5 text-[11px] font-mono" :class="result.ok ? 'text-emerald-300' : 'text-rose-300'">
          <Terminal class="w-3.5 h-3.5" />
          <span>exit {{ result.exit_code }} · {{ result.duration_ms }} ms</span>
          <span v-if="result.error">· {{ result.error }}</span>
        </div>
        <pre class="max-h-60 p-3 bg-slate-950 border border-slate-800 rounded-lg font-mono text-xs text-slate-300 overflow-auto whitespace-pre-wrap">{{ result.output || '(no output)' }}</pre>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button @click="emit('close')" type="button" class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200">Close</button>
        <BtnPrimary :loading="running" :disabled="!role" @click="run">
          <Play class="w-3.5 h-3.5" />
          <span>Run</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
