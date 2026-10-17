<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Rocket, X, Loader2, CheckCircle2, XCircle, Clock, AlertTriangle } from 'lucide-vue-next'
import { usePackagesStore } from '../../stores/packages'
import JobLogPanel from '../common/JobLogPanel.vue'

/** compact: one line for the main board, linking to the Tool Hub. */
const props = defineProps<{ compact?: boolean }>()

const store = usePackagesStore()
const DISMISS_KEY = 'meta_orch_setup_banner_dismissed'

/** Dismissal is tied to what was missing: a newly missing package brings the banner back. */
const signature = computed(() => store.missingRequired.map((p) => p.id).sort().join(','))
const dismissed = ref(readDismissed())

function readDismissed(): string {
  try {
    return localStorage.getItem(DISMISS_KEY) || ''
  } catch {
    return ''
  }
}

function dismiss() {
  dismissed.value = signature.value || 'done'
  try {
    localStorage.setItem(DISMISS_KEY, dismissed.value)
  } catch {
    // storage unavailable: dismissed for this page view only
  }
}

const hasSetup = computed(() => store.setupJobs.length > 0 || store.setupSkipped.length > 0)
const visible = computed(() => {
  if (store.setupRunning) return true
  if (!store.missingRequired.length && !hasSetup.value) return false
  return dismissed.value !== (signature.value || 'done')
})
const doneCount = computed(() => store.setupJobs.filter((j) => j.state === 'succeeded').length)

let detach: (() => void) | null = null
onMounted(() => {
  detach = store.attach()
  if (!store.data && !store.loading) void store.fetch()
})
onBeforeUnmount(() => detach?.())

async function setup() {
  dismissed.value = '' // follow this run's progress even if an earlier one was dismissed
  await store.setup()
}
</script>

<template>
  <div v-if="visible" class="rounded-xl border border-amber-800/60 bg-amber-950/30" :class="props.compact ? 'px-4 py-2.5' : 'p-4 space-y-3'">
    <div class="flex flex-wrap items-center gap-3">
      <Rocket class="w-4 h-4 text-amber-300 shrink-0" />
      <div class="min-w-0 flex-1 text-xs">
        <template v-if="store.missingRequired.length">
          <span class="font-semibold text-amber-200">
            {{ store.missingRequired.length }} required {{ store.missingRequired.length === 1 ? 'tool is' : 'tools are' }} not installed:
          </span>
          <span class="text-amber-100/80"> {{ store.missingRequired.map((p) => p.name).join(', ') }}</span>
        </template>
        <span v-else class="font-semibold text-emerald-300">Every required tool is installed.</span>
        <span v-if="store.setupJobs.length" class="ml-2 text-slate-400">({{ doneCount }}/{{ store.setupJobs.length }} done)</span>
      </div>
      <RouterLink v-if="props.compact" to="/tools" class="text-[11px] text-amber-200 hover:text-white underline-offset-2 hover:underline">Details</RouterLink>
      <button
        v-if="store.missingRequired.length"
        type="button"
        class="h-8 px-3 rounded-lg bg-amber-500 hover:bg-amber-400 text-slate-950 text-xs font-bold flex items-center gap-1.5 disabled:opacity-60"
        :disabled="store.settingUp || store.setupRunning"
        @click="setup"
      >
        <Loader2 v-if="store.settingUp || store.setupRunning" class="w-3.5 h-3.5 animate-spin" />
        <Rocket v-else class="w-3.5 h-3.5" />
        {{ store.setupRunning ? 'Setting up…' : 'Set up everything' }}
      </button>
      <button type="button" class="text-amber-300/70 hover:text-amber-100" title="Dismiss" aria-label="Dismiss" @click="dismiss">
        <X class="w-4 h-4" />
      </button>
    </div>

    <!-- Progress of each job in the setup chain -->
    <template v-if="hasSetup">
      <ul v-if="props.compact" class="mt-2 flex flex-wrap gap-1.5">
        <li v-for="j in store.setupJobs" :key="j.id" class="px-2 py-0.5 rounded border text-[10px] font-mono flex items-center gap-1"
          :class="{
            'border-slate-700 text-slate-300': j.state === 'queued',
            'border-sky-800 text-sky-300': j.state === 'running',
            'border-emerald-800 text-emerald-300': j.state === 'succeeded',
            'border-rose-800 text-rose-300': j.state === 'failed',
          }">
          <Loader2 v-if="j.state === 'running'" class="w-3 h-3 animate-spin" />
          <Clock v-else-if="j.state === 'queued'" class="w-3 h-3" />
          <CheckCircle2 v-else-if="j.state === 'succeeded'" class="w-3 h-3" />
          <XCircle v-else class="w-3 h-3" />
          {{ store.nameOf(j.package) }}
        </li>
      </ul>
      <div v-else class="space-y-2">
        <JobLogPanel v-for="j in store.setupJobs" :key="j.id" :job="j" :label="store.nameOf(j.package)" :collapsed="j.state !== 'running'" />
      </div>
      <ul v-if="store.setupSkipped.length && !props.compact" class="space-y-1">
        <li v-for="s in store.setupSkipped" :key="s.id" class="text-[11px] text-amber-200/80 flex items-start gap-1.5">
          <AlertTriangle class="w-3.5 h-3.5 mt-px shrink-0" />
          <span><span class="font-semibold">{{ store.nameOf(s.id) }}</span> skipped: {{ s.reason }}</span>
        </li>
      </ul>
    </template>
  </div>
</template>
