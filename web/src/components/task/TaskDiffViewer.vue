<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '../../services/api'
import type { DiffAgainst, RepoDiffDTO, TaskWorktreeDTO } from '../../types'
import { GitBranch, GitCommit, RefreshCw, ChevronDown, ChevronRight, FileDiff, AlertCircle, Columns2, AlignJustify } from 'lucide-vue-next'

const props = defineProps<{
  taskId: string
  worktree?: TaskWorktreeDTO | null
}>()

type Line = { kind: 'add' | 'del' | 'ctx' | 'hunk' | 'meta'; text: string; oldNo?: number; newNo?: number }
type FilePatch = { path: string; lines: Line[] }

const MAX_RENDERED_LINES = 4000 // per file; larger files are collapsed behind a button

const against = ref<DiffAgainst>('base')
const selectedRepo = ref<string>('')
const repos = ref<RepoDiffDTO[]>([])
const isLoading = ref(false)
const loadError = ref('')
const collapsed = ref<Record<string, boolean>>({})
const expandedLarge = ref<Record<string, boolean>>({})
const wrap = ref(false)

const current = computed(() => repos.value.find((r) => r.repo === selectedRepo.value) || repos.value[0] || null)

function parsePatch(patch: string): FilePatch[] {
  const files: FilePatch[] = []
  let file: FilePatch | null = null
  let oldNo = 0
  let newNo = 0
  for (const raw of patch.split('\n')) {
    if (raw.startsWith('diff --git ')) {
      const m = raw.match(/ b\/(.+)$/)
      file = { path: m ? m[1] : raw.slice(11), lines: [] }
      files.push(file)
      continue
    }
    if (!file) continue
    if (raw.startsWith('@@')) {
      const m = raw.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/)
      oldNo = m ? Number(m[1]) : 0
      newNo = m ? Number(m[2]) : 0
      file.lines.push({ kind: 'hunk', text: raw })
    } else if (raw.startsWith('+++') || raw.startsWith('---') || raw.startsWith('index ') ||
      raw.startsWith('new file') || raw.startsWith('deleted file') || raw.startsWith('similarity') ||
      raw.startsWith('rename ') || raw.startsWith('old mode') || raw.startsWith('new mode')) {
      continue
    } else if (raw.startsWith('Binary files')) {
      file.lines.push({ kind: 'meta', text: 'Binary file changed' })
    } else if (raw.startsWith('+')) {
      file.lines.push({ kind: 'add', text: raw.slice(1), newNo: newNo++ })
    } else if (raw.startsWith('-')) {
      file.lines.push({ kind: 'del', text: raw.slice(1), oldNo: oldNo++ })
    } else if (raw.startsWith('\\')) {
      file.lines.push({ kind: 'meta', text: raw.slice(2) })
    } else if (file.lines.length > 0) {
      file.lines.push({ kind: 'ctx', text: raw.slice(1), oldNo: oldNo++, newNo: newNo++ })
    }
  }
  return files
}

const patches = computed(() => {
  const map: Record<string, FilePatch> = {}
  for (const f of parsePatch(current.value?.patch || '')) map[f.path] = f
  return map
})

const STATUS_STYLE: Record<string, string> = {
  added: 'text-emerald-300 bg-emerald-950/60 border-emerald-800/60',
  untracked: 'text-emerald-300 bg-emerald-950/60 border-emerald-800/60',
  modified: 'text-amber-300 bg-amber-950/50 border-amber-800/60',
  deleted: 'text-rose-300 bg-rose-950/50 border-rose-800/60',
  renamed: 'text-sky-300 bg-sky-950/50 border-sky-800/60',
}
const STATUS_LETTER: Record<string, string> = { added: 'A', untracked: 'U', modified: 'M', deleted: 'D', renamed: 'R' }

async function load() {
  isLoading.value = true
  loadError.value = ''
  try {
    const res = await api.getTaskDiff(props.taskId, against.value)
    repos.value = res.repos
    if (!res.repos.some((r) => r.repo === selectedRepo.value)) selectedRepo.value = res.repos[0]?.repo || ''
  } catch (err: any) {
    loadError.value = err?.message || 'Failed to load changes'
  } finally {
    isLoading.value = false
  }
}

function scrollToFile(path: string) {
  collapsed.value[path] = false
  document.getElementById(`diff-${path}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

watch(against, load)
watch(() => props.taskId, load)
onMounted(load)
</script>

<template>
  <div class="h-full flex flex-col min-h-0">
    <!-- Controls -->
    <div class="px-4 py-2.5 border-b border-slate-800 flex flex-wrap items-center gap-3 text-xs">
      <div class="flex items-center gap-0.5 p-0.5 rounded-lg bg-slate-950 border border-slate-800" role="group" aria-label="Compare against">
        <button type="button" @click="against = 'base'" :aria-pressed="against === 'base'"
          class="px-2.5 py-1 rounded"
          :class="against === 'base' ? 'bg-slate-800 text-slate-100 font-semibold' : 'text-slate-400 hover:text-slate-200'">
          vs {{ current?.base_ref || worktree?.base_ref || 'base branch' }}
        </button>
        <button type="button" @click="against = 'head'" :aria-pressed="against === 'head'"
          class="px-2.5 py-1 rounded"
          :class="against === 'head' ? 'bg-slate-800 text-slate-100 font-semibold' : 'text-slate-400 hover:text-slate-200'">
          Uncommitted (vs HEAD)
        </button>
      </div>

      <select v-if="repos.length > 1" v-model="selectedRepo" aria-label="Repository"
        class="h-7 px-2 bg-slate-950 border border-slate-800 rounded-lg text-slate-200 focus:outline-none focus:border-blue-500">
        <option v-for="r in repos" :key="r.repo" :value="r.repo">
          {{ r.repo }}{{ r.files?.length ? ` (${r.files.length})` : '' }}
        </option>
      </select>

      <div v-if="current?.branch" class="flex items-center gap-1.5 font-mono text-slate-400 min-w-0">
        <GitBranch class="w-3.5 h-3.5 text-teal-400 flex-shrink-0" />
        <span class="text-teal-300 truncate">{{ current.branch }}</span>
        <span>→</span>
        <span class="truncate">{{ against === 'base' ? current.base_ref : 'HEAD' }}</span>
        <span v-if="current.compare_ref" class="text-slate-600">({{ current.compare_ref }})</span>
      </div>

      <div class="ml-auto flex items-center gap-3">
        <span v-if="current?.files" class="font-mono">
          <span class="text-slate-300">{{ current.files.length }} files</span>
          <span class="text-emerald-400 ml-2">+{{ current.additions || 0 }}</span>
          <span class="text-rose-400 ml-1">−{{ current.deletions || 0 }}</span>
        </span>
        <button type="button" @click="wrap = !wrap" :title="wrap ? 'Scroll long lines' : 'Wrap long lines'"
          class="p-1.5 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800">
          <AlignJustify v-if="!wrap" class="w-3.5 h-3.5" />
          <Columns2 v-else class="w-3.5 h-3.5" />
        </button>
        <button type="button" @click="load" :disabled="isLoading" aria-label="Refresh changes"
          class="p-1.5 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800">
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isLoading }" />
        </button>
      </div>
    </div>

    <!-- States -->
    <div v-if="loadError" class="m-4 p-3 rounded-lg border border-rose-900/60 bg-rose-950/30 text-xs text-rose-200 flex gap-2">
      <AlertCircle class="w-4 h-4 flex-shrink-0" /> {{ loadError }}
    </div>
    <div v-else-if="isLoading && repos.length === 0" class="p-10 text-center text-xs text-slate-500 animate-pulse">Loading changes…</div>
    <div v-else-if="repos.length === 0" class="p-10 text-center text-xs text-slate-500">
      No repositories are assigned to this task, so there is nothing to diff.
    </div>
    <div v-else-if="current?.error" class="m-4 p-4 rounded-lg border border-slate-800 bg-slate-900/60 text-xs text-slate-300">
      <div class="font-semibold text-slate-200 mb-1">{{ current.repo }}</div>
      {{ current.error }}
    </div>
    <div v-else-if="current && (current.files?.length || 0) === 0" class="p-10 text-center text-xs text-slate-500">
      {{ against === 'base' ? `No changes on ${current.branch} since it branched from ${current.base_ref}.` : 'No uncommitted changes.' }}
    </div>

    <!-- Diff -->
    <div v-else-if="current" class="flex-1 min-h-0 flex">
      <!-- File list -->
      <nav class="w-64 flex-shrink-0 border-r border-slate-800 overflow-y-auto py-2 hidden md:block" aria-label="Changed files">
        <div v-if="against === 'base' && current.commits?.length" class="px-3 pb-2 mb-2 border-b border-slate-800">
          <div class="text-[10px] uppercase tracking-wider text-slate-500 mb-1">{{ current.commits.length }} commits</div>
          <div v-for="c in current.commits" :key="c.sha" class="flex items-start gap-1.5 text-[11px] py-0.5" :title="c.subject">
            <GitCommit class="w-3 h-3 text-slate-500 flex-shrink-0 mt-0.5" />
            <span class="font-mono text-slate-500">{{ c.sha }}</span>
            <span class="text-slate-300 truncate">{{ c.subject }}</span>
          </div>
        </div>
        <button v-for="f in current.files" :key="f.path" type="button" @click="scrollToFile(f.path)"
          class="w-full text-left px-3 py-1 flex items-center gap-2 text-[11px] hover:bg-slate-900" :title="f.path">
          <span class="w-4 text-center font-mono font-bold rounded border text-[9px]" :class="STATUS_STYLE[f.status]">{{ STATUS_LETTER[f.status] }}</span>
          <span class="truncate text-slate-300 flex-1" dir="rtl">{{ f.path }}</span>
          <span class="font-mono text-emerald-400">{{ f.additions || '' }}</span>
          <span class="font-mono text-rose-400">{{ f.deletions || '' }}</span>
        </button>
      </nav>

      <!-- Patches -->
      <div class="flex-1 min-w-0 overflow-auto p-4 space-y-3">
        <div v-if="current.truncated" class="p-2.5 rounded border border-amber-800/60 bg-amber-950/30 text-[11px] text-amber-200">
          This diff is too large to show in full; some files are omitted. Open the worktree to review everything.
        </div>
        <section v-for="f in current.files" :key="f.path" :id="`diff-${f.path}`" class="rounded-lg border border-slate-800 overflow-hidden">
          <button type="button" @click="collapsed[f.path] = !collapsed[f.path]" :aria-expanded="!collapsed[f.path]"
            class="w-full px-3 py-2 bg-slate-900 flex items-center gap-2 text-xs sticky top-0 z-10 hover:bg-slate-800/80">
            <ChevronRight v-if="collapsed[f.path]" class="w-3.5 h-3.5 text-slate-500" />
            <ChevronDown v-else class="w-3.5 h-3.5 text-slate-500" />
            <FileDiff class="w-3.5 h-3.5 text-slate-400" />
            <span class="font-mono text-slate-200 truncate">
              <template v-if="f.old_path">{{ f.old_path }} → </template>{{ f.path }}
            </span>
            <span class="px-1.5 rounded border text-[10px]" :class="STATUS_STYLE[f.status]">{{ f.status }}</span>
            <span class="ml-auto font-mono text-emerald-400">+{{ f.additions }}</span>
            <span class="font-mono text-rose-400">−{{ f.deletions }}</span>
          </button>
          <template v-if="!collapsed[f.path]">
            <div v-if="f.binary" class="px-4 py-3 text-[11px] text-slate-500 bg-slate-950">Binary file — not shown.</div>
            <div v-else-if="!patches[f.path]" class="px-4 py-3 text-[11px] text-slate-500 bg-slate-950">Diff not included (size limit).</div>
            <div v-else-if="patches[f.path].lines.length > MAX_RENDERED_LINES && !expandedLarge[f.path]"
              class="px-4 py-3 text-[11px] text-slate-400 bg-slate-950">
              Large diff ({{ patches[f.path].lines.length.toLocaleString() }} lines).
              <button type="button" class="underline text-blue-400" @click="expandedLarge[f.path] = true">Show anyway</button>
            </div>
            <table v-else class="w-full text-[12px] font-mono leading-5 bg-slate-950 border-collapse">
              <tbody>
                <tr v-for="(l, i) in patches[f.path].lines" :key="i"
                  :class="{
                    'bg-emerald-950/40': l.kind === 'add',
                    'bg-rose-950/40': l.kind === 'del',
                    'bg-sky-950/30 text-sky-300': l.kind === 'hunk',
                    'text-slate-500 italic': l.kind === 'meta',
                  }">
                  <template v-if="l.kind === 'hunk' || l.kind === 'meta'">
                    <td colspan="3" class="px-3 py-0.5 select-none">{{ l.text }}</td>
                  </template>
                  <template v-else>
                    <td class="w-12 px-2 text-right text-slate-600 select-none align-top">{{ l.oldNo ?? '' }}</td>
                    <td class="w-12 px-2 text-right text-slate-600 select-none align-top border-r border-slate-800">{{ l.newNo ?? '' }}</td>
                    <td class="px-3 align-top"
                      :class="[wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre',
                        l.kind === 'add' ? 'text-emerald-200' : l.kind === 'del' ? 'text-rose-200' : 'text-slate-300']">
                      <span class="select-none text-slate-600 mr-1">{{ l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : ' ' }}</span>{{ l.text }}
                    </td>
                  </template>
                </tr>
              </tbody>
            </table>
          </template>
        </section>
      </div>
    </div>
  </div>
</template>
