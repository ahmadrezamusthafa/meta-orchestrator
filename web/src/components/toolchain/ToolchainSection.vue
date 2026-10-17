<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RefreshCw, Wrench, CheckCircle2, AlertTriangle, Download, Loader2 } from 'lucide-vue-next'
import ToolchainList from './ToolchainList.vue'
import { useToolchainStore } from '../../stores/toolchain'
import { useToastStore } from '../../stores/toast'
import { toolchainApi, type ToolchainScope } from '../../services/toolchain'
import type { ToolchainReport } from '../../types'

const props = defineProps<{
  scope: ToolchainScope
  title?: string
  /** Shown when the scope needs nothing (e.g. no repositories assigned yet). */
  emptyText?: string
}>()

const store = useToolchainStore()
const toast = useToastStore()
const report = ref<ToolchainReport | null>(null)
const loading = ref(false)
const error = ref('')
const showAll = ref(!!props.scope.all)
const installingAll = ref(false)

const needed = computed(() => (report.value?.items || []).filter((i) => i.needed))
const others = computed(() => (report.value?.items || []).filter((i) => !i.needed))
const missingRequired = computed(() => needed.value.filter((i) => i.required && !i.installed))
const missingRecommended = computed(() => needed.value.filter((i) => !i.required && !i.installed))
const oneClick = computed(() =>
  [...missingRequired.value, ...missingRecommended.value].filter((i) =>
    (i.methods || []).some((m) => m.index === i.recommended && m.auto && m.available),
  ),
)

async function load(recheck = false) {
  loading.value = true
  error.value = ''
  try {
    const scope = { ...props.scope, all: showAll.value }
    report.value = props.scope.task && !recheck && !showAll.value
      ? await toolchainApi.taskReport(props.scope.task)
      : recheck
        ? await toolchainApi.recheck(scope)
        : await toolchainApi.report(scope)
    store.resume(report.value.items)
  } catch (err: any) {
    error.value = err?.message || 'Could not check the tools'
  } finally {
    loading.value = false
  }
}

async function installAll() {
  installingAll.value = true
  try {
    for (const item of oneClick.value) {
      if (!store.isInstalling(item.id)) await store.install(item, item.recommended)
    }
  } finally {
    installingAll.value = false
    await load(true)
  }
}

async function recheck() {
  await load(true)
  if (report.value?.ready && !missingRecommended.value.length) toast.success('Toolchain ready', 'Every tool these repositories need is installed.')
}

onMounted(() => load())
watch(() => store.revision, () => load(true))
watch(() => [props.scope.task, props.scope.project], () => load())
watch(showAll, () => load())
</script>

<template>
  <section class="space-y-3">
    <header class="flex flex-wrap items-center gap-2">
      <Wrench class="w-4 h-4 text-slate-400" />
      <h3 class="text-sm font-semibold text-slate-100">{{ title || 'Toolchain' }}</h3>
      <span v-if="report && needed.length" class="text-[11px]" :class="missingRequired.length ? 'text-amber-300' : 'text-emerald-300'">
        <template v-if="missingRequired.length">
          <AlertTriangle class="inline w-3.5 h-3.5 -mt-0.5" /> {{ missingRequired.length }} required tool{{ missingRequired.length === 1 ? '' : 's' }} missing
        </template>
        <template v-else>
          <CheckCircle2 class="inline w-3.5 h-3.5 -mt-0.5" /> Ready<template v-if="missingRecommended.length"> · {{ missingRecommended.length }} recommended missing</template>
        </template>
      </span>
      <div class="ml-auto flex items-center gap-2">
        <label class="flex items-center gap-1.5 text-[11px] text-slate-400 cursor-pointer select-none">
          <input v-model="showAll" type="checkbox" class="rounded border-slate-700 text-emerald-500 focus:ring-0" />
          Show every supported tool
        </label>
        <button
          v-if="oneClick.length"
          type="button"
          class="h-8 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold flex items-center gap-1.5 disabled:opacity-60"
          :disabled="installingAll"
          :title="oneClick.map((i) => i.name).join(', ')"
          @click="installAll"
        >
          <Loader2 v-if="installingAll" class="w-3.5 h-3.5 animate-spin" />
          <Download v-else class="w-3.5 h-3.5" />
          Install missing ({{ oneClick.length }})
        </button>
        <button
          type="button"
          class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50"
          :disabled="loading"
          title="Checks again — use it after installing from your own terminal"
          @click="recheck"
        >
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': loading }" /> Re-check
        </button>
      </div>
    </header>

    <p v-if="error" class="text-xs text-rose-300">{{ error }}</p>
    <div v-else-if="!report" class="flex items-center gap-2 text-xs text-slate-400"><Loader2 class="w-4 h-4 animate-spin" /> Checking this machine…</div>
    <template v-else>
      <p v-if="report.acknowledged?.length" class="text-[11px] text-slate-400">
        Code stages run without: <span class="text-slate-300">{{ report.acknowledged.join(', ') }}</span> — the agent skips those checks.
      </p>
      <ToolchainList v-if="needed.length" :items="needed" />
      <p v-else-if="!showAll" class="text-xs text-slate-400">{{ emptyText || 'Nothing to install: no detected stack needs command-line tools.' }}</p>
      <template v-if="showAll && others.length">
        <h4 class="pt-2 text-[11px] uppercase tracking-wider text-slate-500 font-semibold">Other supported tools</h4>
        <ToolchainList :items="others" hide-reasons />
      </template>
    </template>
  </section>
</template>
