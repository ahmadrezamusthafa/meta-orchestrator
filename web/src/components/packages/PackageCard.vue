<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  Package as PackageIcon, Download, ArrowUpCircle, History, Trash2, ChevronDown, ExternalLink, ListOrdered, HardDrive, Info,
} from 'lucide-vue-next'
import type { PackageAction, PackageDTO, PackageManual } from '../../types'
import { usePackagesStore } from '../../stores/packages'
import { isActive } from '../../services/jobTracker'
import { safeHref } from '../../utils/safeHref'
import JobLogPanel from '../common/JobLogPanel.vue'
import CopyCommand from '../common/CopyCommand.vue'
import ConfirmDeleteModal from '../common/ConfirmDeleteModal.vue'
import PackageVersionDialog from './PackageVersionDialog.vue'
import PackageHistoryDrawer from './PackageHistoryDrawer.vue'

const props = defineProps<{ pkg: PackageDTO }>()

const store = usePackagesStore()
const dialog = ref<'install' | 'rollback' | null>(null)
const confirmUninstall = ref(false)
const showHistory = ref(false)
/** Manual steps from a 501 install answer (the list's own `manual` is shown regardless). */
const manualFromInstall = ref<{ error: string; manual: PackageManual | null } | null>(null)
const starting = ref(false)

const job = computed(() => store.jobFor(props.pkg))
const busy = computed(() => starting.value || isActive(job.value))
const manual = computed(() => manualFromInstall.value?.manual || props.pkg.manual)
const docs = computed(() => safeHref(props.pkg.docs))

const statusPill = computed(() => ({
  installed: { text: 'Installed', cls: 'bg-emerald-950/80 text-emerald-300 border-emerald-800' },
  not_installed: { text: 'Not installed', cls: 'bg-slate-950 text-slate-400 border-slate-700' },
  error: { text: 'Error', cls: 'bg-rose-950/80 text-rose-300 border-rose-800' },
  checking: { text: 'Checking…', cls: 'bg-sky-950/80 text-sky-300 border-sky-800' },
}[props.pkg.status] || { text: props.pkg.status, cls: 'bg-slate-950 text-slate-400 border-slate-700' }))

const ACTIONS: PackageAction[] = ['install', 'update', 'rollback', 'uninstall']
/** Actions the daemon refuses here, with its reason (e.g. system packages cannot roll back). */
const blocked = computed(() =>
  ACTIONS.filter((a) => !props.pkg.actions?.[a] && props.pkg.action_notes?.[a]).map((a) => ({ action: a, note: props.pkg.action_notes[a] as string })),
)
function allowed(a: PackageAction): boolean {
  return !!props.pkg.actions?.[a]
}
function noteFor(a: PackageAction): string | undefined {
  return props.pkg.action_notes?.[a]
}
function shown(a: PackageAction): boolean {
  return allowed(a) || !!noteFor(a)
}
const LABEL: Record<PackageAction, string> = { install: 'Install', update: 'Update', rollback: 'Roll back', uninstall: 'Uninstall' }

async function withStart(fn: () => Promise<unknown>) {
  starting.value = true
  try {
    await fn()
  } finally {
    starting.value = false
  }
}

async function install(version = '') {
  dialog.value = null
  manualFromInstall.value = null
  await withStart(async () => {
    const res = await store.install(props.pkg, version)
    if (res) manualFromInstall.value = res
  })
}

async function rollback(version: string) {
  dialog.value = null
  await withStart(() => store.rollback(props.pkg, version || undefined))
}

async function uninstall() {
  confirmUninstall.value = false
  await withStart(() => store.uninstall(props.pkg))
}
</script>

<template>
  <article class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl flex flex-col gap-3 hover:border-slate-700 transition-colors">
    <!-- Header -->
    <div class="flex items-start justify-between gap-2">
      <div class="flex items-center gap-2.5 min-w-0">
        <div class="w-8 h-8 rounded-lg bg-slate-950 border border-slate-800 flex items-center justify-center text-emerald-400 shrink-0">
          <PackageIcon class="w-4 h-4" />
        </div>
        <div class="min-w-0">
          <h3 class="text-xs font-bold text-slate-100 truncate flex items-center gap-1.5">
            {{ pkg.name }}
            <span v-if="pkg.required" class="px-1.5 py-0.5 rounded text-[9px] uppercase tracking-wider bg-amber-950/80 text-amber-300 border border-amber-800">Required</span>
          </h3>
          <span class="text-[10px] font-mono text-slate-500 truncate block" :title="pkg.package">{{ pkg.kind }}<template v-if="pkg.package"> · {{ pkg.package }}</template></span>
        </div>
      </div>
      <span class="px-2 py-0.5 rounded text-[10px] font-mono border shrink-0" :class="statusPill.cls">{{ statusPill.text }}</span>
    </div>

    <p v-if="pkg.description" class="text-xs text-slate-400 leading-relaxed">{{ pkg.description }}</p>

    <!-- Versions and location -->
    <div class="space-y-1 text-[11px] font-mono">
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span class="text-slate-500">Active:</span>
        <span class="text-slate-200">{{ pkg.version || '—' }}</span>
        <template v-if="pkg.latest">
          <span class="text-slate-500">Latest:</span>
          <span class="text-slate-300">{{ pkg.latest }}</span>
        </template>
        <span v-if="pkg.update_available" class="px-1.5 py-0.5 rounded text-[10px] bg-sky-950/80 text-sky-300 border border-sky-800">Update available</span>
      </div>
      <div v-if="pkg.location || pkg.status === 'installed'" class="flex items-center gap-1.5 min-w-0">
        <HardDrive class="w-3 h-3 text-slate-500 shrink-0" />
        <span class="shrink-0" :class="pkg.managed ? 'text-emerald-400' : 'text-slate-400'">{{ pkg.managed ? 'Managed' : 'System' }}</span>
        <span v-if="pkg.location" class="text-slate-500 truncate" :title="pkg.location">{{ pkg.location }}</span>
      </div>
      <div v-if="pkg.system_copy" class="text-slate-500 truncate" :title="pkg.system_copy">
        Also on the system: {{ pkg.system_copy }}
      </div>
      <div v-if="pkg.requires?.length" class="text-slate-500">Needs: {{ pkg.requires.map((r) => store.nameOf(r)).join(', ') }}</div>
    </div>

    <p v-if="pkg.notes" class="text-[11px] text-slate-400 flex items-start gap-1.5"><Info class="w-3.5 h-3.5 mt-px shrink-0 text-slate-500" />{{ pkg.notes }}</p>

    <!-- How to install it by hand -->
    <div v-if="manual && (manual.commands?.length || manual.note) && pkg.status !== 'installed'" class="space-y-1.5 p-2.5 rounded-lg bg-slate-950/60 border border-slate-800">
      <p v-if="manualFromInstall?.error" class="text-[11px] text-amber-300">{{ manualFromInstall.error }}</p>
      <p class="text-[10px] uppercase tracking-wider text-slate-500 font-semibold">Install it yourself</p>
      <CopyCommand v-for="cmd in manual.commands || []" :key="cmd" :command="cmd" />
      <p v-if="manual.note" class="text-[11px] text-slate-400">{{ manual.note }}</p>
    </div>
    <p v-else-if="manualFromInstall?.error" class="text-[11px] text-amber-300">{{ manualFromInstall.error }}</p>

    <JobLogPanel v-if="job" :key="job.id" :job="job" :collapsed="!isActive(job)" />

    <!-- Actions -->
    <div class="mt-auto pt-3 border-t border-slate-800/80 space-y-2">
      <div class="flex flex-wrap items-center gap-1.5">
        <div v-if="shown('install')" class="flex">
          <button
            type="button"
            class="h-7 px-2.5 text-[11px] font-semibold flex items-center gap-1 bg-emerald-600 hover:bg-emerald-500 text-white disabled:opacity-40 disabled:cursor-not-allowed"
            :class="allowed('install') && pkg.actions.pick_version ? 'rounded-l' : 'rounded'"
            :disabled="!allowed('install') || busy"
            :title="noteFor('install')"
            @click="install()"
          >
            <Download class="w-3.5 h-3.5" /> {{ LABEL.install }}
          </button>
          <button
            v-if="allowed('install') && pkg.actions.pick_version"
            type="button"
            class="h-7 px-1.5 rounded-r bg-emerald-700 hover:bg-emerald-600 text-white border-l border-emerald-800 disabled:opacity-40"
            title="Pick a version"
            aria-label="Pick a version to install"
            :disabled="busy"
            @click="dialog = 'install'"
          >
            <ChevronDown class="w-3.5 h-3.5" />
          </button>
        </div>
        <button
          v-if="shown('update')"
          type="button"
          class="h-7 px-2.5 rounded text-[11px] font-semibold flex items-center gap-1 bg-sky-700 hover:bg-sky-600 text-white disabled:opacity-40 disabled:cursor-not-allowed"
          :disabled="!allowed('update') || busy"
          :title="noteFor('update') || (pkg.latest ? `Update to ${pkg.latest}` : undefined)"
          @click="withStart(() => store.update(pkg))"
        >
          <ArrowUpCircle class="w-3.5 h-3.5" /> {{ LABEL.update }}<template v-if="allowed('update') && pkg.latest"> to {{ pkg.latest }}</template>
        </button>
        <button
          v-if="shown('rollback')"
          type="button"
          class="h-7 px-2.5 rounded text-[11px] font-semibold flex items-center gap-1 bg-slate-800 hover:bg-slate-700 text-slate-200 disabled:opacity-40 disabled:cursor-not-allowed"
          :disabled="!allowed('rollback') || busy"
          :title="noteFor('rollback')"
          @click="dialog = 'rollback'"
        >
          <History class="w-3.5 h-3.5" /> {{ LABEL.rollback }}…
        </button>
        <button
          v-if="shown('uninstall')"
          type="button"
          class="h-7 px-2.5 rounded text-[11px] font-semibold flex items-center gap-1 bg-slate-800 hover:bg-rose-900/70 text-rose-300 disabled:opacity-40 disabled:cursor-not-allowed"
          :disabled="!allowed('uninstall') || busy"
          :title="noteFor('uninstall')"
          @click="confirmUninstall = true"
        >
          <Trash2 class="w-3.5 h-3.5" /> {{ LABEL.uninstall }}
        </button>

        <div class="ml-auto flex items-center gap-2">
          <a v-if="docs" :href="docs" target="_blank" rel="noopener noreferrer" class="text-[11px] text-slate-400 hover:text-emerald-300 flex items-center gap-1">
            <ExternalLink class="w-3.5 h-3.5" /> Docs
          </a>
          <button type="button" class="text-[11px] text-slate-400 hover:text-slate-200 flex items-center gap-1" @click="showHistory = true">
            <ListOrdered class="w-3.5 h-3.5" /> History<template v-if="pkg.history?.length"> ({{ pkg.history.length }})</template>
          </button>
        </div>
      </div>
      <ul v-if="blocked.length" class="space-y-0.5">
        <li v-for="b in blocked" :key="b.action" class="text-[10px] text-slate-500">
          <span class="text-slate-400">{{ LABEL[b.action] }}:</span> {{ b.note }}
        </li>
      </ul>
    </div>

    <PackageVersionDialog
      v-if="dialog"
      :pkg="pkg"
      :mode="dialog"
      @close="dialog = null"
      @confirm="(v: string) => (dialog === 'install' ? install(v) : rollback(v))"
    />
    <ConfirmDeleteModal
      :is-open="confirmUninstall"
      :title="`Uninstall ${pkg.name}?`"
      :message="pkg.required ? 'This package is required: tasks that need it will stop until it is installed again.' : 'The orchestrator will remove this package from this machine.'"
      :item-name="pkg.version ? `${pkg.name} ${pkg.version}` : pkg.name"
      confirm-text="Uninstall"
      :note="pkg.managed ? 'Only the orchestrator-managed copy is removed; system installs stay untouched.' : ''"
      @close="confirmUninstall = false"
      @confirm="uninstall"
    />
    <PackageHistoryDrawer v-if="showHistory" :pkg="pkg" @close="showHistory = false" />
  </article>
</template>
