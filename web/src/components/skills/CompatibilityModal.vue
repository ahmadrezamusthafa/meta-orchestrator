<script setup lang="ts">
import { computed } from 'vue'
import type { UniversalSkillDTO } from '../../types'
import {
  X,
  ShieldCheck,
  ShieldAlert,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Info,
  Terminal,
  Clock,
  Globe,
  Lock,
} from 'lucide-vue-next'

const props = defineProps<{
  skill: UniversalSkillDTO
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'toggle', enabled: boolean): void
}>()

const compat = computed(() => props.skill.compatibility)
const score = computed(() => compat.value?.score ?? 100)
const status = computed(() => compat.value?.status || 'compatible')
const issues = computed(() => compat.value?.issues || [])

const statusTheme = computed(() => {
  switch (status.value) {
    case 'compatible':
      return {
        badge: 'bg-emerald-950/80 text-emerald-300 border-emerald-800',
        ring: 'text-emerald-400 stroke-emerald-500',
        title: 'Meta-Orchestrator Compatible',
      }
    case 'warning':
      return {
        badge: 'bg-amber-950/80 text-amber-300 border-amber-800',
        ring: 'text-amber-400 stroke-amber-500',
        title: 'Compatibility Warnings Detected',
      }
    case 'incompatible':
      return {
        badge: 'bg-rose-950/80 text-rose-300 border-rose-800',
        ring: 'text-rose-400 stroke-rose-500',
        title: 'Incompatible with Zero-Trust Execution',
      }
    default:
      return {
        badge: 'bg-slate-800 text-slate-300 border-slate-700',
        ring: 'text-slate-400 stroke-slate-500',
        title: 'Audit Pending',
      }
  }
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm animate-in fade-in duration-200">
    <div
      class="w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]"
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b border-slate-800 bg-slate-900/60 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-slate-800 border border-slate-700 flex items-center justify-center">
            <ShieldCheck v-if="status === 'compatible'" class="w-5 h-5 text-emerald-400" />
            <AlertTriangle v-else-if="status === 'warning'" class="w-5 h-5 text-amber-400" />
            <ShieldAlert v-else class="w-5 h-5 text-rose-400" />
          </div>
          <div>
            <h3 class="text-base font-bold text-slate-100 flex items-center gap-2">
              <span>{{ skill.name }}</span>
              <span class="px-2 py-0.5 rounded text-[10px] uppercase font-mono font-bold tracking-wider" :class="statusTheme.badge">
                {{ status }}
              </span>
            </h3>
            <p class="text-xs text-slate-400 font-mono truncate max-w-md">
              {{ skill.source_location || 'Built-in Native Kernel' }}
            </p>
          </div>
        </div>

        <button
          type="button"
          @click="emit('close')"
          class="w-8 h-8 rounded-lg flex items-center justify-center text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Body / Scrollable details -->
      <div class="p-6 overflow-y-auto space-y-6">
        <!-- Score Card Banner -->
        <div class="p-4 rounded-xl bg-slate-950/70 border border-slate-800/80 flex items-center justify-between">
          <div class="space-y-1">
            <span class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Zero-Trust Compatibility Score</span>
            <div class="text-2xl font-bold font-mono" :class="{
              'text-emerald-400': status === 'compatible',
              'text-amber-400': status === 'warning',
              'text-rose-400': status === 'incompatible'
            }">
              {{ score }} / 100
            </div>
            <p class="text-xs text-slate-400">
              {{ statusTheme.title }}
            </p>
          </div>

          <div class="flex items-center gap-3 text-xs">
            <div class="flex flex-col items-center p-2 rounded-lg bg-slate-900 border border-slate-800">
              <span class="text-slate-500 text-[10px]">RUNTIME</span>
              <span v-if="compat?.runtime_ready" class="text-emerald-400 font-semibold flex items-center gap-1">
                <CheckCircle2 class="w-3.5 h-3.5" /> Ready
              </span>
              <span v-else class="text-rose-400 font-semibold flex items-center gap-1">
                <XCircle class="w-3.5 h-3.5" /> Missing
              </span>
            </div>

            <div class="flex flex-col items-center p-2 rounded-lg bg-slate-900 border border-slate-800">
              <span class="text-slate-500 text-[10px]">SANDBOX</span>
              <span v-if="compat?.sandbox_safe" class="text-emerald-400 font-semibold flex items-center gap-1">
                <Lock class="w-3.5 h-3.5" /> Isolated
              </span>
              <span v-else class="text-amber-400 font-semibold flex items-center gap-1">
                <AlertTriangle class="w-3.5 h-3.5" /> Host Mode
              </span>
            </div>
          </div>
        </div>

        <!-- Capability & Runtime Environment Matrix -->
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
          <div class="p-3 rounded-lg bg-slate-950/50 border border-slate-800">
            <div class="text-slate-500 text-[10px] uppercase font-semibold mb-1 flex items-center gap-1">
              <Terminal class="w-3 h-3" /> Source Format
            </div>
            <div class="text-slate-200 font-mono font-medium uppercase">{{ skill.source_format }}</div>
          </div>

          <div class="p-3 rounded-lg bg-slate-950/50 border border-slate-800">
            <div class="text-slate-500 text-[10px] uppercase font-semibold mb-1 flex items-center gap-1">
              <Lock class="w-3 h-3" /> Isolation
            </div>
            <div class="text-slate-200 font-mono font-medium capitalize">{{ skill.isolation }}</div>
          </div>

          <div class="p-3 rounded-lg bg-slate-950/50 border border-slate-800">
            <div class="text-slate-500 text-[10px] uppercase font-semibold mb-1 flex items-center gap-1">
              <Clock class="w-3 h-3" /> Timeout
            </div>
            <div class="text-slate-200 font-mono font-medium">{{ skill.timeout_seconds || 120 }}s</div>
          </div>

          <div class="p-3 rounded-lg bg-slate-950/50 border border-slate-800">
            <div class="text-slate-500 text-[10px] uppercase font-semibold mb-1 flex items-center gap-1">
              <Globe class="w-3 h-3" /> Egress
            </div>
            <div class="text-slate-200 font-mono font-medium">
              {{ skill.requires_network ? 'Required' : 'Air-gapped' }}
            </div>
          </div>
        </div>

        <!-- Issues and Diagnostics Checklist -->
        <div class="space-y-3">
          <h4 class="text-xs font-semibold uppercase tracking-wider text-slate-400 flex items-center justify-between">
            <span>Diagnostic Findings ({{ issues.length }})</span>
            <span class="text-[10px] text-slate-500 font-normal">Audited against SDLC Phase 1 Spec</span>
          </h4>

          <div v-if="issues.length === 0" class="p-4 rounded-xl bg-slate-950/40 border border-slate-800 text-center text-xs text-slate-400">
            <CheckCircle2 class="w-6 h-6 text-emerald-400 mx-auto mb-1.5" />
            All verification checks passed with zero issues.
          </div>

          <div v-else class="space-y-2.5">
            <div
              v-for="(issue, idx) in issues"
              :key="idx"
              class="p-3.5 rounded-xl border transition-colors"
              :class="{
                'bg-rose-950/20 border-rose-800/60': issue.severity === 'error',
                'bg-amber-950/20 border-amber-800/60': issue.severity === 'warning',
                'bg-slate-950/40 border-slate-800': issue.severity === 'info'
              }"
            >
              <div class="flex items-start gap-2.5">
                <XCircle v-if="issue.severity === 'error'" class="w-4 h-4 text-rose-400 flex-shrink-0 mt-0.5" />
                <AlertTriangle v-else-if="issue.severity === 'warning'" class="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
                <Info v-else class="w-4 h-4 text-slate-400 flex-shrink-0 mt-0.5" />

                <div class="flex-1 space-y-1">
                  <div class="flex items-center gap-2">
                    <span
                      class="px-1.5 py-0.5 rounded text-[10px] font-mono uppercase font-bold"
                      :class="{
                        'bg-rose-900/60 text-rose-300': issue.severity === 'error',
                        'bg-amber-900/60 text-amber-300': issue.severity === 'warning',
                        'bg-slate-800 text-slate-400': issue.severity === 'info'
                      }"
                    >
                      {{ issue.severity }}
                    </span>
                    <span class="text-[10px] font-mono uppercase text-slate-400 tracking-wider">
                      [{{ issue.check }}]
                    </span>
                  </div>

                  <p class="text-xs text-slate-200">
                    {{ issue.message }}
                  </p>

                  <div
                    v-if="issue.suggestion"
                    class="mt-1.5 p-2 rounded bg-slate-950/80 border border-slate-800/80 text-[11px] text-slate-300 flex items-start gap-1.5"
                  >
                    <span class="text-emerald-400 font-bold font-mono">FIX:</span>
                    <span>{{ issue.suggestion }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-6 py-3.5 border-t border-slate-800 bg-slate-900/60 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="text-xs text-slate-400">Execution Status:</span>
          <span class="text-xs font-semibold font-mono" :class="skill.enabled ? 'text-emerald-400' : 'text-slate-500'">
            {{ skill.enabled ? 'ENABLED' : 'DISABLED' }}
          </span>
        </div>

        <div class="flex items-center gap-2.5">
          <button
            type="button"
            @click="emit('toggle', !skill.enabled)"
            class="px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-colors shadow-sm"
            :class="skill.enabled
              ? 'bg-rose-950/80 hover:bg-rose-900 text-rose-300 border border-rose-800'
              : 'bg-emerald-950/80 hover:bg-emerald-900 text-emerald-300 border border-emerald-800'"
          >
            {{ skill.enabled ? 'Disable Skill' : 'Enable Skill' }}
          </button>
          <button
            type="button"
            @click="emit('close')"
            class="px-4 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
