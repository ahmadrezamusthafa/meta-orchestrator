<script setup lang="ts">
import { computed } from 'vue'
import { Package as PackageIcon, AlertTriangle, Loader2 } from 'lucide-vue-next'
import { usePackagesStore } from '../../stores/packages'
import type { PackageDTO } from '../../types'
import PackageCard from './PackageCard.vue'

const props = defineProps<{ search?: string; category?: string }>()
const store = usePackagesStore()

const filtered = computed(() => {
  const q = (props.search || '').trim().toLowerCase()
  return store.packages.filter((p) => {
    if (props.category && props.category !== 'All' && p.category !== props.category) return false
    if (!q) return true
    return [p.name, p.id, p.package, p.description].some((s) => (s || '').toLowerCase().includes(q))
  })
})

/** Category groups in first-seen order; required and missing packages first inside each. */
const groups = computed(() => {
  const map = new Map<string, PackageDTO[]>()
  for (const p of filtered.value) {
    const c = p.category || 'Other'
    if (!map.has(c)) map.set(c, [])
    map.get(c)!.push(p)
  }
  const rank = (p: PackageDTO) => (p.required ? 0 : 2) + (p.status === 'installed' ? 1 : 0)
  return [...map.entries()].map(([category, items]) => ({ category, items: [...items].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name)) }))
})
</script>

<template>
  <section class="space-y-4">
    <header class="flex flex-wrap items-center gap-2">
      <PackageIcon class="w-4 h-4 text-slate-400" />
      <h3 class="text-sm font-semibold text-slate-100">Packages the orchestrator uses</h3>
      <Loader2 v-if="store.loading" class="w-3.5 h-3.5 animate-spin text-slate-500" />
      <span v-if="store.data?.home" class="ml-auto text-[11px] font-mono text-slate-500 truncate" :title="store.data.home">
        Managed home: {{ store.data.home }}
      </span>
    </header>

    <div v-if="store.data && store.data.bin_dir && !store.data.on_path" class="p-3 rounded-lg border border-amber-800/60 bg-amber-950/30 text-[11px] text-amber-200 flex items-start gap-2">
      <AlertTriangle class="w-3.5 h-3.5 mt-px shrink-0" />
      <span>
        <span class="font-mono">{{ store.data.bin_dir }}</span> is not on your PATH. The orchestrator uses its managed tools either way;
        add it to your shell profile to run them from your own terminal too.
      </span>
    </div>

    <p v-if="store.error" class="text-xs text-rose-300">{{ store.error }}</p>
    <p v-else-if="!store.loading && store.data && !store.packages.length" class="text-xs text-slate-500">No packages are listed by the daemon.</p>
    <p v-else-if="store.packages.length && !filtered.length" class="text-xs text-slate-500">No package matches the filter.</p>

    <div v-for="g in groups" :key="g.category" class="space-y-2">
      <h4 class="text-[11px] uppercase tracking-wider text-slate-500 font-semibold">{{ g.category }}</h4>
      <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <PackageCard v-for="p in g.items" :key="p.id" :pkg="p" />
      </div>
    </div>
  </section>
</template>
