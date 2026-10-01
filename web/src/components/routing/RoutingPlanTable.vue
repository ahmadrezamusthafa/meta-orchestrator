<script setup lang="ts">
import { computed } from 'vue'
import type { StageRoute, MethodOptionDTO, ModelOptionDTO } from '../../types'
import { stageName } from '../../composables/taskLifecycle'
import { RotateCcw } from 'lucide-vue-next'

// Editable routing plan: one row per stage with the proposed method and model, which the
// operator can change. A row differing from its proposal is marked overridden.
const props = defineProps<{
  rows: StageRoute[]
  proposed: Record<string, { method: string; model: string }>
  methods: MethodOptionDTO[]
  models: ModelOptionDTO[]
  disabled?: boolean
  currentStage?: string
}>()
const emit = defineEmits<{ (e: 'update:rows', rows: StageRoute[]): void }>()

const methodName = (id: string) => props.methods.find((m) => m.id === id)?.name || id
const modelChoices = computed(() => props.models.map((m) => m.model))

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
</script>

<template>
  <div class="rounded-lg bg-slate-950 border border-slate-800 overflow-hidden">
    <div class="px-3.5 py-2 border-b border-slate-800 flex items-center justify-between text-[11px]">
      <span class="font-medium text-slate-200">Method &amp; model per stage</span>
      <span class="font-mono" :class="overriddenCount ? 'text-amber-400' : 'text-slate-500'">
        {{ overriddenCount ? `${overriddenCount} changed by you` : 'as proposed' }}
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
          <tr v-for="(r, i) in rows" :key="r.stage_id" :class="r.stage_id === currentStage ? 'bg-sky-950/20' : ''">
            <td class="px-3 py-1.5 align-top min-w-[150px]">
              <div class="text-slate-200">{{ stageName(r.stage_id) }}</div>
              <div class="text-[10px] text-slate-500 leading-snug max-w-[260px]" :title="r.reasoning">
                <template v-if="r.overridden">Proposed: {{ methodName(proposed[r.stage_id]?.method) }} on {{ proposed[r.stage_id]?.model }}</template>
                <template v-else>{{ r.reasoning }}</template>
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
                class="h-7 max-w-[230px] px-1.5 bg-slate-900 border rounded text-[11px] font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
                :class="r.overridden && r.model !== proposed[r.stage_id]?.model ? 'border-amber-600' : 'border-slate-700'"
                @change="set(i, { model: ($event.target as HTMLSelectElement).value, tier: '' })"
              >
                <option v-if="!modelChoices.includes(r.model)" :value="r.model">{{ r.model }}</option>
                <option v-for="m in models" :key="m.model" :value="m.model">
                  {{ m.model }}<template v-if="m.tiers?.length"> ({{ m.tiers.join(', ') }})</template>
                </option>
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
        </tbody>
      </table>
    </div>
    <div class="px-3.5 py-2 border-t border-slate-800 grid gap-1 sm:grid-cols-2">
      <p v-for="m in methods" :key="m.id" class="text-[10px] text-slate-500 leading-snug">
        <span class="font-semibold text-slate-400">{{ m.name }}</span> — {{ m.description }}
      </p>
    </div>
  </div>
</template>
