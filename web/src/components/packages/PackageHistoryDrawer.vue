<script setup lang="ts">
import { computed } from 'vue'
import { X, CheckCircle2, XCircle, History } from 'lucide-vue-next'
import type { PackageDTO } from '../../types'

const props = defineProps<{ pkg: PackageDTO }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const entries = computed(() => [...(props.pkg.history || [])].sort((a, b) => (b.at || '').localeCompare(a.at || '')))
const kept = computed(() =>
  [...(props.pkg.installed_versions || [])].sort((a, b) => (b.installed_at || '').localeCompare(a.installed_at || '')),
)

function when(at: string): string {
  const d = new Date(at)
  return isNaN(d.getTime()) ? at : d.toLocaleString()
}
</script>

<template>
  <div class="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex justify-end" @click.self="emit('close')">
    <aside class="w-full max-w-md h-full bg-slate-900 border-l border-slate-800 shadow-2xl flex flex-col">
      <header class="px-5 py-4 border-b border-slate-800 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <History class="w-4 h-4 text-emerald-400" />
          <h3 class="text-sm font-bold text-slate-100">{{ pkg.name }} history</h3>
        </div>
        <button type="button" class="text-slate-500 hover:text-slate-300" @click="emit('close')"><X class="w-5 h-5" /></button>
      </header>

      <div class="flex-1 overflow-y-auto p-5 space-y-6 text-xs">
        <section v-if="kept.length" class="space-y-2">
          <h4 class="text-[11px] uppercase tracking-wider text-slate-500 font-semibold">Versions on this machine</h4>
          <ul class="space-y-1">
            <li v-for="v in kept" :key="v.version" class="flex items-center gap-2 font-mono">
              <span class="text-slate-200">{{ v.version }}</span>
              <span v-if="v.active" class="px-1.5 py-0.5 rounded text-[10px] bg-emerald-950 text-emerald-300 border border-emerald-800">active</span>
              <span v-if="v.kind === 'snapshot'" class="px-1.5 py-0.5 rounded text-[10px] bg-slate-950 text-slate-400 border border-slate-700">snapshot</span>
              <span class="ml-auto text-[10px] text-slate-500">{{ when(v.installed_at) }}</span>
            </li>
          </ul>
        </section>

        <section class="space-y-2">
          <h4 class="text-[11px] uppercase tracking-wider text-slate-500 font-semibold">Changes</h4>
          <p v-if="!entries.length" class="text-slate-500">Nothing installed, updated or rolled back yet.</p>
          <ol class="space-y-2">
            <li v-for="(h, i) in entries" :key="i" class="p-2.5 rounded-lg bg-slate-950 border border-slate-800 space-y-1">
              <div class="flex items-center gap-2">
                <CheckCircle2 v-if="h.ok" class="w-3.5 h-3.5 text-emerald-400" />
                <XCircle v-else class="w-3.5 h-3.5 text-rose-400" />
                <span class="font-semibold text-slate-200 capitalize">{{ h.action }}</span>
                <span class="font-mono text-slate-400">
                  <template v-if="h.from && h.to">{{ h.from }} → {{ h.to }}</template>
                  <template v-else>{{ h.to || h.from }}</template>
                </span>
                <span class="ml-auto text-[10px] text-slate-500">{{ when(h.at) }}</span>
              </div>
              <p v-if="h.message" class="text-[11px] break-words" :class="h.ok ? 'text-slate-400' : 'text-rose-300'">{{ h.message }}</p>
            </li>
          </ol>
        </section>
      </div>
    </aside>
  </div>
</template>
