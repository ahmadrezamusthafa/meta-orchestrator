<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Plug, CheckCircle2, CircleDashed, AlertTriangle, ArrowRight, Loader2 } from 'lucide-vue-next'
import type { ConnectorSuggestion } from '../../types'
import { toolchainApi, type ToolchainScope } from '../../services/toolchain'

const props = defineProps<{ scope: ToolchainScope; title?: string }>()
const router = useRouter()
const items = ref<ConnectorSuggestion[] | null>(null)
const error = ref('')

const readyCount = computed(() => (items.value || []).filter((s) => s.ready).length)

async function load() {
  error.value = ''
  try {
    items.value = await toolchainApi.connectorSuggestions(props.scope)
  } catch (err: any) {
    error.value = err?.message || 'Could not load connector suggestions'
  }
}

function state(s: ConnectorSuggestion): { label: string; cls: string; hint: string } {
  if (s.ready) return { label: 'Agents use it', cls: 'text-emerald-300 border-emerald-800/60 bg-emerald-950/40', hint: '' }
  if (s.status === 'error') return { label: 'Test failed', cls: 'text-rose-300 border-rose-800/60 bg-rose-950/40', hint: 'Open it and run Test to see why.' }
  if (s.enabled) return { label: 'Needs credentials', cls: 'text-amber-300 border-amber-800/60 bg-amber-950/40', hint: 'Enabled, but not usable yet.' }
  return { label: 'Not connected', cls: 'text-slate-400 border-slate-700 bg-slate-900', hint: '' }
}

function connect(id: string) {
  router.push({ path: '/connectors', query: { focus: id } })
}

onMounted(load)
watch(() => [props.scope.task, props.scope.project], load)
</script>

<template>
  <section class="space-y-3">
    <header class="flex flex-wrap items-center gap-2">
      <Plug class="w-4 h-4 text-slate-400" />
      <h3 class="text-sm font-semibold text-slate-100">{{ title || 'Connectors these repositories use' }}</h3>
      <span v-if="items?.length" class="text-[11px]" :class="readyCount === items.length ? 'text-emerald-300' : 'text-slate-400'">
        {{ readyCount }} of {{ items.length }} available to agents
      </span>
    </header>

    <p v-if="error" class="text-xs text-rose-300">{{ error }}</p>
    <div v-else-if="!items" class="flex items-center gap-2 text-xs text-slate-400"><Loader2 class="w-4 h-4 animate-spin" /> Checking…</div>
    <p v-else-if="!items.length" class="text-xs text-slate-400">
      No third-party service detected for these repositories. Connectors are optional — the agent works from the repositories alone.
    </p>
    <ul v-else class="space-y-2">
      <li v-for="s in items" :key="s.id" class="rounded-xl border border-slate-800 bg-slate-900/60 p-3.5 text-xs flex items-start gap-3">
        <CheckCircle2 v-if="s.ready" class="w-4 h-4 text-emerald-400 mt-0.5 flex-shrink-0" />
        <AlertTriangle v-else-if="s.status === 'error' || s.enabled" class="w-4 h-4 text-amber-400 mt-0.5 flex-shrink-0" />
        <CircleDashed v-else class="w-4 h-4 text-slate-500 mt-0.5 flex-shrink-0" />
        <div class="min-w-0 flex-1 space-y-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-semibold text-slate-100 text-[13px]">{{ s.name }}</span>
            <span class="px-1.5 py-0.5 rounded border text-[10px]" :class="state(s).cls">{{ state(s).label }}</span>
          </div>
          <ul class="text-[11px] text-slate-500 space-y-0.5">
            <li v-for="r in s.reasons" :key="r">• {{ r }}</li>
          </ul>
          <p v-if="s.access" class="text-[11px] text-purple-300/80">Agents: {{ s.access }}</p>
          <p v-if="state(s).hint" class="text-[11px] text-slate-400">{{ state(s).hint }}</p>
        </div>
        <button
          v-if="!s.ready"
          type="button"
          class="h-8 px-3 rounded-lg bg-sky-600 hover:bg-sky-500 text-white font-semibold flex items-center gap-1.5 flex-shrink-0"
          @click="connect(s.id)"
        >
          {{ s.enabled ? 'Finish setup' : 'Connect' }} <ArrowRight class="w-3.5 h-3.5" />
        </button>
        <button
          v-else
          type="button"
          class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 flex-shrink-0"
          @click="connect(s.id)"
        >Settings</button>
      </li>
    </ul>
  </section>
</template>
