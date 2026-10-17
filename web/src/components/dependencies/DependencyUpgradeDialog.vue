<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { X, ArrowUpCircle, Loader2, ExternalLink } from 'lucide-vue-next'
import type { RepoDependency, Task } from '../../types'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'

/** Asks for a target version and creates a task that upgrades one dependency. */
const props = defineProps<{ projectId: string; repo: string; ecosystem: string; dep: RepoDependency }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const toast = useToastStore()
const target = ref(props.dep.latest || '')
const note = ref('')
const saving = ref(false)
const error = ref('')
const created = ref<Task | null>(null)

async function submit() {
  if (!target.value.trim()) return
  saving.value = true
  error.value = ''
  try {
    const res = await api.upgradeProjectDependency(props.projectId, {
      repo: props.repo,
      ecosystem: props.ecosystem,
      name: props.dep.name,
      target: target.value.trim(),
      note: note.value.trim() || undefined,
    })
    created.value = res.task
    toast.success('Upgrade task created', res.task?.title || `${props.dep.name} → ${target.value.trim()}`)
  } catch (e: any) {
    error.value = e?.message || 'Could not create the upgrade task'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="fixed inset-0 z-[60] bg-black/70 backdrop-blur-sm flex items-center justify-center p-4" @click.self="emit('close')">
    <div class="w-full max-w-sm bg-slate-900 border border-slate-800 rounded-xl shadow-2xl">
      <div class="px-5 py-4 border-b border-slate-800 flex items-center justify-between gap-2">
        <h3 class="text-sm font-bold text-slate-100 truncate">Upgrade <span class="font-mono">{{ dep.name }}</span></h3>
        <button type="button" class="text-slate-500 hover:text-slate-300" aria-label="Close" @click="emit('close')"><X class="w-4 h-4" /></button>
      </div>

      <div v-if="created" class="p-5 space-y-3 text-xs">
        <p class="text-emerald-300">A task was created to upgrade {{ dep.name }} in {{ repo }}.</p>
        <RouterLink
          :to="{ name: 'task-detail', params: { id: created.id } }"
          class="inline-flex items-center gap-1.5 text-emerald-400 hover:text-emerald-300"
          @click="emit('close')"
        >
          <ExternalLink class="w-3.5 h-3.5" /> Open “{{ created.title || created.id }}”
        </RouterLink>
      </div>

      <form v-else class="p-5 space-y-3 text-xs" @submit.prevent="submit">
        <p class="text-slate-400">
          <span class="font-mono text-slate-300">{{ repo }}</span> · {{ ecosystem }} · currently
          <span class="font-mono text-slate-200">{{ dep.installed || dep.wanted || 'unknown' }}</span>
        </p>
        <label class="block space-y-1">
          <span class="text-[11px] text-slate-400">Target version</span>
          <input v-model="target" type="text" required :placeholder="dep.latest || 'e.g. 1.2.3'"
            class="w-full h-9 px-2.5 rounded-lg bg-slate-950 border border-slate-800 font-mono text-slate-200 focus:outline-none focus:border-emerald-500" />
        </label>
        <label class="block space-y-1">
          <span class="text-[11px] text-slate-400">Note for the agents (optional)</span>
          <textarea v-model="note" rows="3" placeholder="e.g. follow the migration guide for breaking changes"
            class="w-full px-2.5 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500" />
        </label>
        <p v-if="error" class="text-rose-300">{{ error }}</p>
        <div class="flex justify-end gap-2 pt-1">
          <button type="button" class="h-8 px-3 rounded text-xs text-slate-400 hover:text-slate-200" @click="emit('close')">Cancel</button>
          <button type="submit" :disabled="saving || !target.trim()"
            class="h-8 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-semibold flex items-center gap-1.5">
            <Loader2 v-if="saving" class="w-3.5 h-3.5 animate-spin" />
            <ArrowUpCircle v-else class="w-3.5 h-3.5" />
            Create upgrade task
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
