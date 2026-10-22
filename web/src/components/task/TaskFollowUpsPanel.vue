<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { useTaskStore } from '../../stores/tasks'
import type { Task, FollowUpProposalDTO, TaskFollowUpsDTO } from '../../types'
import { AlertTriangle, ExternalLink, ListPlus, Loader2, RefreshCw, Layers } from 'lucide-vue-next'

const props = defineProps<{ task: Task }>()
const router = useRouter()
const toast = useToastStore()
const taskStore = useTaskStore()

const data = ref<TaskFollowUpsDTO | null>(null)
const loading = ref(false)
const error = ref('')
/** proposal id → item ids the operator unticked (every item is included by default) */
const excluded = reactive<Record<string, string[]>>({})
/** proposals ticked to be combined into one task */
const combine = ref<string[]>([])
const combinedTitle = ref('')
/** proposal id, or 'combine', while its task is being created */
const busy = ref('')

const PRIORITY: Record<string, string> = {
  high: 'bg-rose-950/70 border-rose-800/70 text-rose-300',
  medium: 'bg-amber-950/60 border-amber-800/60 text-amber-300',
  low: 'bg-slate-800 border-slate-700 text-slate-400',
}

const proposals = computed(() => data.value?.proposals ?? [])
const open = computed(() => proposals.value.filter((p) => !p.created_task_id))
const groups = computed(() => (data.value?.stages ?? []).map((s) => ({ ...s, proposals: proposals.value.filter((p) => p.stage === s.id) })))
const combining = computed(() => combine.value.filter((id) => open.value.some((p) => p.id === id)))

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.getTaskFollowUps(props.task.id)
    // Forget choices for proposals that left the reports (stage re-run) or were added meanwhile.
    const ids = new Set(open.value.map((p) => p.id))
    combine.value = combine.value.filter((id) => ids.has(id))
    for (const id of Object.keys(excluded)) if (!ids.has(id)) delete excluded[id]
  } catch (e: any) {
    error.value = e?.message || 'Failed to load follow-ups'
  } finally {
    loading.value = false
  }
}

function included(p: FollowUpProposalDTO, itemId: string) {
  return !(excluded[p.id] || []).includes(itemId)
}
function toggleItem(p: FollowUpProposalDTO, itemId: string) {
  const cur = excluded[p.id] || []
  excluded[p.id] = cur.includes(itemId) ? cur.filter((i) => i !== itemId) : [...cur, itemId]
}
function chosenItems(p: FollowUpProposalDTO) {
  return p.items.filter((it) => included(p, it.id)).map((it) => it.id)
}
/** A proposal with items needs at least one ticked; one without items is added as a whole. */
function canAdd(p: FollowUpProposalDTO) {
  return !p.created_task_id && (p.items.length === 0 || chosenItems(p).length > 0)
}
function selection(p: FollowUpProposalDTO) {
  // Send item ids only when some were unticked, so a proposal with every item reads as "all".
  const items = chosenItems(p)
  return { proposal_id: p.id, item_ids: items.length < p.items.length ? items : undefined }
}

async function create(ps: FollowUpProposalDTO[], key: string, title?: string) {
  if (busy.value || !ps.length || !ps.every(canAdd)) return
  busy.value = key
  try {
    const created = await api.createFollowUpTask(props.task.id, { selections: ps.map(selection), title: title?.trim() || undefined })
    taskStore.addTask(created)
    toast.success(`Added ${created.id} to the board`, created.title)
    if (key === 'combine') {
      combine.value = []
      combinedTitle.value = ''
    }
  } catch (e: any) {
    toast.error('Could not add the follow-up task', e?.message)
  } finally {
    busy.value = ''
    await load()
  }
}

function addOne(p: FollowUpProposalDTO) {
  return create([p], p.id)
}
function addCombined() {
  const ps = open.value.filter((p) => combining.value.includes(p.id))
  return create(ps, 'combine', combinedTitle.value)
}

function stageNote(s: { has_report: boolean; listed: boolean }, count: number) {
  if (!s.has_report) return 'Not run yet'
  if (!s.listed) return 'Its report has no follow-up list — re-run the stage to get proposals'
  if (!count) return 'Nothing left to follow up'
  return `${count} proposed task${count === 1 ? '' : 's'}`
}

// A stage's report is rewritten when it finishes: reload when the task moves on or settles.
watch(() => [props.task.id, props.task.state, props.task.current_stage_id], load)
onMounted(load)
</script>

<template>
  <div class="flex-1 min-h-0 overflow-y-auto">
    <div class="max-w-5xl mx-auto p-4 space-y-4 pb-28">
      <div class="flex items-start gap-3">
        <div class="flex-1 min-w-0">
          <h2 class="text-sm font-semibold text-slate-100">Follow-up tasks</h2>
          <p class="text-xs text-slate-400 mt-0.5">
            What each launch-readiness stage found still missing — gaps it did not close, or work the product code needs.
            Nothing is added automatically: add a proposal as a General AI SDLC task, untick items you do not want, or tick
            several related proposals to combine them into one task.
          </p>
        </div>
        <button type="button" @click="load" :disabled="loading"
          class="h-8 px-3 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-200 flex items-center gap-1.5 disabled:opacity-60">
          <Loader2 v-if="loading" class="w-3.5 h-3.5 animate-spin" /><RefreshCw v-else class="w-3.5 h-3.5" /> Refresh
        </button>
      </div>

      <div v-if="error" class="flex items-center gap-2 text-xs text-rose-300 bg-rose-950/40 border border-rose-900 rounded-lg px-3 py-2">
        <AlertTriangle class="w-4 h-4 flex-shrink-0" /> {{ error }}
      </div>

      <div v-if="data && !groups.length" class="text-xs text-slate-500 border border-dashed border-slate-800 rounded-lg p-6 text-center">
        This task's workflow has no launch-readiness stages, so there is nothing to follow up.
      </div>

      <section v-for="g in groups" :key="g.id" class="space-y-2">
        <div class="flex items-baseline gap-2">
          <h3 class="text-xs font-semibold text-slate-200">{{ g.name }}</h3>
          <span class="text-[11px] text-slate-500">{{ stageNote(g, g.proposals.length) }}</span>
        </div>

        <article v-for="p in g.proposals" :key="p.id" class="rounded-lg border bg-slate-900/60 p-3 space-y-2"
          :class="p.created_task_id ? 'border-emerald-900/60 opacity-80' : combining.includes(p.id) ? 'border-sky-700' : 'border-slate-800'">
          <header class="flex items-start gap-2">
            <input v-if="!p.created_task_id" type="checkbox" :value="p.id" v-model="combine" :aria-label="`Combine ${p.title}`"
              title="Tick related proposals to combine them into one task" class="mt-0.5 accent-sky-500" />
            <div class="flex-1 min-w-0">
              <div class="text-sm text-slate-100 font-medium">{{ p.title }}</div>
              <p v-if="p.summary" class="text-xs text-slate-400 mt-0.5 whitespace-pre-line">{{ p.summary }}</p>
              <div v-if="p.repos?.length" class="mt-1 flex flex-wrap gap-1">
                <span v-for="r in p.repos" :key="r" class="font-mono text-[10px] px-1.5 py-0.5 rounded bg-slate-800 border border-slate-700 text-teal-300">{{ r }}</span>
              </div>
            </div>
            <button v-if="p.created_task_id" type="button" @click="router.push(`/tasks/${p.created_task_id}`)"
              class="h-7 px-2.5 rounded-lg border border-emerald-800 bg-emerald-950/60 text-emerald-300 hover:text-emerald-100 text-[11px] font-semibold flex items-center gap-1 flex-shrink-0">
              Added as {{ p.created_task_id }} <ExternalLink class="w-3 h-3" />
            </button>
            <button v-else type="button" @click="addOne(p)" :disabled="!!busy || !canAdd(p)"
              :title="canAdd(p) ? 'Add this proposal to the board as a General AI SDLC task' : 'Tick at least one item'"
              class="h-7 px-2.5 rounded-lg border border-sky-700 bg-sky-900/60 hover:bg-sky-800 text-sky-100 text-[11px] font-semibold flex items-center gap-1 flex-shrink-0 disabled:opacity-50">
              <Loader2 v-if="busy === p.id" class="w-3 h-3 animate-spin" /><ListPlus v-else class="w-3 h-3" /> Add as task
            </button>
          </header>

          <ul v-if="p.items.length" class="space-y-1 pl-6">
            <li v-for="it in p.items" :key="it.id" class="flex items-start gap-2 text-xs">
              <input type="checkbox" :checked="included(p, it.id)" :disabled="!!p.created_task_id" @change="toggleItem(p, it.id)"
                :aria-label="`Include ${it.title}`" class="mt-0.5 accent-sky-500" />
              <span class="px-1.5 py-px rounded border text-[10px] uppercase font-semibold flex-shrink-0" :class="PRIORITY[it.priority]">{{ it.priority }}</span>
              <div class="min-w-0" :class="included(p, it.id) ? '' : 'opacity-50 line-through'">
                <span class="text-slate-200">{{ it.title }}</span>
                <span v-if="it.detail" class="text-slate-400"> — {{ it.detail }}</span>
                <span v-if="it.evidence" class="ml-1 font-mono text-[10px] text-slate-500">{{ it.evidence }}</span>
              </div>
            </li>
          </ul>
        </article>
      </section>
    </div>

    <div v-if="combining.length >= 2" class="sticky bottom-0 border-t border-slate-800 bg-slate-900/95 backdrop-blur px-4 py-2.5">
      <div class="max-w-5xl mx-auto flex flex-wrap items-center gap-2">
        <Layers class="w-4 h-4 text-sky-300" />
        <span class="text-xs text-slate-300">{{ combining.length }} proposals into one task</span>
        <input v-model="combinedTitle" type="text" maxlength="200" placeholder="Task title (optional — defaults to the first proposal's)"
          class="flex-1 min-w-48 h-8 px-2.5 rounded-lg bg-slate-950 border border-slate-700 text-xs text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-600" />
        <button type="button" @click="combine = []" class="h-8 px-3 rounded-lg text-xs text-slate-400 hover:text-slate-200">Clear</button>
        <button type="button" @click="addCombined" :disabled="!!busy || !open.filter((p) => combining.includes(p.id)).every(canAdd)"
          class="h-8 px-3 rounded-lg border border-sky-700 bg-sky-800 hover:bg-sky-700 text-xs font-semibold text-white flex items-center gap-1.5 disabled:opacity-50">
          <Loader2 v-if="busy === 'combine'" class="w-3.5 h-3.5 animate-spin" /><ListPlus v-else class="w-3.5 h-3.5" /> Combine into one task
        </button>
      </div>
    </div>
  </div>
</template>
