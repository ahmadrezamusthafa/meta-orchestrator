<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import {
  Boxes, RefreshCw, Sparkles, Download, Loader2, CheckCircle2, AlertTriangle, Search, ArrowUpCircle, FolderGit2, ChevronDown, ChevronRight,
} from 'lucide-vue-next'
import type { DependencyEcosystem, RepoDependency, RepoDependencies } from '../../types'
import { useDependenciesStore, type DependencyScope } from '../../stores/dependencies'
import type { DependencyInstallResult } from '../../services/api'
import { isActive } from '../../services/jobTracker'
import JobLogPanel from '../common/JobLogPanel.vue'
import CopyCommand from '../common/CopyCommand.vue'
import DependencyUpgradeDialog from './DependencyUpgradeDialog.vue'

const props = defineProps<{
  projectId: string
  /** Limit to a task's assigned repositories (and its worktrees). */
  taskId?: string
  /** Smaller layout for the task detail: dependency tables start folded. */
  compact?: boolean
  title?: string
}>()

const store = useDependenciesStore()
const scope = computed<DependencyScope>(() => ({ projectId: props.projectId, task: props.taskId }))
const key = computed(() => store.keyOf(scope.value))
const report = computed(() => store.reports[key.value] || null)
const loading = computed(() => !!store.loading[key.value])
const error = computed(() => store.errors[key.value] || '')
const checkedLatest = ref(false)

/** Per ecosystem (repo|ecosystem): table open, show all, search text, a 501 answer. */
const open = ref<Record<string, boolean>>({})
const showAll = ref<Record<string, boolean>>({})
const search = ref<Record<string, string>>({})
const manual = ref<Record<string, Extract<DependencyInstallResult, { kind: 'manual' }>>>({})
const starting = ref<Record<string, boolean>>({})
const upgrading = ref<{ repo: string; ecosystem: string; dep: RepoDependency } | null>(null)

function ecoKey(repo: RepoDependencies, eco: DependencyEcosystem) {
  return `${repo.repo}|${eco.ecosystem}|${eco.manifest}`
}

function isOpen(k: string) {
  return open.value[k] ?? !props.compact
}

const KEY_LIMIT = 12
function rows(k: string, eco: DependencyEcosystem): RepoDependency[] {
  const deps = [...(eco.dependencies || [])].sort(
    (a, b) => Number(b.key) - Number(a.key) || Number(a.dev) - Number(b.dev) || a.name.localeCompare(b.name),
  )
  const q = (search.value[k] || '').trim().toLowerCase()
  if (q) return deps.filter((d) => d.name.toLowerCase().includes(q))
  if (showAll.value[k]) return deps
  const keyDeps = deps.filter((d) => d.key)
  return keyDeps.length ? keyDeps : deps.slice(0, KEY_LIMIT)
}

function outdated(d: RepoDependency): boolean {
  const cur = d.installed || d.wanted
  return !!d.latest && !!cur && d.latest !== cur && d.latest !== cur.replace(/^[v^~=]+/, '')
}

function statusOf(eco: DependencyEcosystem): { text: string; cls: string } {
  if (!eco.installed) return { text: 'Not installed', cls: 'bg-slate-950 text-slate-400 border-slate-700' }
  if (eco.drift) return { text: 'Drift', cls: 'bg-amber-950/80 text-amber-300 border-amber-800' }
  return { text: 'Installed', cls: 'bg-emerald-950/80 text-emerald-300 border-emerald-800' }
}

async function load(latest = false) {
  await store.load(scope.value, latest)
  if (latest) checkedLatest.value = true
}

async function install(repo: RepoDependencies, eco: DependencyEcosystem) {
  const k = ecoKey(repo, eco)
  starting.value = { ...starting.value, [k]: true }
  try {
    const res = await store.install(scope.value, repo.repo, eco.ecosystem)
    const next = { ...manual.value }
    delete next[k]
    if (res?.kind === 'manual') next[k] = res
    manual.value = next
  } finally {
    starting.value = { ...starting.value, [k]: false }
  }
}

let detach: (() => void) | null = null
onMounted(() => {
  detach = store.attach()
  void load()
})
onBeforeUnmount(() => detach?.())
watch(key, () => void load())
</script>

<template>
  <section class="space-y-3">
    <header class="flex flex-wrap items-center gap-2">
      <Boxes class="w-4 h-4 text-slate-400" />
      <h3 class="text-sm font-semibold text-slate-100">{{ title || 'Dependencies' }}</h3>
      <Loader2 v-if="loading" class="w-3.5 h-3.5 animate-spin text-slate-500" />
      <div class="ml-auto flex items-center gap-1.5">
        <button type="button" :disabled="loading" title="Ask each registry for the latest versions"
          class="h-7 px-2.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-[11px] text-slate-200 flex items-center gap-1 disabled:opacity-50"
          @click="load(true)">
          <Sparkles class="w-3.5 h-3.5 text-emerald-400" /> {{ checkedLatest ? 'Re-check latest' : 'Check latest' }}
        </button>
        <button type="button" :disabled="loading" title="Refresh" aria-label="Refresh dependencies"
          class="h-7 w-7 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 flex items-center justify-center disabled:opacity-50"
          @click="load()">
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </header>

    <p v-if="error" class="text-xs text-rose-300">{{ error }}</p>
    <p v-else-if="report && !report.repos.length" class="text-xs text-slate-500">No repository with a recognised manifest.</p>

    <article v-for="repo in report?.repos || []" :key="repo.repo" class="rounded-xl border border-slate-800 bg-slate-950/40 p-3 space-y-3">
      <div class="flex items-center gap-2 min-w-0">
        <FolderGit2 class="w-4 h-4 text-emerald-400 shrink-0" />
        <span class="text-xs font-semibold text-slate-100">{{ repo.repo }}</span>
        <span class="text-[10px] font-mono text-slate-500 truncate" :title="repo.path">{{ repo.path }}</span>
      </div>

      <JobLogPanel v-if="store.jobFor(repo)" :key="store.jobFor(repo)!.id" :job="store.jobFor(repo)!" :collapsed="!isActive(store.jobFor(repo))" />

      <p v-if="!repo.ecosystems?.length" class="text-[11px] text-slate-500">No dependency manifest found.</p>

      <div v-for="eco in repo.ecosystems || []" :key="ecoKey(repo, eco)" class="rounded-lg border border-slate-800 bg-slate-900/60 p-3 space-y-2">
        <!-- Ecosystem summary -->
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-[11px]">
          <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-200 font-mono uppercase text-[10px]">{{ eco.ecosystem }}</span>
          <span class="text-slate-300 font-mono">{{ eco.manager }}</span>
          <span class="text-slate-500 font-mono truncate" :title="eco.manifest">{{ eco.manifest }}</span>
          <span class="font-mono" :class="eco.lockfile ? 'text-slate-500' : 'text-amber-300'">{{ eco.lockfile || 'no lockfile' }}</span>
          <span class="px-1.5 py-0.5 rounded border text-[10px] font-mono" :class="statusOf(eco).cls">{{ statusOf(eco).text }}</span>

          <div class="ml-auto">
            <button
              v-if="eco.install?.auto && !eco.missing_tool"
              type="button"
              class="h-7 px-2.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white text-[11px] font-semibold flex items-center gap-1 disabled:opacity-50"
              :disabled="starting[ecoKey(repo, eco)] || isActive(store.jobFor(repo))"
              @click="install(repo, eco)"
            >
              <Loader2 v-if="starting[ecoKey(repo, eco)] || isActive(store.jobFor(repo))" class="w-3.5 h-3.5 animate-spin" />
              <Download v-else class="w-3.5 h-3.5" />
              {{ eco.installed ? 'Reinstall' : 'Install' }}
            </button>
          </div>
        </div>

        <p v-if="eco.drift" class="text-[11px] text-amber-300 flex items-start gap-1.5"><AlertTriangle class="w-3.5 h-3.5 mt-px shrink-0" />{{ eco.drift }}</p>

        <!-- Cannot run it here: say why and give the command -->
        <div v-if="eco.missing_tool || !eco.install?.auto || manual[ecoKey(repo, eco)]" class="space-y-1.5">
          <p v-if="manual[ecoKey(repo, eco)]" class="text-[11px] text-amber-300">{{ manual[ecoKey(repo, eco)].error }}</p>
          <p v-if="eco.missing_tool || manual[ecoKey(repo, eco)]?.missing_tool" class="text-[11px] text-amber-200 flex items-center gap-1.5">
            <AlertTriangle class="w-3.5 h-3.5 shrink-0" />
            <RouterLink to="/tools#toolchain" class="underline underline-offset-2 hover:text-white">
              Install {{ eco.missing_tool || manual[ecoKey(repo, eco)]?.missing_tool }} first
            </RouterLink>
          </p>
          <CopyCommand
            v-if="(manual[ecoKey(repo, eco)]?.install?.command || eco.install?.command)?.length"
            :command="(manual[ecoKey(repo, eco)]?.install?.command || eco.install.command).join(' ')"
          />
        </div>
        <p v-if="eco.install?.note" class="text-[11px] text-slate-500">{{ eco.install.note }}</p>

        <!-- Dependency table -->
        <button type="button" class="text-[11px] text-slate-400 hover:text-slate-200 flex items-center gap-1"
          @click="open = { ...open, [ecoKey(repo, eco)]: !isOpen(ecoKey(repo, eco)) }">
          <component :is="isOpen(ecoKey(repo, eco)) ? ChevronDown : ChevronRight" class="w-3.5 h-3.5" />
          Dependencies ({{ eco.dependencies?.length || 0 }})
        </button>
        <div v-if="isOpen(ecoKey(repo, eco)) && eco.dependencies?.length" class="space-y-2">
          <div class="flex flex-wrap items-center gap-2">
            <div class="relative w-56">
              <Search class="absolute left-2 top-2 w-3.5 h-3.5 text-slate-500" />
              <input
                :value="search[ecoKey(repo, eco)] || ''"
                type="text"
                placeholder="Search dependencies…"
                class="w-full h-7 pl-7 pr-2 rounded bg-slate-950 border border-slate-800 text-[11px] text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
                @input="search = { ...search, [ecoKey(repo, eco)]: ($event.target as HTMLInputElement).value }"
              />
            </div>
            <label class="flex items-center gap-1.5 text-[11px] text-slate-400">
              <input type="checkbox" class="accent-emerald-500" :checked="!!showAll[ecoKey(repo, eco)]"
                @change="showAll = { ...showAll, [ecoKey(repo, eco)]: ($event.target as HTMLInputElement).checked }" />
              Show all
            </label>
          </div>
          <div class="overflow-x-auto border border-slate-800 rounded-lg">
            <table class="w-full text-left text-[11px]">
              <thead class="bg-slate-950 text-slate-500 border-b border-slate-800">
                <tr>
                  <th class="px-2.5 py-1.5 font-medium">Name</th>
                  <th class="px-2.5 py-1.5 font-medium">Wanted</th>
                  <th class="px-2.5 py-1.5 font-medium">Installed</th>
                  <th class="px-2.5 py-1.5 font-medium">Latest</th>
                  <th class="px-2.5 py-1.5"></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-800/80 font-mono text-slate-300">
                <tr v-for="d in rows(ecoKey(repo, eco), eco)" :key="d.name">
                  <td class="px-2.5 py-1.5">
                    <span class="text-slate-100">{{ d.name }}</span>
                    <span v-if="d.key" class="ml-1.5 px-1 rounded text-[9px] bg-emerald-950 text-emerald-300 border border-emerald-800">key</span>
                    <span v-if="d.dev" class="ml-1.5 px-1 rounded text-[9px] bg-slate-900 text-slate-400 border border-slate-700">dev</span>
                  </td>
                  <td class="px-2.5 py-1.5 text-slate-400">{{ d.wanted || '—' }}</td>
                  <td class="px-2.5 py-1.5">{{ d.installed || '—' }}</td>
                  <td class="px-2.5 py-1.5" :class="outdated(d) ? 'text-sky-300' : 'text-slate-500'">
                    {{ d.latest || (checkedLatest ? '—' : '') }}
                  </td>
                  <td class="px-2.5 py-1.5 text-right">
                    <button type="button" class="text-[11px] font-sans text-slate-400 hover:text-emerald-300 inline-flex items-center gap-1"
                      @click="upgrading = { repo: repo.repo, ecosystem: eco.ecosystem, dep: d }">
                      <ArrowUpCircle class="w-3.5 h-3.5" /> Upgrade…
                    </button>
                  </td>
                </tr>
                <tr v-if="!rows(ecoKey(repo, eco), eco).length">
                  <td colspan="5" class="px-2.5 py-2 text-slate-500 font-sans">No dependency matches.</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-if="!showAll[ecoKey(repo, eco)] && !search[ecoKey(repo, eco)] && rows(ecoKey(repo, eco), eco).length < eco.dependencies.length"
            class="text-[10px] text-slate-500">
            Showing {{ rows(ecoKey(repo, eco), eco).length }} of {{ eco.dependencies.length }}; tick “Show all” or search for the rest.
          </p>
        </div>
        <p v-else-if="isOpen(ecoKey(repo, eco))" class="text-[11px] text-slate-500 flex items-center gap-1.5">
          <CheckCircle2 class="w-3.5 h-3.5" /> No dependencies declared.
        </p>
      </div>
    </article>

    <DependencyUpgradeDialog
      v-if="upgrading"
      :project-id="projectId"
      :repo="upgrading.repo"
      :ecosystem="upgrading.ecosystem"
      :dep="upgrading.dep"
      @close="upgrading = null"
    />
  </section>
</template>
