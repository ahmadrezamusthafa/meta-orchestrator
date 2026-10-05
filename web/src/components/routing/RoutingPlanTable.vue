<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import type { StageRoute, MethodOptionDTO, ModelOptionDTO, SkillOptionDTO } from '../../types'
import { stageName } from '../../composables/taskLifecycle'
import { Plus, RotateCcw, Sparkles, X } from 'lucide-vue-next'

// Editable routing plan: one row per stage with the proposed method and model, which the
// operator can change. A row differing from its proposal is marked overridden.
const props = defineProps<{
  rows: StageRoute[]
  proposed: Record<string, { method: string; model: string }>
  // What the router recommends right now, when it differs from the saved plan (task page only)
  current?: Record<string, { method: string; model: string }>
  methods: MethodOptionDTO[]
  models: ModelOptionDTO[]
  disabled?: boolean
  currentStage?: string
  // Skills that can be attached to a stage, and the ones suggested per stage
  skills?: SkillOptionDTO[]
  skillSuggestions?: Record<string, string[]>
}>()
const emit = defineEmits<{ (e: 'update:rows', rows: StageRoute[]): void }>()

const methodName = (id: string) => props.methods.find((m) => m.id === id)?.name || id
const modelChoices = computed(() => props.models.map((m) => m.model))
const TIER: Record<string, string> = { tier1: 'Tier 1', tier2: 'Tier 2', tier3: 'Tier 3' }

// "claude/claude-sonnet-5-5" → "claude-sonnet-5-5"; the provider shows in the option title.
function shortModel(m?: string) {
  if (!m) return ''
  const i = m.indexOf('/')
  return i >= 0 ? m.slice(i + 1) : m
}
function modelLabel(m: ModelOptionDTO) {
  const tiers = (m.tiers ?? []).map((t) => TIER[t] || t).join(', ')
  return tiers ? `${shortModel(m.model)} · ${tiers}` : shortModel(m.model)
}
// The policy sentence only; chain fallback details are summarized by the "wanted" note.
function shortReason(r?: string) {
  if (!r) return ''
  return r.split(' | ')[0].replace(/^Best practice:\s*/, '')
}
function drift(r: StageRoute) {
  const c = props.current?.[r.stage_id]
  return !r.overridden && c && (c.method !== r.method || c.model !== r.model) ? c : null
}

function set(i: number, patch: Partial<StageRoute>) {
  const rows = props.rows.map((r, j) => {
    if (j !== i) return r
    const next = { ...r, ...patch }
    const p = props.proposed[r.stage_id]
    next.overridden = !!p && (next.method !== p.method || next.model !== p.model)
    return next
  })
  emit('update:rows', rows)
}

function reset(i: number) {
  const p = props.proposed[props.rows[i].stage_id]
  if (p) set(i, { method: p.method, model: p.model })
}

const overriddenCount = computed(() => props.rows.filter((r) => r.overridden).length)

// Skills: attached per stage; their instructions go into the stage brief when the stage runs.
const MAX_SKILLS = 5
const skillByName = computed(() => new Map((props.skills ?? []).map((s) => [s.name, s])))
const skillsCount = computed(() => props.rows.reduce((n, r) => n + (r.skills?.length ?? 0), 0))
const pickerFor = ref<string | null>(null)
const query = ref('')
const searchEl = ref<HTMLInputElement[] | HTMLInputElement | null>(null)

function attached(r: StageRoute) {
  return r.skills ?? []
}
function suggestionsFor(r: StageRoute) {
  const have = new Set(attached(r))
  return (props.skillSuggestions?.[r.stage_id] ?? []).filter((n) => !have.has(n) && skillByName.value.has(n))
}
function setSkills(i: number, skills: string[]) {
  emit('update:rows', props.rows.map((r, j) => (j === i ? { ...r, skills } : r)))
}
function addSkill(i: number, name: string) {
  const cur = attached(props.rows[i])
  if (cur.includes(name) || cur.length >= MAX_SKILLS) return
  setSkills(i, [...cur, name])
  closePicker()
}
function removeSkill(i: number, name: string) {
  setSkills(i, attached(props.rows[i]).filter((n) => n !== name))
}
async function openPicker(stageId: string) {
  pickerFor.value = pickerFor.value === stageId ? null : stageId
  query.value = ''
  await nextTick()
  const el = Array.isArray(searchEl.value) ? searchEl.value[0] : searchEl.value
  el?.focus()
}
function closePicker() {
  pickerFor.value = null
  query.value = ''
}
function matches(r: StageRoute) {
  const have = new Set(attached(r))
  const q = query.value.trim().toLowerCase()
  return (props.skills ?? [])
    .filter((s) => !have.has(s.name))
    .filter((s) => !q || s.name.toLowerCase().includes(q) || s.description.toLowerCase().includes(q))
    .slice(0, 8)
}
</script>

<template>
  <div class="rounded-lg bg-slate-950 border border-slate-800 overflow-hidden">
    <div class="px-3.5 py-2 border-b border-slate-800 flex items-center justify-between text-[11px]">
      <span class="font-medium text-slate-200">{{ skills ? 'Method, model & skills per stage' : 'Method & model per stage' }}</span>
      <span class="font-mono" :class="overriddenCount ? 'text-amber-400' : 'text-slate-500'">
        {{ overriddenCount ? `${overriddenCount} changed by you` : 'as proposed' }}<template v-if="skillsCount"> · {{ skillsCount }} skill{{ skillsCount === 1 ? '' : 's' }}</template>
      </span>
    </div>
    <div class="overflow-x-auto">
      <table class="w-full text-xs">
        <thead class="text-[10px] font-mono text-slate-500 text-left">
          <tr>
            <th class="px-3 py-1.5 font-normal">Stage</th>
            <th class="px-2 py-1.5 font-normal">Method</th>
            <th class="px-2 py-1.5 font-normal">Model</th>
            <th class="px-2 py-1.5"><span class="sr-only">Reset</span></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/70">
          <template v-for="(r, i) in rows" :key="r.stage_id">
          <tr :class="r.stage_id === currentStage ? 'bg-sky-950/20' : ''">
            <td class="px-3 py-1.5 align-top min-w-[150px]">
              <div class="text-slate-200">{{ stageName(r.stage_id) }}</div>
              <div class="text-[10px] text-slate-500 leading-snug max-w-[260px]" :title="r.reasoning">
                <template v-if="r.overridden">Proposed: {{ methodName(proposed[r.stage_id]?.method) }} on {{ shortModel(proposed[r.stage_id]?.model) }}</template>
                <template v-else>{{ shortReason(r.reasoning) }}</template>
              </div>
              <div v-if="r.wanted && !r.overridden" class="text-[10px] text-amber-300 leading-snug max-w-[260px]">
                Wanted {{ shortModel(r.wanted) }}, not available — using a fallback
              </div>
              <div v-if="drift(r)" class="text-[10px] text-sky-300 leading-snug max-w-[260px]">
                Router now recommends {{ methodName(drift(r)!.method) }} on {{ shortModel(drift(r)!.model) }}
              </div>
            </td>
            <td class="px-2 py-1.5 align-top">
              <select
                :value="r.method"
                :disabled="disabled"
                :aria-label="`Method for ${stageName(r.stage_id)}`"
                class="h-7 px-1.5 bg-slate-900 border rounded text-[11px] text-slate-200 focus:outline-none focus:border-emerald-500"
                :class="r.overridden && r.method !== proposed[r.stage_id]?.method ? 'border-amber-600' : 'border-slate-700'"
                :title="methods.find((m) => m.id === r.method)?.description"
                @change="set(i, { method: ($event.target as HTMLSelectElement).value })"
              >
                <option v-for="m in methods" :key="m.id" :value="m.id" :title="m.description">{{ m.name }}</option>
              </select>
            </td>
            <td class="px-2 py-1.5 align-top">
              <select
                :value="r.model"
                :disabled="disabled"
                :aria-label="`Model for ${stageName(r.stage_id)}`"
                class="h-7 w-full min-w-[13rem] px-1.5 bg-slate-900 border rounded text-[11px] font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
                :class="r.overridden && r.model !== proposed[r.stage_id]?.model ? 'border-amber-600' : 'border-slate-700'"
                @change="set(i, { model: ($event.target as HTMLSelectElement).value, tier: '' })"
              >
                <option v-if="!modelChoices.includes(r.model)" :value="r.model">{{ shortModel(r.model) }} · not in your list</option>
                <option v-for="m in models" :key="m.model" :value="m.model" :title="m.model">{{ modelLabel(m) }}</option>
              </select>
            </td>
            <td class="px-2 py-1.5 align-top">
              <button
                v-if="r.overridden && !disabled"
                type="button"
                class="h-7 w-7 inline-flex items-center justify-center rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800"
                :title="`Reset ${stageName(r.stage_id)} to the proposal`"
                :aria-label="`Reset ${stageName(r.stage_id)} to the proposal`"
                @click="reset(i)"
              >
                <RotateCcw class="w-3.5 h-3.5" />
              </button>
            </td>
          </tr>
          <tr v-if="skills" :class="r.stage_id === currentStage ? 'bg-sky-950/20' : ''" class="!border-t-0">
            <td colspan="4" class="px-3 pb-2 pt-0">
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="text-[10px] font-mono text-slate-500 mr-0.5">Skills</span>
                <span
                  v-for="name in attached(r)"
                  :key="name"
                  class="inline-flex items-center gap-1 h-6 pl-2 pr-1 rounded-full border text-[11px] font-mono"
                  :class="skillByName.has(name) ? 'bg-emerald-950/60 border-emerald-800 text-emerald-200' : 'bg-rose-950/50 border-rose-800 text-rose-200'"
                  :title="skillByName.get(name)?.description || 'This skill is disabled or no longer installed; it will be skipped. Remove it or re-enable it on the Skills page.'"
                >
                  {{ name }}<span v-if="!skillByName.has(name)" class="text-[10px]">· unavailable</span>
                  <button
                    v-if="!disabled"
                    type="button"
                    class="h-4 w-4 inline-flex items-center justify-center rounded-full hover:bg-slate-800"
                    :aria-label="`Remove skill ${name} from ${stageName(r.stage_id)}`"
                    @click="removeSkill(i, name)"
                  >
                    <X class="w-3 h-3" />
                  </button>
                </span>
                <button
                  v-for="name in suggestionsFor(r)"
                  :key="`s-${name}`"
                  type="button"
                  :disabled="disabled || attached(r).length >= MAX_SKILLS"
                  class="inline-flex items-center gap-1 h-6 px-2 rounded-full border border-dashed border-slate-600 text-[11px] font-mono text-slate-400 hover:text-emerald-300 hover:border-emerald-600 disabled:opacity-50"
                  :title="`Suggested for this stage — click to attach.\n${skillByName.get(name)?.description || ''}`"
                  @click="addSkill(i, name)"
                >
                  <Sparkles class="w-3 h-3" /> {{ name }}
                </button>
                <button
                  v-if="!disabled && skills.length && attached(r).length < MAX_SKILLS"
                  type="button"
                  class="inline-flex items-center gap-1 h-6 px-2 rounded-full text-[11px] text-slate-400 hover:text-slate-100 hover:bg-slate-800"
                  :aria-expanded="pickerFor === r.stage_id"
                  :aria-label="`Add a skill to ${stageName(r.stage_id)}`"
                  @click="openPicker(r.stage_id)"
                >
                  <Plus class="w-3 h-3" /> Add skill
                </button>
                <span v-if="!attached(r).length && !suggestionsFor(r).length && skills.length" class="text-[10px] text-slate-600">
                  none — the agent works from the stage brief only
                </span>
              </div>
              <div
                v-if="pickerFor === r.stage_id"
                class="mt-1.5 max-w-xl rounded-lg border border-slate-700 bg-slate-900 shadow-lg"
                @keydown.esc.stop="closePicker"
              >
                <input
                  ref="searchEl"
                  v-model="query"
                  type="text"
                  placeholder="Search skills by name or purpose…"
                  :aria-label="`Search skills for ${stageName(r.stage_id)}`"
                  class="w-full h-8 px-2.5 bg-transparent border-b border-slate-800 text-[11px] text-slate-200 placeholder-slate-500 focus:outline-none"
                  @keydown.enter.prevent="matches(r)[0] && addSkill(i, matches(r)[0].name)"
                />
                <ul class="max-h-60 overflow-y-auto py-1">
                  <li v-for="s in matches(r)" :key="s.name">
                    <button
                      type="button"
                      class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 focus:bg-slate-800 focus:outline-none"
                      @click="addSkill(i, s.name)"
                    >
                      <div class="flex items-baseline justify-between gap-2">
                        <span class="text-[11px] font-mono text-slate-100">{{ s.name }}</span>
                        <span class="text-[10px] font-mono text-slate-500 truncate">{{ s.source }}</span>
                      </div>
                      <div class="text-[10px] text-slate-400 line-clamp-2 leading-snug">{{ s.description }}</div>
                    </button>
                  </li>
                  <li v-if="!matches(r).length" class="px-2.5 py-2 text-[11px] text-slate-500">No skills match “{{ query }}”.</li>
                </ul>
              </div>
            </td>
          </tr>
          </template>
        </tbody>
      </table>
    </div>
    <p v-if="skills" class="px-3.5 py-2 border-t border-slate-800 text-[10px] text-slate-500 leading-snug">
      <span class="font-semibold text-slate-400">Skills</span> — when a stage runs, the instructions of its attached skills go into the agent's brief
      and their folders become readable to it. The task's Activity log lists the skills loaded and which of their files the agent opened.
      <template v-if="!skills.length">No skills can be attached yet: install or enable one on the <RouterLink to="/skills" class="text-emerald-400 hover:underline">Skills page</RouterLink>.</template>
      <template v-else>Suggestions (dashed) are based on the skill name; nothing is attached until you click it.</template>
    </p>
    <div class="px-3.5 py-2 border-t border-slate-800 grid gap-1 sm:grid-cols-2">
      <p v-for="m in methods" :key="m.id" class="text-[10px] text-slate-500 leading-snug">
        <span class="font-semibold text-slate-400">{{ m.name }}</span> — {{ m.description }}
      </p>
    </div>
  </div>
</template>
