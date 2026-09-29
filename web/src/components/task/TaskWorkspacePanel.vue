<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../../services/api'
import { useProjectStore } from '../../stores/projects'
import { useTaskStore } from '../../stores/tasks'
import { useToastStore } from '../../stores/toast'
import type { Task, TaskWorktreeDTO, RepoDiffDTO } from '../../types'
import {
  FolderGit2, GitBranch, Copy, Check, AlertCircle, FileDiff, RefreshCw, Pencil, ShieldCheck, CircleDashed,
} from 'lucide-vue-next'

const props = defineProps<{
  task: Task
  worktree: TaskWorktreeDTO | null
}>()
const emit = defineEmits<{ (e: 'open-changes'): void; (e: 'refresh'): void }>()

const projectStore = useProjectStore()
const taskStore = useTaskStore()
const toast = useToastStore()

const diffs = ref<Record<string, RepoDiffDTO>>({})
const loadingDiff = ref(false)
const copied = ref('')
const editing = ref(false)
const draftRepos = ref<string[]>([])
const saving = ref(false)

const repos = computed(() => props.worktree?.repos || [])
const running = computed(() => props.task.state === 'RUNNING')

// Registered repositories, with the project they belong to.
const catalog = computed(() => {
  const list: { name: string; project: string; role: string; path: string; manifest: string }[] = []
  for (const p of projectStore.projects) {
    for (const r of p.repos || []) list.push({ name: r.name, project: p.name, role: r.role, path: r.path, manifest: r.manifest_type })
  }
  return list
})
const repoInfo = (name: string) => catalog.value.find((r) => r.name === name)

async function loadDiffs() {
  if (!repos.value.some((r) => r.exists)) {
    diffs.value = {}
    return
  }
  loadingDiff.value = true
  try {
    const res = await api.getTaskDiff(props.task.id, 'base')
    diffs.value = Object.fromEntries(res.repos.map((r) => [r.repo, r]))
  } catch {
    diffs.value = {}
  } finally {
    loadingDiff.value = false
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = text
    setTimeout(() => (copied.value = ''), 1400)
  } catch {
    /* clipboard unavailable */
  }
}

function startEdit() {
  draftRepos.value = [...(props.task.assigned_repos || [])]
  editing.value = true
}

function toggleDraft(name: string) {
  draftRepos.value = draftRepos.value.includes(name) ? draftRepos.value.filter((r) => r !== name) : [...draftRepos.value, name]
}

async function saveRepos() {
  saving.value = true
  try {
    const updated = await api.patchTask(props.task.id, { assigned_repos: draftRepos.value })
    await taskStore.fetchTask(updated.id)
    editing.value = false
    toast.success('Repositories updated', draftRepos.value.length ? 'Worktrees are created on the next run.' : 'No repositories assigned.')
    emit('refresh')
  } catch (err: any) {
    toast.error('Could not update repositories', err?.message || 'Unknown error')
  } finally {
    saving.value = false
  }
}

function statusOf(r: { exists: boolean; error?: string }) {
  if (r.error) return { label: r.error.includes('not registered') ? 'Not registered' : 'Unavailable', cls: 'bg-rose-950/60 border-rose-800/60 text-rose-300' }
  if (r.exists) return { label: 'Worktree active', cls: 'bg-teal-950/70 border-teal-700/70 text-teal-300' }
  return { label: 'Created on first run', cls: 'bg-slate-800 border-slate-700 text-slate-300' }
}

watch(() => props.worktree, loadDiffs)
onMounted(async () => {
  if (!projectStore.projects.length) await projectStore.fetchProjects()
  loadDiffs()
})
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="max-w-5xl mx-auto p-5 space-y-4">
      <!-- Isolation summary -->
      <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 flex flex-wrap items-start gap-4">
        <div class="flex-1 min-w-[240px] space-y-1">
          <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
            <ShieldCheck class="w-4 h-4 text-teal-400" /> Isolated workspace
          </h3>
          <p class="text-xs text-slate-400 leading-relaxed">
            <template v-if="worktree?.status === 'DISABLED'">Worktrees are disabled for this task — the agent works directly in the source checkouts below.</template>
            <template v-else>The agent edits a dedicated git worktree per repository on its own branch. Your checkouts and their uncommitted work are never touched.</template>
          </p>
        </div>
        <div class="text-xs space-y-1.5">
          <div class="flex items-center gap-2">
            <GitBranch class="w-3.5 h-3.5 text-teal-400" />
            <span class="font-mono text-teal-300">{{ worktree?.branch || task.metadata?.worktree_branch || '—' }}</span>
            <button type="button" v-if="worktree?.branch" @click="copy(worktree.branch)" aria-label="Copy branch name" class="text-slate-500 hover:text-slate-200">
              <Check v-if="copied === worktree.branch" class="w-3 h-3 text-emerald-400" /><Copy v-else class="w-3 h-3" />
            </button>
          </div>
          <div class="text-slate-500">{{ worktree?.status === 'ACTIVE' ? 'Branch created' : 'Branch is created when the task first runs' }}</div>
        </div>
      </section>

      <div v-if="worktree?.error" class="p-3 rounded-lg border border-amber-800/60 bg-amber-950/30 text-xs text-amber-200 flex gap-2">
        <AlertCircle class="w-4 h-4 flex-shrink-0" /> {{ worktree.error }}
      </div>

      <!-- Repositories -->
      <section class="space-y-2">
        <div class="flex items-center justify-between">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-400">Repositories ({{ task.assigned_repos?.length || 0 }})</h3>
          <div class="flex items-center gap-2">
            <button type="button" @click="loadDiffs(); emit('refresh')" aria-label="Refresh" class="p-1.5 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800">
              <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': loadingDiff }" />
            </button>
            <button v-if="!editing" type="button" @click="startEdit" :disabled="running" :title="running ? 'Pause the task to change repositories' : ''"
              class="h-7 px-2.5 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-xs text-slate-200 flex items-center gap-1.5">
              <Pencil class="w-3 h-3" /> Edit
            </button>
          </div>
        </div>

        <!-- Editor -->
        <div v-if="editing" class="p-4 rounded-xl border border-blue-800/60 bg-blue-950/10 space-y-3">
          <p class="text-xs text-slate-300">Choose the repositories the agent may change. Only repositories registered in a project can be assigned.</p>
          <p v-if="catalog.length === 0" class="text-xs text-slate-500">
            No repositories are registered. Add one in <router-link to="/projects" class="underline text-slate-300">Projects</router-link>.
          </p>
          <div class="grid sm:grid-cols-2 gap-2">
            <label v-for="r in catalog" :key="r.name"
              class="flex items-start gap-2.5 p-2.5 rounded-lg border cursor-pointer text-xs"
              :class="draftRepos.includes(r.name) ? 'border-blue-600 bg-blue-950/40' : 'border-slate-800 bg-slate-950 hover:border-slate-700'">
              <input type="checkbox" :checked="draftRepos.includes(r.name)" @change="toggleDraft(r.name)" class="mt-0.5 accent-blue-500" />
              <span class="min-w-0">
                <span class="block font-mono text-slate-100">{{ r.name }}</span>
                <span class="block text-[11px] text-slate-500 truncate" :title="r.path">{{ r.project }} · {{ r.role }} · {{ r.path }}</span>
              </span>
            </label>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" @click="editing = false" class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs text-slate-300">Cancel</button>
            <button type="button" @click="saveRepos" :disabled="saving" class="h-8 px-3 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-xs font-medium text-white">
              {{ saving ? 'Saving…' : 'Save repositories' }}
            </button>
          </div>
        </div>

        <div v-if="!editing && repos.length === 0" class="p-8 rounded-xl border border-dashed border-slate-800 text-center text-xs text-slate-400 space-y-2">
          <FolderGit2 class="w-6 h-6 mx-auto text-slate-600" />
          <p>No repositories assigned. The agent can discuss the task but has no code to change.</p>
          <button type="button" @click="startEdit" :disabled="running" class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200">Assign repositories</button>
        </div>

        <article v-for="r in repos" :key="r.repo" class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
          <header class="flex flex-wrap items-center gap-2">
            <FolderGit2 class="w-4 h-4 text-slate-400" />
            <span class="font-mono text-sm text-slate-100">{{ r.repo }}</span>
            <span v-if="repoInfo(r.repo)" class="text-[11px] text-slate-500">{{ repoInfo(r.repo)!.project }} · {{ repoInfo(r.repo)!.role }}<template v-if="repoInfo(r.repo)!.manifest"> · {{ repoInfo(r.repo)!.manifest }}</template></span>
            <span class="ml-auto px-1.5 py-0.5 rounded border text-[10px] font-semibold" :class="statusOf(r).cls">{{ statusOf(r).label }}</span>
            <span v-if="r.dirty" class="px-1.5 py-0.5 rounded border border-amber-800/60 bg-amber-950/50 text-amber-300 text-[10px]">Uncommitted changes</span>
          </header>

          <dl class="grid sm:grid-cols-2 gap-x-6 gap-y-2 text-xs">
            <div class="min-w-0">
              <dt class="text-[10px] uppercase tracking-wider text-slate-500">Source checkout</dt>
              <dd class="flex items-center gap-1.5 font-mono text-slate-300 min-w-0">
                <span class="truncate" :title="r.source_path">{{ r.source_path || '—' }}</span>
                <button v-if="r.source_path" type="button" @click="copy(r.source_path)" aria-label="Copy source path" class="text-slate-500 hover:text-slate-200 flex-shrink-0">
                  <Check v-if="copied === r.source_path" class="w-3 h-3 text-emerald-400" /><Copy v-else class="w-3 h-3" />
                </button>
              </dd>
            </div>
            <div class="min-w-0">
              <dt class="text-[10px] uppercase tracking-wider text-slate-500">Agent worktree</dt>
              <dd class="flex items-center gap-1.5 font-mono min-w-0" :class="r.exists ? 'text-teal-300' : 'text-slate-500'">
                <span class="truncate" :title="r.worktree_path">{{ r.worktree_path || '—' }}</span>
                <button v-if="r.exists" type="button" @click="copy(r.worktree_path)" aria-label="Copy worktree path" class="text-slate-500 hover:text-slate-200 flex-shrink-0">
                  <Check v-if="copied === r.worktree_path" class="w-3 h-3 text-emerald-400" /><Copy v-else class="w-3 h-3" />
                </button>
              </dd>
            </div>
            <div>
              <dt class="text-[10px] uppercase tracking-wider text-slate-500">Based on</dt>
              <dd class="font-mono text-slate-300">{{ r.base_ref || 'default branch (resolved on first run)' }}</dd>
            </div>
            <div>
              <dt class="text-[10px] uppercase tracking-wider text-slate-500">Changes vs base</dt>
              <dd class="text-slate-300">
                <template v-if="!r.exists"><CircleDashed class="inline w-3 h-3 mr-1 text-slate-600" />None yet</template>
                <template v-else-if="diffs[r.repo]?.error">{{ diffs[r.repo].error }}</template>
                <template v-else-if="diffs[r.repo]">
                  {{ diffs[r.repo].files?.length || 0 }} files
                  <span class="text-emerald-400 font-mono">+{{ diffs[r.repo].additions || 0 }}</span>
                  <span class="text-rose-400 font-mono">−{{ diffs[r.repo].deletions || 0 }}</span>
                  · {{ diffs[r.repo].commits?.length || 0 }} commits
                </template>
                <template v-else>…</template>
              </dd>
            </div>
          </dl>

          <div v-if="r.error" class="text-[11px] text-rose-300">{{ r.error }}</div>
          <div v-if="r.exists" class="flex justify-end">
            <button type="button" @click="emit('open-changes')" class="h-7 px-2.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5">
              <FileDiff class="w-3.5 h-3.5" /> View changes
            </button>
          </div>
        </article>
      </section>
    </div>
  </div>
</template>
