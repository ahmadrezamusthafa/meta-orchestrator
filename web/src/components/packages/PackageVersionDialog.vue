<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { X, Loader2, Download, History } from 'lucide-vue-next'
import type { PackageDTO } from '../../types'
import { api } from '../../services/api'

/**
 * install: pick a version from the registry (GET /packages/{id}/versions) or leave it on latest.
 * rollback: pick one of the versions kept on this machine; defaults to the previous one.
 */
const props = defineProps<{ pkg: PackageDTO; mode: 'install' | 'rollback' }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'confirm', version: string): void }>()

const versions = ref<string[]>([])
const latest = ref('')
const loading = ref(false)
const error = ref('')
const prerelease = ref(false)
const choice = ref('')

/** Kept versions other than the active one, newest install first. */
const rollbackTargets = computed(() =>
  [...(props.pkg.installed_versions || [])]
    .filter((v) => !v.active)
    .sort((a, b) => (b.installed_at || '').localeCompare(a.installed_at || '')),
)

async function loadVersions() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.getPackageVersions(props.pkg.id, prerelease.value)
    versions.value = res.versions || []
    latest.value = res.latest || ''
  } catch (e: any) {
    error.value = e?.message || 'Could not load versions'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (props.mode === 'install') void loadVersions()
  else choice.value = rollbackTargets.value[0]?.version || ''
})
watch(prerelease, () => void loadVersions())

function confirm() {
  emit('confirm', choice.value)
}
</script>

<template>
  <div class="fixed inset-0 z-[60] bg-black/70 backdrop-blur-sm flex items-center justify-center p-4" @click.self="emit('close')">
    <div class="w-full max-w-sm bg-slate-900 border border-slate-800 rounded-xl shadow-2xl">
      <div class="px-5 py-4 border-b border-slate-800 flex items-center justify-between">
        <h3 class="text-sm font-bold text-slate-100">
          {{ mode === 'install' ? `Install ${pkg.name}` : `Roll back ${pkg.name}` }}
        </h3>
        <button type="button" class="text-slate-500 hover:text-slate-300" @click="emit('close')"><X class="w-4 h-4" /></button>
      </div>

      <div class="p-5 space-y-3 text-xs">
        <template v-if="mode === 'install'">
          <label class="block text-[11px] text-slate-400">Version</label>
          <div v-if="loading" class="flex items-center gap-2 text-slate-400"><Loader2 class="w-3.5 h-3.5 animate-spin" /> Loading versions…</div>
          <p v-else-if="error" class="text-rose-300">{{ error }}</p>
          <select v-else v-model="choice" class="w-full h-9 px-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono focus:outline-none focus:border-emerald-500">
            <option value="">Latest{{ latest ? ` (${latest})` : '' }}</option>
            <option v-for="v in versions" :key="v" :value="v">{{ v }}{{ v === latest ? ' (latest)' : '' }}{{ v === pkg.version ? ' (current)' : '' }}</option>
          </select>
          <label class="flex items-center gap-2 text-[11px] text-slate-400">
            <input v-model="prerelease" type="checkbox" class="accent-emerald-500" /> Include pre-releases
          </label>
        </template>

        <template v-else>
          <p class="text-slate-400">
            Currently active: <span class="font-mono text-slate-200">{{ pkg.version || 'unknown' }}</span>
          </p>
          <p v-if="!rollbackTargets.length" class="text-amber-300">No other version is kept on this machine.</p>
          <template v-else>
            <label class="block text-[11px] text-slate-400">Switch to</label>
            <select v-model="choice" class="w-full h-9 px-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono focus:outline-none focus:border-emerald-500">
              <option v-for="(v, i) in rollbackTargets" :key="v.version" :value="v.version">
                {{ v.version }}{{ i === 0 ? ' (previous)' : '' }}{{ v.kind === 'snapshot' ? ' · snapshot' : '' }}
              </option>
            </select>
          </template>
        </template>
      </div>

      <div class="px-5 py-3 border-t border-slate-800 flex justify-end gap-2">
        <button type="button" class="h-8 px-3 rounded text-xs text-slate-400 hover:text-slate-200" @click="emit('close')">Cancel</button>
        <button
          type="button"
          class="h-8 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-semibold flex items-center gap-1.5"
          :disabled="mode === 'install' ? loading : !choice"
          @click="confirm"
        >
          <component :is="mode === 'install' ? Download : History" class="w-3.5 h-3.5" />
          {{ mode === 'install' ? (choice ? `Install ${choice}` : 'Install latest') : `Roll back to ${choice}` }}
        </button>
      </div>
    </div>
  </div>
</template>
