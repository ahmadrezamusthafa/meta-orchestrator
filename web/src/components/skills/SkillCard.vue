<script setup lang="ts">
import { computed } from 'vue'
import type { UniversalSkillDTO } from '../../types'
import CompatibilityBadge from './CompatibilityBadge.vue'
import {
  Sparkles,
  Terminal,
  Shield,
  Clock,
  Globe,
  Lock,
  Layers,
  FileCode2,
} from 'lucide-vue-next'

const props = defineProps<{
  skill: UniversalSkillDTO
}>()

const emit = defineEmits<{
  (e: 'toggle', enabled: boolean): void
  (e: 'inspect-compatibility'): void
}>()

const formatBadge = computed(() => {
  switch (props.skill.source_format) {
    case 'claude':
      return { label: 'Claude SKILL.md', class: 'bg-amber-950/60 text-amber-300 border-amber-800/80' }
    case 'mcp':
      return { label: 'MCP Connector', class: 'bg-indigo-950/60 text-indigo-300 border-indigo-800/80' }
    case 'native':
      return { label: 'Native Go', class: 'bg-emerald-950/60 text-emerald-300 border-emerald-800/80' }
    case 'bmad':
      return { label: 'BMAD Pack', class: 'bg-cyan-950/60 text-cyan-300 border-cyan-800/80' }
    case 'superpower':
      return { label: 'Superpower', class: 'bg-purple-950/60 text-purple-300 border-purple-800/80' }
    default:
      return { label: props.skill.source_format, class: 'bg-slate-800 text-slate-300 border-slate-700' }
  }
})

const sourceTypeLabel = computed(() => {
  switch (props.skill.source_type) {
    case 'builtin':
      return 'Core Kernel'
    case 'mcp_connector':
      return 'Live Connector'
    case 'claude':
      return 'Auto-detected Claude'
    case 'custom_dir':
      return 'External Directory'
    case 'project_local':
      return 'Project Local'
    case 'system':
      return 'System User'
    default:
      return 'Modular Skill'
  }
})

function handleToggle() {
  emit('toggle', !props.skill.enabled)
}
</script>

<template>
  <div
    class="rounded-2xl border transition-all duration-200 flex flex-col justify-between overflow-hidden shadow-sm"
    :class="skill.enabled
      ? 'bg-slate-900/90 border-slate-800 hover:border-slate-700'
      : 'bg-slate-950/60 border-slate-800/50 opacity-75'"
  >
    <!-- Top Bar -->
    <div class="p-5 space-y-3.5">
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 flex-wrap">
          <span class="px-2.5 py-0.5 rounded-full text-[10px] font-mono uppercase font-bold border" :class="formatBadge.class">
            {{ formatBadge.label }}
          </span>
          <span class="text-[10px] text-slate-400 font-mono flex items-center gap-1">
            <Layers class="w-3 h-3 text-slate-500" />
            {{ sourceTypeLabel }}
          </span>
        </div>

        <!-- Toggle Switch (Consistent iOS/Tailwind style, no raw checkbox) -->
        <button
          type="button"
          @click="handleToggle"
          class="relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
          :class="skill.enabled ? 'bg-emerald-600' : 'bg-slate-800'"
          :title="skill.enabled ? 'Click to disable skill' : 'Click to enable skill'"
        >
          <span
            aria-hidden="true"
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="skill.enabled ? 'translate-x-5' : 'translate-x-0'"
          />
        </button>
      </div>

      <!-- Skill Title & Compatibility Score Badge -->
      <div class="space-y-1">
        <div class="flex items-start justify-between gap-2">
          <h4 class="text-sm font-bold text-slate-100 font-mono tracking-tight break-all">
            {{ skill.name }}
          </h4>
          <CompatibilityBadge
            :compatibility="skill.compatibility"
            :interactive="true"
            @click="emit('inspect-compatibility')"
          />
        </div>
        <p class="text-xs text-slate-400 line-clamp-2 leading-relaxed">
          {{ skill.description || 'No description provided for this skill.' }}
        </p>
      </div>

      <!-- Command or Entrypoint info -->
      <div
        v-if="skill.command || (skill.args && skill.args.length > 0)"
        class="px-2.5 py-1.5 rounded-lg bg-slate-950/80 border border-slate-800/80 text-[11px] font-mono text-slate-300 flex items-center gap-2 truncate"
      >
        <Terminal class="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" />
        <span class="truncate">{{ skill.command }} {{ skill.args ? skill.args.join(' ') : '' }}</span>
      </div>

      <!-- Roles Requirements -->
      <div v-if="skill.required_roles && skill.required_roles.length > 0" class="flex items-center gap-1.5 flex-wrap">
        <span class="text-[10px] text-slate-500 font-semibold uppercase">Roles:</span>
        <span
          v-for="r in skill.required_roles"
          :key="r"
          class="px-2 py-0.5 rounded bg-slate-800/80 border border-slate-700/80 text-[10px] font-mono text-slate-300"
        >
          {{ r }}
        </span>
      </div>
    </div>

    <!-- Bottom Metadata Bar -->
    <div class="px-5 py-3 bg-slate-950/50 border-t border-slate-800/70 flex items-center justify-between gap-2 text-xs">
      <div class="flex items-center gap-3 text-slate-400 text-[11px]">
        <span class="flex items-center gap-1" :title="`Isolation: ${skill.isolation}`">
          <Lock class="w-3 h-3 text-slate-500" />
          <span class="capitalize">{{ skill.isolation }}</span>
        </span>
        <span class="flex items-center gap-1" :title="`Timeout: ${skill.timeout_seconds || 120}s`">
          <Clock class="w-3 h-3 text-slate-500" />
          <span>{{ skill.timeout_seconds || 120 }}s</span>
        </span>
        <span class="flex items-center gap-1" :title="skill.requires_network ? 'Requires Outbound Network' : 'Air-gapped'">
          <Globe class="w-3 h-3" :class="skill.requires_network ? 'text-cyan-400' : 'text-slate-500'" />
          <span>{{ skill.requires_network ? 'Net' : 'Local' }}</span>
        </span>
      </div>

      <button
        type="button"
        @click="emit('inspect-compatibility')"
        class="text-[11px] font-semibold text-emerald-400 hover:text-emerald-300 flex items-center gap-1 transition-colors"
      >
        <Sparkles class="w-3 h-3" />
        <span>Audit Details</span>
      </button>
    </div>
  </div>
</template>
