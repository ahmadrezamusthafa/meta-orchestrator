<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import ComplexityAssessmentCard from '../routing/ComplexityAssessmentCard.vue'
import RoutingPlanTable from '../routing/RoutingPlanTable.vue'
import type { Task, TaskAnalysisDTO, StageRoute, Complexity, ComplexityAssessment } from '../../types'
import { AlertTriangle, Loader2, Sparkles } from 'lucide-vue-next'

const props = defineProps<{ task: Task }>()
const emit = defineEmits<{ (e: 'updated'): void }>()
const toast = useToastStore()

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const options = ref<Pick<TaskAnalysisDTO, 'methods' | 'models' | 'skills' | 'skill_suggestions'>>({
  methods: [], models: [], skills: [], skill_suggestions: {},
})
const assessment = ref<ComplexityAssessment | null>(null)
const rows = ref<StageRoute[]>([])
// proposed is each row's baseline (its saved value unless the operator changed it); current is what
// the router recommends now, shown as a hint where it differs from a saved, unchanged row.
const proposed = ref<Record<string, { method: string; model: string }>>({})
const current = ref<Record<string, { method: string; model: string }>>({})
const dirty = ref(false)

const confirmed = computed(() => props.task.metadata?.routing_confirmed === 'true')
const running = computed(() => props.task.state === 'RUNNING')

function stagesOf(plan: StageRoute[]) {
  return { start: plan[0]?.stage_id, halt: plan[plan.length - 1]?.stage_id }
}

async function load(regrade?: Complexity) {
  loading.value = true
  error.value = ''
  try {
    const r = await api.getTaskRouting(props.task.id)
    const plan = r.routing_plan ?? []
    const { start, halt } = stagesOf(plan)
    // Re-plan with the task's complexity (no AI call) to know what the router proposes now.
    const cx = (regrade || r.complexity || 'MEDIUM') as Complexity
    const a = await api.analyzeTask({ title: props.task.title, description: props.task.description,
      assigned_repos: props.task.assigned_repos, complexity: cx, task_type: props.task.metadata?.task_type,
      start_stage: start, halt_stage: halt })
    options.value = { methods: a.methods, models: a.models, skills: a.skills ?? [], skill_suggestions: a.skill_suggestions ?? {} }
    current.value = Object.fromEntries(a.plan.map((p) => [p.stage_id, { method: p.method, model: p.model }]))
    assessment.value = {
      ...a.assessment,
      source: regrade ? 'operator' : ((r.complexity_source || 'heuristic') as ComplexityAssessment['source']),
      rationale: regrade ? 'Set by you.' : r.complexity_rationale || a.assessment.rationale,
    }
    const reasoning = Object.fromEntries(a.plan.map((p) => [p.stage_id, p.reasoning]))
    const wanted = Object.fromEntries(a.plan.map((p) => [p.stage_id, p.wanted]))
    const base: StageRoute[] = plan.length ? plan : a.plan.map((p) => ({ stage_id: p.stage_id, method: p.method, model: p.model, tier: p.tier }))
    // On re-grade keep the skills the operator attached but has not saved yet.
    const unsavedSkills = regrade ? new Map(rows.value.map((r) => [r.stage_id, r.skills])) : null
    rows.value = base.map((s) => {
      const c = current.value[s.stage_id]
      // On re-grade, rows the operator never changed follow the new proposal; changed rows keep their choice.
      const follow = regrade && !s.overridden && c
      return { ...s, method: follow ? c.method : s.method, model: follow ? c.model : s.model,
        reasoning: reasoning[s.stage_id] || s.reasoning, overridden: !!s.overridden,
        skills: unsavedSkills?.get(s.stage_id) ?? s.skills ?? [],
        wanted: (follow || (c && c.model === s.model)) ? wanted[s.stage_id] : undefined }
    })
    proposed.value = Object.fromEntries(rows.value.map((r) => [r.stage_id,
      r.overridden && current.value[r.stage_id] ? current.value[r.stage_id] : { method: r.method, model: r.model }]))
    dirty.value = !!regrade
  } catch (e: any) {
    error.value = e?.message || 'Failed to load the routing plan'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await api.updateTaskRouting(props.task.id, {
      complexity: assessment.value?.source === 'operator' ? assessment.value.complexity : undefined,
      routing_plan: rows.value,
    })
    dirty.value = false
    toast.success('Routing plan saved', 'Applies from the next stage run')
    emit('updated')
  } catch (e: any) {
    toast.error('Could not save the routing plan', e?.message || '')
  } finally {
    saving.value = false
  }
}

function onRows(next: StageRoute[]) {
  rows.value = next
  dirty.value = true
}

watch(() => props.task.id, () => load(), { immediate: true })
</script>

<template>
  <div class="h-full overflow-y-auto">
  <div class="max-w-4xl mx-auto px-5 pt-5 space-y-4">
    <div v-if="!confirmed && !loading" class="p-3 rounded-lg bg-amber-950/40 border border-amber-800 flex items-start gap-2 text-xs text-amber-200">
      <AlertTriangle class="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
      <span>This plan was proposed automatically and has not been reviewed. Check the method, model and skills for each stage, then save to confirm it.</span>
    </div>
    <div v-if="running" class="text-[11px] text-sky-300">A stage is running now; changes apply from the next stage run.</div>

    <div v-if="loading && !assessment" class="flex items-center gap-2 text-xs text-slate-400"><Loader2 class="w-4 h-4 animate-spin" /> Loading routing plan…</div>
    <p v-else-if="error" class="text-xs text-rose-400">{{ error }}</p>

    <template v-if="assessment">
      <ComplexityAssessmentCard :assessment="assessment" :busy="loading" @regrade="load" />
      <RoutingPlanTable :rows="rows" :proposed="proposed" :current="current" :methods="options.methods" :models="options.models"
        :skills="options.skills" :skill-suggestions="options.skill_suggestions"
        :disabled="loading || saving" :current-stage="task.current_stage_id" @update:rows="onRows" />
      <div class="sticky bottom-0 -mx-5 px-5 py-3 bg-slate-950/95 border-t border-slate-800 backdrop-blur flex items-center justify-end gap-3">
        <span v-if="dirty" class="text-[11px] text-amber-400 mr-auto">Unsaved changes</span>
        <button type="button" :disabled="saving || loading || (!dirty && confirmed)"
          class="h-8 px-4 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-xs font-medium text-white flex items-center gap-1.5"
          @click="save">
          <Loader2 v-if="saving" class="w-3.5 h-3.5 animate-spin" />
          <Sparkles v-else class="w-3.5 h-3.5" />
          {{ confirmed ? 'Save plan' : 'Confirm plan' }}
        </button>
      </div>
    </template>
  </div>
  </div>
</template>
