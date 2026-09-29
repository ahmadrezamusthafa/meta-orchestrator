<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ProviderUsage, ProviderUsageWindow } from '../../types'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { formatCost, formatTokens } from '../console/consoleFormat'
import { Activity, Gauge, Pencil, ShieldCheck } from 'lucide-vue-next'

const props = defineProps<{
  providerId: string
  usage?: ProviderUsage
  window: ProviderUsageWindow
  loading?: boolean
}>()

const emit = defineEmits<{ (e: 'quota-saved'): void }>()

const toastStore = useToastStore()

const editingQuota = ref(false)
const tokenLimitInput = ref('')
const costLimitInput = ref('')
const savingQuota = ref(false)

const w = computed(() => props.usage?.window)
const limits = computed(() => props.usage?.limits)
const showLimits = computed(() => !!limits.value && (limits.value.status !== 'unavailable' || props.providerId !== 'opencode'))

function resetsIn(at?: string): string {
  if (!at) return ''
  const mins = Math.round((new Date(at).getTime() - Date.now()) / 60000)
  if (mins <= 0) return 'resets now'
  if (mins < 60) return `resets in ${mins}m`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `resets in ${hrs}h ${mins % 60}m`
  const days = Math.floor(hrs / 24)
  return `resets in ${days}d ${hrs % 24}h`
}

function resetsTitle(at?: string): string {
  return at ? `Resets ${new Date(at).toLocaleString()}` : ''
}
const quota = computed(() => props.usage?.quota)
const hasQuota = computed(() => !!quota.value && (quota.value.monthly_token_limit > 0 || quota.value.monthly_cost_limit_usd > 0))
const topModels = computed(() => (props.usage?.models || []).slice(0, 3))

const lastUsed = computed(() => {
  const at = props.usage?.last_used_at
  if (!at) return 'never'
  const mins = Math.round((Date.now() - new Date(at).getTime()) / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.round(mins / 60)
  if (hrs < 48) return `${hrs}h ago`
  return `${Math.round(hrs / 24)}d ago`
})

function barClass(pct: number | undefined): string {
  const p = pct || 0
  if (p >= 100) return 'bg-rose-500'
  if (p >= 80) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function barWidth(pct: number | undefined): string {
  return `${Math.min(100, Math.max(0, pct || 0))}%`
}

function startEditQuota() {
  tokenLimitInput.value = quota.value?.monthly_token_limit ? String(quota.value.monthly_token_limit) : ''
  costLimitInput.value = quota.value?.monthly_cost_limit_usd ? String(quota.value.monthly_cost_limit_usd) : ''
  editingQuota.value = true
}

async function saveQuota() {
  const tokens = tokenLimitInput.value.trim() === '' ? 0 : Number(tokenLimitInput.value)
  const cost = costLimitInput.value.trim() === '' ? 0 : Number(costLimitInput.value)
  if (!Number.isFinite(tokens) || tokens < 0 || !Number.isFinite(cost) || cost < 0) {
    toastStore.warning('Invalid Quota', 'Limits must be empty (no limit) or a positive number')
    return
  }
  savingQuota.value = true
  try {
    const res = await api.saveProviderQuota(props.providerId, {
      monthly_token_limit: Math.round(tokens),
      monthly_cost_limit_usd: cost,
    })
    if (res?.warning) toastStore.warning('Quota Saved For This Session', res.warning)
    else toastStore.success('Quota Saved', tokens || cost ? 'Monthly quota updated' : 'Quota cleared')
    editingQuota.value = false
    emit('quota-saved')
  } catch (err: any) {
    toastStore.error('Save Failed', err.message || 'Unable to save quota')
  } finally {
    savingQuota.value = false
  }
}
</script>

<template>
  <div class="pt-2 border-t border-slate-800/80 space-y-2 text-xs">
    <div class="flex items-center justify-between">
      <label class="text-[11px] text-slate-400 flex items-center gap-1">
        <Activity class="w-3 h-3 text-slate-500" />
        <span>Usage · last {{ window }}</span>
      </label>
      <span class="text-[9px] font-mono text-slate-500" :title="usage?.last_used_at || ''">used {{ lastUsed }}</span>
    </div>

    <div v-if="loading && !usage" class="h-14 rounded-lg bg-slate-950 border border-slate-800 animate-pulse"></div>

    <template v-else>
      <!-- Provider-reported plan limits (like `claude /usage`) -->
      <div v-if="showLimits && limits" class="p-2 rounded-lg bg-slate-950 border border-slate-800 space-y-2">
        <div class="flex items-center justify-between">
          <span class="text-[10px] text-slate-400 flex items-center gap-1" :title="limits.source">
            <ShieldCheck class="w-3 h-3 text-slate-500" />
            <span>Plan limits</span>
            <span v-if="limits.plan" class="ml-1 px-1 rounded bg-slate-800 text-[9px] font-mono text-slate-300">{{ limits.plan }}</span>
          </span>
          <span
            class="text-[9px] font-mono"
            :class="limits.status === 'ok' ? 'text-emerald-400' : limits.status === 'stale' ? 'text-amber-400' : 'text-slate-500'"
          >
            {{ limits.status === 'ok' ? 'live' : limits.status }}
          </span>
        </div>

        <template v-if="limits.status === 'ok' && limits.windows.length">
          <div v-for="lw in limits.windows" :key="lw.id" class="space-y-0.5">
            <div class="flex items-baseline justify-between gap-2 text-[10px]">
              <span class="truncate text-slate-300" :title="lw.models?.join(', ') || lw.label">{{ lw.label }}</span>
              <span class="shrink-0 font-mono text-slate-200">{{ lw.used_percent.toFixed(0) }}% used</span>
            </div>
            <div class="h-1.5 rounded-full bg-slate-800 overflow-hidden">
              <div class="h-full rounded-full transition-all" :class="barClass(lw.used_percent)" :style="{ width: barWidth(lw.used_percent) }"></div>
            </div>
            <div v-if="lw.resets_at" class="text-[9px] font-mono text-slate-500" :title="resetsTitle(lw.resets_at)">
              {{ resetsIn(lw.resets_at) }}
            </div>
          </div>
        </template>
        <p v-else-if="limits.status === 'ok'" class="text-[9px] text-slate-500">No limits reported for this plan.</p>
        <p v-else class="text-[9px] leading-relaxed" :class="limits.status === 'stale' ? 'text-amber-300/80' : 'text-slate-500'">
          {{ limits.message }}
        </p>
      </div>

      <!-- Totals -->
      <div class="grid grid-cols-3 gap-1.5">
        <div class="p-2 rounded-lg bg-slate-950 border border-slate-800">
          <div class="text-[9px] text-slate-500">Tokens</div>
          <div class="text-sm font-mono text-slate-100">{{ formatTokens(w?.total_tokens) }}</div>
        </div>
        <div class="p-2 rounded-lg bg-slate-950 border border-slate-800">
          <div class="text-[9px] text-slate-500">Cost</div>
          <div class="text-sm font-mono text-slate-100">{{ formatCost(w?.cost_usd) }}</div>
        </div>
        <div class="p-2 rounded-lg bg-slate-950 border border-slate-800">
          <div class="text-[9px] text-slate-500">Calls</div>
          <div class="text-sm font-mono text-slate-100">{{ w?.calls || 0 }}</div>
        </div>
      </div>
      <div class="text-[9px] font-mono text-slate-500">
        {{ formatTokens(w?.prompt_tokens) }} in · {{ formatTokens(w?.completion_tokens) }} out
        <template v-if="w?.cached_tokens"> · {{ formatTokens(w?.cached_tokens) }} cached</template>
        · 24h {{ formatTokens(usage?.last_24h?.total_tokens) }}
      </div>

      <!-- Top models -->
      <div v-if="topModels.length" class="space-y-0.5">
        <div
          v-for="m in topModels"
          :key="m.model"
          class="flex items-center justify-between gap-2 text-[10px] font-mono"
        >
          <span class="truncate text-slate-400" :title="m.model">{{ m.model }}</span>
          <span class="shrink-0 text-slate-500">{{ formatTokens(m.total_tokens) }} · {{ formatCost(m.cost_usd) }}</span>
        </div>
      </div>

      <!-- Quota -->
      <div class="p-2 rounded-lg bg-slate-950 border border-slate-800 space-y-1.5">
        <div class="flex items-center justify-between">
          <span class="text-[10px] text-slate-400 flex items-center gap-1">
            <Gauge class="w-3 h-3 text-slate-500" />
            <span>Monthly quota</span>
            <span v-if="quota?.exceeded" class="ml-1 px-1 rounded bg-rose-950 border border-rose-800 text-[9px] text-rose-300">exceeded</span>
          </span>
          <button
            v-if="!editingQuota"
            type="button"
            class="text-[10px] text-sky-400 hover:text-sky-300 flex items-center gap-1"
            @click="startEditQuota"
          >
            <Pencil class="w-2.5 h-2.5" />
            <span>{{ hasQuota ? 'Edit' : 'Set quota' }}</span>
          </button>
        </div>

        <template v-if="!editingQuota">
          <template v-if="hasQuota">
            <div v-if="quota!.monthly_token_limit > 0" class="space-y-0.5">
              <div class="flex justify-between text-[9px] font-mono text-slate-500">
                <span>{{ formatTokens(usage?.month_to_date?.total_tokens) }} / {{ formatTokens(quota!.monthly_token_limit) }} tokens</span>
                <span>{{ (quota!.token_percent || 0).toFixed(0) }}%</span>
              </div>
              <div class="h-1.5 rounded-full bg-slate-800 overflow-hidden">
                <div class="h-full rounded-full transition-all" :class="barClass(quota!.token_percent)" :style="{ width: barWidth(quota!.token_percent) }"></div>
              </div>
            </div>
            <div v-if="quota!.monthly_cost_limit_usd > 0" class="space-y-0.5">
              <div class="flex justify-between text-[9px] font-mono text-slate-500">
                <span>{{ formatCost(usage?.month_to_date?.cost_usd) }} / {{ formatCost(quota!.monthly_cost_limit_usd) }}</span>
                <span>{{ (quota!.cost_percent || 0).toFixed(0) }}%</span>
              </div>
              <div class="h-1.5 rounded-full bg-slate-800 overflow-hidden">
                <div class="h-full rounded-full transition-all" :class="barClass(quota!.cost_percent)" :style="{ width: barWidth(quota!.cost_percent) }"></div>
              </div>
            </div>
          </template>
          <div v-else class="text-[9px] font-mono text-slate-500">
            No limit · {{ formatTokens(usage?.month_to_date?.total_tokens) }} tokens, {{ formatCost(usage?.month_to_date?.cost_usd) }} this month
          </div>
        </template>

        <div v-else class="space-y-1.5">
          <div class="grid grid-cols-2 gap-1.5">
            <input
              v-model="tokenLimitInput"
              type="number"
              min="0"
              placeholder="Token limit"
              class="h-7 px-2 bg-slate-900 border border-slate-700 rounded text-[10px] font-mono text-slate-200 focus:outline-none focus:border-sky-500"
            />
            <input
              v-model="costLimitInput"
              type="number"
              min="0"
              step="0.01"
              placeholder="USD limit"
              class="h-7 px-2 bg-slate-900 border border-slate-700 rounded text-[10px] font-mono text-slate-200 focus:outline-none focus:border-sky-500"
            />
          </div>
          <div class="flex gap-1.5">
            <button
              type="button"
              :disabled="savingQuota"
              class="flex-1 h-7 rounded bg-sky-950 hover:bg-sky-900 border border-sky-800 text-[10px] font-medium text-sky-200 disabled:opacity-40"
              @click="saveQuota"
            >
              {{ savingQuota ? 'Saving…' : 'Save' }}
            </button>
            <button
              type="button"
              class="h-7 px-2 rounded text-[10px] text-slate-500 hover:text-slate-300"
              @click="editingQuota = false"
            >
              Cancel
            </button>
          </div>
          <p class="text-[9px] text-slate-500">Leave empty for no limit. Measured month-to-date.</p>
        </div>
      </div>

      <p class="text-[9px] text-slate-600 leading-relaxed">
        Token and cost totals count only calls made by this orchestrator.<template v-if="limits?.status === 'ok'"> Plan limits come from the provider, via your local {{ providerId === 'claude' ? 'Claude Code' : 'Antigravity' }} login.</template>
      </p>
    </template>
  </div>
</template>
