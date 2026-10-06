<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import MarkdownView from '../common/MarkdownView.vue'
import type { Task, PullRequestDraftDTO } from '../../types'
import { GitPullRequest, GitBranch, ExternalLink, Loader2, RefreshCw, AlertCircle, Copy, Check, Eye, Pencil, UploadCloud, CheckCircle2 } from 'lucide-vue-next'

const props = defineProps<{ task: Task }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()
const toast = useToastStore()

const drafts = ref<PullRequestDraftDTO[]>([])
const loading = ref(false)
const error = ref('')
const selected = ref('')
const title = ref('')
const body = ref('')
const mode = ref<'preview' | 'edit'>('preview')
const opening = ref(false)
const copied = ref('')

const draft = computed(() => drafts.value.find((d) => d.repo === selected.value) || drafts.value[0] || null)
const running = computed(() => props.task.state === 'RUNNING')
const outOfSync = computed(() => draft.value?.sync === 'out_of_sync')
const actionLabel = computed(() => {
  if (!draft.value?.existing_url) return 'Commit, push & open pull request'
  return outOfSync.value ? 'Commit, push & update pull request' : 'Update pull request'
})
const edited = computed(() => !!draft.value && (title.value !== draft.value.title || body.value !== draft.value.body))

function adopt(d: PullRequestDraftDTO | null) {
  title.value = d?.title || ''
  body.value = d?.body || ''
}

async function load(keepEdits = false) {
  loading.value = true
  error.value = ''
  try {
    const res = await api.getPullRequestDrafts(props.task.id)
    drafts.value = res.drafts || []
    if (!drafts.value.some((d) => d.repo === selected.value)) selected.value = drafts.value[0]?.repo || ''
    if (!keepEdits || !edited.value) adopt(draft.value)
  } catch (err: any) {
    error.value = err?.message || 'Could not prepare the pull request'
  } finally {
    loading.value = false
  }
}

function pick(repo: string) {
  selected.value = repo
  adopt(draft.value)
}

async function open() {
  const d = draft.value
  if (!d || !d.can_create || opening.value) return
  opening.value = true
  try {
    const res = await api.openPullRequest(props.task.id, { repo: d.repo, title: title.value, body: body.value })
    toast.success(res.updated ? `Pull request #${res.number} updated` : `Pull request #${res.number} opened`,
      res.committed ? 'Pending edits were committed and pushed.' : 'Branch pushed.')
    await load()
    emit('refresh')
  } catch (err: any) {
    toast.error('Could not open the pull request', err?.message || 'Unknown error')
  } finally {
    opening.value = false
  }
}

async function copy(text: string, key: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = key
    setTimeout(() => (copied.value = ''), 1400)
  } catch {
    /* clipboard unavailable */
  }
}

watch(() => [props.task.state, props.task.current_stage_id, props.task.metadata?.pr_out_of_sync], () => load(true))
onMounted(() => load())
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="max-w-5xl mx-auto p-5 space-y-4">
      <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 flex flex-wrap items-start gap-4">
        <div class="flex-1 min-w-[240px] space-y-1">
          <h3 class="text-sm font-semibold text-slate-100 flex items-center gap-2">
            <GitPullRequest class="w-4 h-4 text-violet-400" /> Pull request
          </h3>
          <p class="text-xs text-slate-400 leading-relaxed">
            Opens the task branch as a pull request in the standard format: ticket, summary, type of change, changed files,
            acceptance criteria, how to test, risk &amp; rollback and the review checklist. Pending edits in the worktree are
            committed first. Opening again refreshes the existing pull request.
          </p>
          <p class="text-xs text-slate-400 leading-relaxed">
            An opened pull request is <span class="text-slate-200">not updated automatically</span>: changes the agent or you make
            afterwards stay in the worktree until you choose <span class="text-slate-200">Update pull request</span>, which commits,
            pushes and refreshes the description.
          </p>
        </div>
        <button type="button" @click="load()" :disabled="loading" class="h-8 px-3 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5 disabled:opacity-60">
          <RefreshCw class="w-3.5 h-3.5" :class="loading && 'animate-spin'" /> Refresh
        </button>
      </section>

      <div v-if="error" class="p-3 rounded-lg border border-rose-800/70 bg-rose-950/40 text-xs text-rose-200 flex items-center gap-2">
        <AlertCircle class="w-4 h-4" /> {{ error }}
      </div>
      <div v-else-if="!loading && !drafts.length" class="p-6 rounded-xl border border-dashed border-slate-800 text-center text-xs text-slate-500">
        Assign repositories and run the task — a pull request is prepared from its worktree branch.
      </div>

      <template v-if="draft">
        <div v-if="drafts.length > 1" class="flex flex-wrap gap-1.5" role="tablist" aria-label="Repository">
          <button v-for="d in drafts" :key="d.repo" type="button" role="tab" :aria-selected="d.repo === draft.repo" @click="pick(d.repo)"
            class="h-7 px-2.5 rounded border text-xs font-mono"
            :class="d.repo === draft.repo ? 'bg-slate-800 border-slate-600 text-slate-100' : 'border-slate-800 text-slate-400 hover:text-slate-200'">
            {{ d.repos.join(' + ') }}
          </button>
        </div>

        <section class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-3">
          <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs">
            <span class="flex items-center gap-1.5 font-mono text-teal-300"><GitBranch class="w-3.5 h-3.5" /> {{ draft.source_branch }}</span>
            <span class="text-slate-500">→</span>
            <span class="font-mono text-slate-200">{{ draft.target_branch || '—' }}</span>
            <span class="text-slate-400">{{ draft.files }} file(s) · {{ draft.commits }} commit(s)<template v-if="draft.uncommitted"> · uncommitted edits</template><template v-if="draft.on_remote && draft.unpushed"> · {{ draft.unpushed }} not pushed</template><template v-else-if="!draft.on_remote && draft.commits"> · not pushed yet</template></span>
            <a v-if="draft.repo_url" :href="draft.repo_url" target="_blank" rel="noopener" class="text-slate-400 hover:text-slate-100 flex items-center gap-1 capitalize">
              {{ draft.provider || 'remote' }} <ExternalLink class="w-3 h-3" />
            </a>
            <a v-if="draft.existing_url" :href="draft.existing_url" target="_blank" rel="noopener"
              class="ml-auto inline-flex items-center gap-1 px-2 py-0.5 rounded bg-violet-950/70 border border-violet-800/70 text-violet-200 hover:text-white">
              <GitPullRequest class="w-3 h-3" /> Open pull request <ExternalLink class="w-3 h-3" />
            </a>
          </div>
          <div v-if="outOfSync" role="status"
            class="p-2.5 rounded-lg border border-sky-800/60 bg-sky-950/30 text-xs text-sky-200 flex items-start gap-2">
            <UploadCloud class="w-4 h-4 flex-shrink-0 mt-px" />
            <div class="space-y-0.5">
              <div class="font-semibold">Pull request needs an update</div>
              <div class="text-sky-200/80">{{ draft.sync_note }}</div>
            </div>
          </div>
          <div v-else-if="draft.sync === 'in_sync'" role="status" class="text-[11px] text-emerald-300 flex items-center gap-1.5">
            <CheckCircle2 class="w-3.5 h-3.5" /> {{ draft.sync_note }}
          </div>
          <div v-if="draft.uncommitted" class="text-[11px] text-slate-400">
            Pending edits are committed as <code class="font-mono text-slate-200">{{ draft.commit_message }}</code>
          </div>
          <div v-if="draft.blocker" class="p-2.5 rounded-lg border border-amber-800/60 bg-amber-950/30 text-xs text-amber-200 flex items-start gap-2">
            <AlertCircle class="w-4 h-4 flex-shrink-0 mt-px" /> {{ draft.blocker }}
          </div>

          <label class="block space-y-1">
            <span class="text-[11px] uppercase tracking-wide text-slate-500">Title</span>
            <input v-model="title" type="text" class="w-full h-9 px-3 rounded-lg bg-slate-950 border border-slate-700 text-sm text-slate-100 focus:outline-none focus:border-violet-500" />
          </label>

          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="text-[11px] uppercase tracking-wide text-slate-500">Description</span>
              <div class="ml-auto flex rounded-lg border border-slate-700 overflow-hidden text-[11px]">
                <button type="button" @click="mode = 'preview'" class="px-2 py-1 flex items-center gap-1" :class="mode === 'preview' ? 'bg-slate-800 text-slate-100' : 'text-slate-400'"><Eye class="w-3 h-3" /> Preview</button>
                <button type="button" @click="mode = 'edit'" class="px-2 py-1 flex items-center gap-1" :class="mode === 'edit' ? 'bg-slate-800 text-slate-100' : 'text-slate-400'"><Pencil class="w-3 h-3" /> Edit</button>
              </div>
              <button type="button" @click="copy(body, 'body')" class="text-[11px] text-slate-400 hover:text-slate-100 flex items-center gap-1">
                <Check v-if="copied === 'body'" class="w-3 h-3 text-emerald-400" /><Copy v-else class="w-3 h-3" /> Copy
              </button>
            </div>
            <textarea v-if="mode === 'edit'" v-model="body" rows="22"
              class="w-full p-3 rounded-lg bg-slate-950 border border-slate-700 font-mono text-xs text-slate-200 leading-relaxed focus:outline-none focus:border-violet-500" />
            <div v-else class="p-4 rounded-lg bg-slate-950 border border-slate-800 max-h-[560px] overflow-y-auto">
              <MarkdownView :source="body" />
            </div>
          </div>

          <div class="flex items-center gap-2 pt-1">
            <button v-if="edited" type="button" @click="adopt(draft)" class="text-xs text-slate-400 hover:text-slate-100">Reset to standard format</button>
            <button type="button" @click="open" :disabled="!draft.can_create || opening || running"
              :title="running ? 'Wait for the stage to finish' : draft.blocker || ''"
              class="ml-auto h-9 px-4 rounded-lg bg-violet-600 hover:bg-violet-500 text-white text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed">
              <Loader2 v-if="opening" class="w-3.5 h-3.5 animate-spin" /><GitPullRequest v-else class="w-3.5 h-3.5" />
              {{ actionLabel }}
            </button>
          </div>
        </section>
      </template>
    </div>
  </div>
</template>
