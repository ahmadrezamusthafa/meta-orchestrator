<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '../../services/api'
import type { DiffAgainst, DiffFileDTO, RepoDiffDTO, TaskWorktreeDTO } from '../../types'
import { parsePatch, splitRows, unifiedRows, type FilePatch } from './diffModel'
import {
  GitBranch, GitCommit, RefreshCw, ChevronDown, ChevronRight, AlertCircle, Columns2, Rows2, WrapText,
  ChevronsDownUp, ChevronsUpDown, Copy, Check,
} from 'lucide-vue-next'

const props = defineProps<{
  taskId: string
  worktree?: TaskWorktreeDTO | null
}>()

const MAX_RENDERED_LINES = 3000 // per file; larger diffs stay collapsed until asked for
const VIEW_KEY = 'mo.diff.view'
const WRAP_KEY = 'mo.diff.wrap'

function readPref(key: string, fallback: string) {
  try {
    return localStorage.getItem(key) || fallback
  } catch {
    return fallback
  }
}
function writePref(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* storage unavailable */
  }
}

const against = ref<DiffAgainst>('base')
const view = ref<'unified' | 'split'>(readPref(VIEW_KEY, 'unified') === 'split' ? 'split' : 'unified')
const wrap = ref(readPref(WRAP_KEY, 'false') === 'true')
const selectedRepo = ref('')
const repos = ref<RepoDiffDTO[]>([])
const isLoading = ref(false)
const loadError = ref('')
const collapsed = ref<Record<string, boolean>>({})
const viewed = ref<Record<string, boolean>>({})
const showLarge = ref<Record<string, boolean>>({})
const copiedPath = ref('')

watch(view, (v) => writePref(VIEW_KEY, v))
watch(wrap, (v) => writePref(WRAP_KEY, String(v)))

const current = computed(() => repos.value.find((r) => r.repo === selectedRepo.value) || repos.value[0] || null)
const patches = computed<Record<string, FilePatch>>(() => parsePatch(current.value?.patch || ''))
const files = computed<DiffFileDTO[]>(() => current.value?.files || [])
const viewedCount = computed(() => files.value.filter((f) => viewed.value[f.path]).length)

const STATUS_STYLE: Record<string, string> = {
  added: 'text-[#3fb950] border-[#238636]/60',
  untracked: 'text-[#3fb950] border-[#238636]/60',
  modified: 'text-[#d29922] border-[#9e6a03]/60',
  deleted: 'text-[#f85149] border-[#da3633]/60',
  renamed: 'text-[#58a6ff] border-[#1f6feb]/60',
}
const STATUS_LETTER: Record<string, string> = { added: 'A', untracked: 'U', modified: 'M', deleted: 'D', renamed: 'R' }

/** GitHub-style bar of five blocks showing the add/delete ratio. */
function diffstat(f: DiffFileDTO) {
  const total = f.additions + f.deletions
  if (!total) return Array(5).fill('n')
  const adds = Math.round((f.additions / total) * 5)
  return Array.from({ length: 5 }, (_, i) => (i < adds ? 'a' : 'd'))
}

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

function toggleViewed(path: string) {
  viewed.value[path] = !viewed.value[path]
  collapsed.value[path] = viewed.value[path] // GitHub collapses a file once you mark it viewed
}

function setAll(isCollapsed: boolean) {
  const next: Record<string, boolean> = {}
  for (const f of files.value) next[f.path] = isCollapsed
  collapsed.value = next
}

async function copyPath(path: string) {
  try {
    await navigator.clipboard.writeText(path)
    copiedPath.value = path
    setTimeout(() => (copiedPath.value = ''), 1200)
  } catch {
    /* clipboard unavailable */
  }
}

const SIGN: Record<string, string> = { add: '+', del: '-', ctx: ' ' }
const copiedPatch = ref('')

// Split view: a selection stays on the side it started on, like GitHub.
function pickSide(e: MouseEvent) {
  const cell = (e.target as HTMLElement).closest('td[data-side]') as HTMLElement | null
  const table = e.currentTarget as HTMLElement
  if (cell) table.dataset.selectSide = cell.dataset.side
}

// Copy exactly the selected code: no line numbers, +/- signs or cell separators, and in split
// view only the side being selected. Indentation is preserved.
function onCopy(e: ClipboardEvent) {
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0 || sel.isCollapsed) return
  const range = sel.getRangeAt(0)
  const root = e.currentTarget as HTMLElement
  const tables = Array.from(root.querySelectorAll('table.gh-diff')).filter((t) => range.intersectsNode(t))
  if (!tables.length) return // selection is outside the diff tables; let the browser copy it
  const lines: string[] = []
  for (const table of tables) {
    const side = (table as HTMLElement).dataset.selectSide
    const selector = table.classList.contains('gh-split') ? `td.gh-code[data-side="${side || 'right'}"]` : 'td.gh-code'
    for (const cell of Array.from(table.querySelectorAll(selector))) {
      if (!range.intersectsNode(cell) || cell.classList.contains('gh-empty')) continue
      const part = document.createRange()
      part.selectNodeContents(cell)
      if (cell.contains(range.startContainer)) part.setStart(range.startContainer, range.startOffset)
      if (cell.contains(range.endContainer)) part.setEnd(range.endContainer, range.endOffset)
      lines.push(part.toString())
    }
  }
  if (!lines.length) return
  e.preventDefault()
  e.clipboardData?.setData('text/plain', lines.join('\n'))
}

// The unified patch of one file, as git produced it.
function patchText(path: string): string {
  const patch = current.value?.patch || ''
  const start = patch.indexOf(`diff --git a/`)
  if (start < 0) return ''
  const chunks = patch.slice(start).split(/\n(?=diff --git )/)
  return chunks.find((c) => c.split('\n')[0].endsWith(` b/${path}`)) || ''
}

async function copyPatch(path: string) {
  try {
    await navigator.clipboard.writeText(patchText(path).trimEnd() + '\n')
    copiedPatch.value = path
    setTimeout(() => (copiedPatch.value = ''), 1400)
  } catch {
    /* clipboard unavailable */
  }
}

const tooLarge = (path: string) => (patches.value[path]?.lineCount || 0) > MAX_RENDERED_LINES && !showLarge.value[path]
const hunkText = (header: string) => header.replace(/^(@@[^@]*@@)(.*)$/, '$1  $2').trim()

watch(against, load)
watch(() => props.taskId, load)
watch(selectedRepo, () => {
  collapsed.value = {}
  viewed.value = {}
})
onMounted(load)
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[#0d1117]">
    <!-- Toolbar -->
    <div class="px-4 py-2 border-b border-[#30363d] flex flex-wrap items-center gap-3 text-xs bg-[#010409]">
      <div class="flex items-center p-0.5 rounded-md bg-[#0d1117] border border-[#30363d]" role="group" aria-label="Compare against">
        <button type="button" @click="against = 'base'" :aria-pressed="against === 'base'" class="px-2.5 py-1 rounded"
          :class="against === 'base' ? 'bg-[#21262d] text-[#e6edf3] font-semibold' : 'text-[#9198a1] hover:text-[#e6edf3]'">
          vs {{ current?.base_ref || worktree?.base_ref || 'base branch' }}
        </button>
        <button type="button" @click="against = 'head'" :aria-pressed="against === 'head'" class="px-2.5 py-1 rounded"
          :class="against === 'head' ? 'bg-[#21262d] text-[#e6edf3] font-semibold' : 'text-[#9198a1] hover:text-[#e6edf3]'">
          Uncommitted
        </button>
      </div>

      <select v-if="repos.length > 1" v-model="selectedRepo" aria-label="Repository"
        class="h-7 px-2 bg-[#0d1117] border border-[#30363d] rounded-md text-[#e6edf3] focus:outline-none focus:border-[#1f6feb]">
        <option v-for="r in repos" :key="r.repo" :value="r.repo">{{ r.repo }}{{ r.files?.length ? ` (${r.files.length})` : '' }}</option>
      </select>

      <div v-if="current?.branch" class="flex items-center gap-1.5 font-mono text-[#9198a1] min-w-0">
        <GitBranch class="w-3.5 h-3.5 flex-shrink-0" />
        <span class="px-1.5 py-0.5 rounded-md bg-[#388bfd26] text-[#58a6ff] truncate">{{ current.branch }}</span>
        <span>←</span>
        <span class="px-1.5 py-0.5 rounded-md bg-[#21262d] text-[#e6edf3] truncate">{{ against === 'base' ? current.base_ref : 'HEAD' }}</span>
      </div>

      <div class="ml-auto flex items-center gap-2">
        <span v-if="files.length" class="font-mono text-[#9198a1]">
          <span class="text-[#e6edf3]">{{ files.length }}</span> files
          <span class="text-[#3fb950] ml-1.5">+{{ current?.additions || 0 }}</span>
          <span class="text-[#f85149] ml-1">−{{ current?.deletions || 0 }}</span>
        </span>
        <span v-if="files.length" class="text-[#9198a1]">{{ viewedCount }}/{{ files.length }} viewed</span>

        <div class="flex items-center p-0.5 rounded-md bg-[#0d1117] border border-[#30363d]" role="group" aria-label="Diff layout">
          <button type="button" @click="view = 'unified'" :aria-pressed="view === 'unified'" title="Unified"
            class="px-2 py-1 rounded flex items-center gap-1"
            :class="view === 'unified' ? 'bg-[#21262d] text-[#e6edf3]' : 'text-[#9198a1] hover:text-[#e6edf3]'">
            <Rows2 class="w-3.5 h-3.5" /> Unified
          </button>
          <button type="button" @click="view = 'split'" :aria-pressed="view === 'split'" title="Split"
            class="px-2 py-1 rounded flex items-center gap-1"
            :class="view === 'split' ? 'bg-[#21262d] text-[#e6edf3]' : 'text-[#9198a1] hover:text-[#e6edf3]'">
            <Columns2 class="w-3.5 h-3.5" /> Split
          </button>
        </div>
        <button v-if="view === 'unified'" type="button" @click="wrap = !wrap" :aria-pressed="wrap" :title="wrap ? 'Stop wrapping lines' : 'Wrap long lines'"
          class="p-1.5 rounded-md border border-transparent" :class="wrap ? 'bg-[#21262d] border-[#30363d] text-[#e6edf3]' : 'text-[#9198a1] hover:text-[#e6edf3]'">
          <WrapText class="w-3.5 h-3.5" />
        </button>
        <button type="button" @click="setAll(true)" title="Collapse all" aria-label="Collapse all files" class="p-1.5 rounded-md text-[#9198a1] hover:text-[#e6edf3]">
          <ChevronsDownUp class="w-3.5 h-3.5" />
        </button>
        <button type="button" @click="setAll(false)" title="Expand all" aria-label="Expand all files" class="p-1.5 rounded-md text-[#9198a1] hover:text-[#e6edf3]">
          <ChevronsUpDown class="w-3.5 h-3.5" />
        </button>
        <button type="button" @click="load" :disabled="isLoading" aria-label="Refresh changes" class="p-1.5 rounded-md text-[#9198a1] hover:text-[#e6edf3]">
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isLoading }" />
        </button>
      </div>
    </div>

    <!-- States -->
    <div v-if="loadError" class="m-4 p-3 rounded-md border border-[#f8514966] bg-[#f851491a] text-xs text-[#ffa198] flex gap-2">
      <AlertCircle class="w-4 h-4 flex-shrink-0" /> {{ loadError }}
    </div>
    <div v-else-if="isLoading && repos.length === 0" class="p-10 text-center text-xs text-[#9198a1] animate-pulse">Loading changes…</div>
    <div v-else-if="repos.length === 0" class="p-10 text-center text-xs text-[#9198a1]">No repositories are assigned to this task, so there is nothing to diff.</div>
    <div v-else-if="current?.error" class="m-4 p-4 rounded-md border border-[#30363d] bg-[#151b23] text-xs text-[#e6edf3]">
      <div class="font-semibold mb-1">{{ current.repo }}</div>{{ current.error }}
    </div>
    <div v-else-if="current && files.length === 0" class="p-10 text-center text-xs text-[#9198a1]">
      {{ against === 'base' ? `No changes on ${current.branch} since it branched from ${current.base_ref}.` : 'No uncommitted changes.' }}
    </div>

    <div v-else-if="current" class="flex-1 min-h-0 flex">
      <!-- File tree -->
      <nav class="w-72 flex-shrink-0 border-r border-[#30363d] overflow-y-auto py-2 hidden lg:block" aria-label="Changed files">
        <div v-if="against === 'base' && current.commits?.length" class="px-3 pb-2 mb-2 border-b border-[#30363d]">
          <div class="text-[10px] uppercase tracking-wider text-[#9198a1] mb-1">{{ current.commits.length }} commits</div>
          <div v-for="c in current.commits" :key="c.sha" class="flex items-start gap-1.5 text-[11px] py-0.5" :title="c.subject">
            <GitCommit class="w-3 h-3 text-[#9198a1] flex-shrink-0 mt-0.5" />
            <span class="font-mono text-[#58a6ff]">{{ c.sha }}</span>
            <span class="text-[#e6edf3] truncate">{{ c.subject }}</span>
          </div>
        </div>
        <button v-for="f in files" :key="f.path" type="button" @click="scrollToFile(f.path)" :title="f.path"
          class="w-full text-left px-3 py-1 flex items-center gap-2 text-[12px] hover:bg-[#151b23]"
          :class="viewed[f.path] ? 'opacity-50' : ''">
          <span class="w-4 text-center font-mono font-bold rounded-sm border text-[9px] leading-4 flex-shrink-0" :class="STATUS_STYLE[f.status]">{{ STATUS_LETTER[f.status] }}</span>
          <span class="truncate text-[#e6edf3] flex-1" dir="rtl">{{ f.path }}</span>
          <span class="font-mono text-[11px] text-[#3fb950]">{{ f.additions ? `+${f.additions}` : '' }}</span>
          <span class="font-mono text-[11px] text-[#f85149]">{{ f.deletions ? `−${f.deletions}` : '' }}</span>
        </button>
      </nav>

      <!-- Files -->
      <div class="flex-1 min-w-0 overflow-auto p-4 space-y-4" @copy="onCopy">
        <div v-if="current.truncated" class="p-2.5 rounded-md border border-[#bb800966] bg-[#bb80091a] text-[11px] text-[#e3b341]">
          This diff is too large to show in full; some files are omitted. Open the worktree to review everything.
        </div>

        <section v-for="f in files" :key="f.path" :id="`diff-${f.path}`" class="rounded-md border border-[#30363d] overflow-hidden">
          <!-- File header -->
          <header class="sticky top-0 z-10 flex items-center gap-2 px-2 py-1.5 bg-[#151b23] border-b border-[#30363d] text-xs">
            <button type="button" @click="collapsed[f.path] = !collapsed[f.path]" :aria-expanded="!collapsed[f.path]" :aria-label="`Toggle ${f.path}`"
              class="p-1 rounded text-[#9198a1] hover:text-[#e6edf3] hover:bg-[#21262d]">
              <ChevronRight v-if="collapsed[f.path]" class="w-4 h-4" /><ChevronDown v-else class="w-4 h-4" />
            </button>
            <span class="font-mono text-[#e6edf3] font-semibold">+{{ f.additions }}</span>
            <span class="font-mono text-[#e6edf3] font-semibold -ml-1">−{{ f.deletions }}</span>
            <span class="flex gap-px" aria-hidden="true">
              <span v-for="(b, i) in diffstat(f)" :key="i" class="w-2 h-2 rounded-[1px]"
                :class="b === 'a' ? 'bg-[#3fb950]' : b === 'd' ? 'bg-[#f85149]' : 'bg-[#30363d]'"></span>
            </span>
            <span class="font-mono text-[#e6edf3] truncate min-w-0">
              <template v-if="f.old_path"><span class="text-[#9198a1]">{{ f.old_path }}</span> → </template>{{ f.path }}
            </span>
            <button type="button" @click="copyPath(f.path)" :aria-label="`Copy path ${f.path}`" class="p-1 rounded text-[#9198a1] hover:text-[#e6edf3]">
              <Check v-if="copiedPath === f.path" class="w-3.5 h-3.5 text-[#3fb950]" /><Copy v-else class="w-3.5 h-3.5" />
            </button>
            <button v-if="patchText(f.path)" type="button" @click="copyPatch(f.path)" :title="`Copy the diff of ${f.path}`"
              class="px-1.5 py-0.5 rounded text-[11px] text-[#9198a1] hover:text-[#e6edf3] hover:bg-[#21262d]">
              {{ copiedPatch === f.path ? 'Copied' : 'Copy diff' }}
            </button>
            <span v-if="f.status === 'untracked' || f.status === 'added'" class="px-1.5 rounded-full border border-[#238636]/60 text-[#3fb950] text-[10px]">new</span>
            <span v-else-if="f.status === 'deleted'" class="px-1.5 rounded-full border border-[#da3633]/60 text-[#f85149] text-[10px]">deleted</span>
            <label class="ml-auto flex items-center gap-1.5 px-2 py-0.5 rounded-md border border-[#30363d] text-[#9198a1] cursor-pointer hover:text-[#e6edf3]"
              :class="viewed[f.path] ? 'bg-[#1f6feb33] border-[#1f6feb99] text-[#e6edf3]' : ''">
              <input type="checkbox" :checked="!!viewed[f.path]" @change="toggleViewed(f.path)" class="accent-[#1f6feb]" /> Viewed
            </label>
          </header>

          <template v-if="!collapsed[f.path]">
            <div v-if="f.binary || patches[f.path]?.binary" class="px-4 py-3 text-[12px] text-[#9198a1] bg-[#0d1117]">Binary file not shown.</div>
            <div v-else-if="!patches[f.path]" class="px-4 py-3 text-[12px] text-[#9198a1] bg-[#0d1117]">Diff not included (size limit).</div>
            <div v-else-if="tooLarge(f.path)" class="px-4 py-3 text-[12px] text-[#9198a1] bg-[#0d1117]">
              Large diff ({{ patches[f.path].lineCount.toLocaleString() }} lines) is hidden.
              <button type="button" class="text-[#58a6ff] hover:underline" @click="showLarge[f.path] = true">Load diff</button>
            </div>

            <!-- Unified -->
            <table v-else-if="view === 'unified'" class="gh-diff w-full" :class="{ 'gh-wrap': wrap }">
              <colgroup><col class="gh-numcol" /><col class="gh-numcol" /><col /></colgroup>
              <tbody>
                <template v-for="(row, i) in unifiedRows(patches[f.path])" :key="i">
                  <tr v-if="row.type === 'hunk'" class="gh-hunk"><td colspan="3">{{ hunkText(row.header) }}</td></tr>
                  <tr v-else-if="row.type === 'line'" :class="`gh-${row.line.kind}`">
                    <td class="gh-num" :data-n="row.line.oldNo ?? ''"></td>
                    <td class="gh-num" :data-n="row.line.newNo ?? ''"></td>
                    <td class="gh-code" :data-sign="SIGN[row.line.kind]"><template
                      v-for="(s, j) in row.line.segments" :key="j"><span v-if="s.hl" class="gh-word">{{ s.text }}</span><template v-else>{{ s.text }}</template></template></td>
                  </tr>
                </template>
              </tbody>
            </table>

            <!-- Split -->
            <table v-else class="gh-diff gh-split gh-wrap w-full" @mousedown="pickSide">
              <colgroup><col class="gh-numcol" /><col /><col class="gh-numcol" /><col /></colgroup>
              <tbody>
                <template v-for="(row, i) in splitRows(patches[f.path])" :key="i">
                  <tr v-if="row.type === 'hunk'" class="gh-hunk"><td colspan="4">{{ hunkText(row.header) }}</td></tr>
                  <tr v-else-if="row.type === 'pair'">
                    <template v-for="(side, si) in [row.left, row.right]" :key="si">
                      <td class="gh-num" :data-side="si === 0 ? 'left' : 'right'" :class="side ? (side.kind === 'ctx' ? '' : `gh-${side.kind}`) : 'gh-empty'"
                        :data-n="side ? ((si === 0 ? side.oldNo : side.newNo) ?? '') : ''"></td>
                      <td class="gh-code" :data-side="si === 0 ? 'left' : 'right'" :data-sign="side ? SIGN[side.kind] : ''"
                        :class="[side ? (side.kind === 'ctx' ? '' : `gh-${side.kind}`) : 'gh-empty', si === 0 ? 'gh-split-left' : '']"><template
                        v-if="side"><template v-for="(s, j) in side.segments" :key="j"><span
                        v-if="s.hl" class="gh-word">{{ s.text }}</span><template v-else>{{ s.text }}</template></template></template></td>
                    </template>
                  </tr>
                </template>
              </tbody>
            </table>
          </template>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* GitHub dark diff palette */
.gh-diff { table-layout: fixed; border-collapse: collapse; font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; line-height: 20px; color: #e6edf3; background: #0d1117; }
.gh-diff td { padding: 0; vertical-align: top; }
.gh-numcol { width: 56px; }
/* Line numbers and +/- signs are drawn by CSS so they are never part of a text selection. */
.gh-num::before { content: attr(data-n); }
.gh-code::before { content: attr(data-sign); position: absolute; left: 8px; color: #9198a1; user-select: none; }
tr.gh-add .gh-code::before, td.gh-add.gh-code::before { color: #3fb950; }
tr.gh-del .gh-code::before, td.gh-del.gh-code::before { color: #f85149; }
.gh-split[data-select-side='left'] td[data-side='right'],
.gh-split[data-select-side='right'] td[data-side='left'] { user-select: none; }
.gh-code ::selection, .gh-code::selection { background: rgba(56, 139, 253, 0.4); }
.gh-num {
  padding: 0 10px !important; text-align: right; color: #6e7681; user-select: none; white-space: nowrap;
  font-size: 12px;
}
.gh-code { padding: 0 10px 0 22px !important; white-space: pre; position: relative; }
.gh-wrap .gh-code { white-space: pre-wrap; overflow-wrap: anywhere; }
.gh-hunk td { background: rgba(56, 139, 253, 0.1); color: #9198a1; padding: 4px 10px !important; white-space: pre-wrap; }

tr.gh-add .gh-code, td.gh-code.gh-add { background: rgba(46, 160, 67, 0.15); }
tr.gh-add .gh-num, td.gh-num.gh-add { background: rgba(63, 185, 80, 0.3); color: #e6edf3; }
tr.gh-add .gh-word, td.gh-add .gh-word { background: rgba(46, 160, 67, 0.4); border-radius: 2px; }

tr.gh-del .gh-code, td.gh-code.gh-del { background: rgba(248, 81, 73, 0.1); }
tr.gh-del .gh-num, td.gh-num.gh-del { background: rgba(248, 81, 73, 0.3); color: #e6edf3; }
tr.gh-del .gh-word, td.gh-del .gh-word { background: rgba(248, 81, 73, 0.4); border-radius: 2px; }

td.gh-empty { background: rgba(110, 118, 129, 0.1); }
.gh-split td.gh-split-left { border-right: 1px solid #30363d; }
.gh-diff tbody tr:hover .gh-num:not(.gh-empty) { color: #e6edf3; }
</style>
