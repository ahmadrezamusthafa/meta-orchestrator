<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Wrench, X, RefreshCw, Download, Play, Loader2, ChevronDown, ChevronRight } from 'lucide-vue-next'
import ToolchainList from './ToolchainList.vue'
import { useToolchainStore } from '../../stores/toolchain'
import { useTaskStore } from '../../stores/tasks'
import { useToastStore } from '../../stores/toast'
import { toolchainApi } from '../../services/toolchain'
import { stageName } from '../../composables/taskLifecycle'

const store = useToolchainStore()
const tasks = useTaskStore()
const toast = useToastStore()

const rechecking = ref(false)
const installingAll = ref(false)
const starting = ref(false)
const showRecommended = ref(false)

const prompt = computed(() => store.prompt)
const report = computed(() => prompt.value?.report || null)
const required = computed(() => (report.value?.items || []).filter((i) => i.needed && i.required))
const recommended = computed(() => (report.value?.items || []).filter((i) => i.needed && !i.required))
const missingRequired = computed(() => required.value.filter((i) => !i.installed))
const oneClick = computed(() =>
  missingRequired.value.filter((i) => (i.methods || []).some((m) => m.index === i.recommended && m.auto && m.available)),
)
const stage = computed(() => (prompt.value?.stageId ? stageName(prompt.value.stageId) : 'this stage'))
const busy = computed(() => installingAll.value || starting.value || missingRequired.value.some((i) => store.isInstalling(i.id)))

async function recheck(quiet = false) {
  if (!prompt.value) return
  rechecking.value = true
  try {
    const fresh = await toolchainApi.recheck({ task: prompt.value.taskId })
    if (store.prompt) store.prompt = { ...store.prompt, report: fresh }
    if (!quiet && fresh.ready) toast.success('All required tools are installed', `You can run ${stage.value} now.`)
  } catch (err: any) {
    toast.error('Re-check failed', err?.message)
  } finally {
    rechecking.value = false
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && store.prompt) store.closePrompt()
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

// An install finishing anywhere refreshes what this prompt shows.
watch(() => store.revision, () => recheck(true))

async function installAll() {
  installingAll.value = true
  try {
    for (const item of oneClick.value) {
      if (!store.isInstalling(item.id)) await store.install(item, item.recommended) // one at a time: package managers lock
    }
  } finally {
    installingAll.value = false
    await recheck(true)
  }
}

async function start() {
  const p = prompt.value
  if (!p) return
  starting.value = true
  store.closePrompt() // a still-missing tool reopens it with the fresh report
  try {
    await tasks.performTaskAction(p.taskId, p.retry)
  } catch {
    // performTaskAction already reported it
  } finally {
    starting.value = false
  }
}

async function runWithout() {
  const p = prompt.value
  if (!p) return
  starting.value = true
  try {
    await toolchainApi.acknowledge(p.taskId)
  } catch (err: any) {
    toast.error('Could not save your choice', err?.message)
    starting.value = false
    return
  }
  starting.value = false
  await start()
}
</script>

<template>
  <div
    v-if="prompt"
    class="fixed inset-0 z-[80] bg-black/75 backdrop-blur-sm flex items-center justify-center p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="toolchain-prompt-title"
    @click.self="store.closePrompt()"
  >
    <div class="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-2xl max-h-[88vh] flex flex-col shadow-2xl ring-1 ring-amber-500/20">
      <!-- Header -->
      <div class="p-5 border-b border-slate-800 flex items-start justify-between gap-3">
        <div class="flex items-start gap-3">
          <div class="p-2.5 rounded-xl bg-amber-500/10 text-amber-400 border border-amber-500/20">
            <Wrench class="w-5 h-5" />
          </div>
          <div>
            <h3 id="toolchain-prompt-title" class="font-bold text-base text-white">Install tools before running {{ stage }}</h3>
            <p class="text-xs text-slate-400 mt-1 leading-relaxed">
              {{ prompt.taskId }} verifies its changes with these tools, and this machine does not have
              <template v-if="missingRequired.length === 1">one of them</template><template v-else>some of them</template>.
              Install them here — one click where possible — then run the stage.
            </p>
          </div>
        </div>
        <button type="button" class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800" aria-label="Close" @click="store.closePrompt()">
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 overflow-y-auto space-y-4">
        <div v-if="!report" class="flex items-center gap-2 text-xs text-slate-400">
          <Loader2 class="w-4 h-4 animate-spin" /> Checking this machine…
        </div>
        <template v-else>
          <div class="flex items-center justify-between text-xs">
            <span class="font-semibold text-slate-200">Required</span>
            <span :class="missingRequired.length ? 'text-amber-300' : 'text-emerald-300'">
              {{ required.length - missingRequired.length }} of {{ required.length }} ready
            </span>
          </div>
          <ToolchainList :items="required" />

          <div v-if="recommended.length">
            <button
              type="button"
              class="text-xs text-slate-400 hover:text-slate-200 flex items-center gap-1"
              @click="showRecommended = !showRecommended"
            >
              <ChevronDown v-if="showRecommended" class="w-3.5 h-3.5" />
              <ChevronRight v-else class="w-3.5 h-3.5" />
              Also recommended ({{ recommended.filter((i) => !i.installed).length }} missing) — the agent skips these checks without them
            </button>
            <ToolchainList v-if="showRecommended" class="mt-2" :items="recommended" />
          </div>
        </template>
      </div>

      <!-- Footer -->
      <div class="p-4 border-t border-slate-800 bg-slate-950/60 flex flex-wrap items-center justify-between gap-2">
        <button
          type="button"
          class="text-xs text-slate-400 hover:text-slate-200 underline-offset-2 hover:underline disabled:opacity-50"
          :disabled="busy || !report"
          title="The agent skips the checks that need the missing tools and lists them in its summary"
          @click="runWithout"
        >
          Run without them
        </button>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="h-9 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50"
            :disabled="rechecking || !report"
            title="Checks again — use it after installing from your own terminal"
            @click="recheck()"
          >
            <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': rechecking }" /> Re-check
          </button>
          <button
            v-if="report && !report.ready && oneClick.length"
            type="button"
            class="h-9 px-3.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold flex items-center gap-1.5 disabled:opacity-60"
            :disabled="busy"
            @click="installAll"
          >
            <Loader2 v-if="installingAll" class="w-3.5 h-3.5 animate-spin" />
            <Download v-else class="w-3.5 h-3.5" />
            {{ installingAll ? 'Installing…' : oneClick.length === missingRequired.length ? `Install all (${oneClick.length})` : `Install ${oneClick.length} automatically` }}
          </button>
          <button
            type="button"
            class="h-9 px-3.5 rounded-lg text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
            :class="report?.ready ? 'bg-emerald-600 hover:bg-emerald-500 text-white' : 'bg-slate-800 text-slate-400'"
            :disabled="!report?.ready || busy"
            :title="report?.ready ? '' : 'Install the required tools first, or choose Run without them'"
            @click="start"
          >
            <Play class="w-3.5 h-3.5" /> Run {{ stage }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
