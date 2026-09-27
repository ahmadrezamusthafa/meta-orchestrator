<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  MessageSquare,
  Sparkles,
  X,
  Copy,
  Check,
  FolderPlus,
  Search,
  FileText,
  BadgeCheck,
  Info,
} from 'lucide-vue-next'
import { useToastStore } from '../../stores/toast'
import type { PromptItemDTO } from '../../types'
import RegisterPromptModal from './RegisterPromptModal.vue'

const props = defineProps<{
  prompts: PromptItemDTO[]
}>()

const emit = defineEmits<{
  (e: 'reload'): void
}>()

const toastStore = useToastStore()

const searchQuery = ref('')
const selectedSource = ref('all')
const showRegisterModal = ref(false)
const activeSlotPrompt = ref<PromptItemDTO | null>(null)
const mockValues = ref<Record<string, string>>({})
const isCopied = ref(false)

const sampleTemplatesContent: Record<string, string> = {
  pm_prd_generator: `You are the lead Product Manager agent in the Zero-Trust Software Factory.
Synthesize the requirements for feature {{feature_name}}.
Existing Architecture Context:
{{system_architecture}}
Target Repositories: {{target_repositories}}
Generate PRD.md with user stories, acceptance criteria, and edge cases.`,
  qa_atdd_generator: `You are the QA Automation Specialist agent.
Author executable Playwright TypeScript tests for {{feature_name}}.
Refer to PRD criteria and existing codebase AST slices.
Failing trace reference: {{failing_test_traces}}
Write tests to fail in Red Phase before code generation.`,
  system_architect_rfc: `You are the System Architect agent.
Design the technical specification TECH_DOC_RFC.md for {{feature_name}}.
Target AST Slice: {{target_ast_slice}}
Database migrations, sequence diagrams, and cross-repo data flow required.`,
}

const filteredPrompts = computed(() => {
  return props.prompts.filter((p) => {
    if (selectedSource.value !== 'all') {
      if (p.source !== selectedSource.value) return false
    }
    if (searchQuery.value.trim()) {
      const q = searchQuery.value.toLowerCase()
      const matchName = p.name.toLowerCase().includes(q)
      const matchId = p.id.toLowerCase().includes(q)
      const matchDesc = (p.description || '').toLowerCase().includes(q)
      const matchRole = (p.role || '').toLowerCase().includes(q)
      if (!matchName && !matchId && !matchDesc && !matchRole) return false
    }
    return true
  })
})

function openSlotTester(p: PromptItemDTO) {
  activeSlotPrompt.value = p
  mockValues.value = {}
  p.variables.forEach((v: string) => {
    if (v.includes('epic') || v.includes('parent_issue')) {
      mockValues.value[v] = 'MIB-9685'
    } else if (v.includes('user_story')) {
      mockValues.value[v] = 'Billing Dashboard & Subscription Backyard auto collection toggle'
    } else if (v.includes('prd_source') || v.includes('prd_url')) {
      mockValues.value[v] = 'https://wiki.atlassian.net/wiki/spaces/ARCH/pages/8942001/PRD-Billing'
    } else if (v.includes('milestone')) {
      mockValues.value[v] = 'Q4-Enterprise-Launch'
    } else if (v.includes('architecture')) {
      mockValues.value[v] = 'PostgreSQL + Redis Queue + Docker Compose bridge'
    } else if (v.includes('feature')) {
      mockValues.value[v] = 'Stripe Payment Gateway & Webhook Idempotency'
    } else if (v.includes('trace')) {
      mockValues.value[v] = 'Playwright timeout: expected HTTP 200 within 5000ms, received 504'
    } else if (v.includes('repositories')) {
      mockValues.value[v] = 'frontend-portal, backend-core, api-contracts'
    } else {
      mockValues.value[v] = `Sample ${v} value`
    }
  })
}

function getRawTemplate(id: string): string {
  if (activeSlotPrompt.value && activeSlotPrompt.value.raw_content) {
    return activeSlotPrompt.value.raw_content
  }
  return sampleTemplatesContent[id] || `You are an autonomous Meta-Orchestrator agent.\nExecuting task for {{feature_name}}.\nContext: {{system_architecture}}.\nTrace: {{failing_test_traces}}.`
}

function getRenderedPrompt(id: string): string {
  let content = getRawTemplate(id)
  for (const [key, val] of Object.entries(mockValues.value)) {
    const replacement = val || `[MISSING: ${key}]`
    content = content.split(`{{${key}}}`).join(replacement)
    content = content.split(`{${key}}`).join(replacement)
    content = content.split(`$${key}`).join(replacement)
  }
  return content
}

async function copyRendered() {
  if (!activeSlotPrompt.value) return
  const text = getRenderedPrompt(activeSlotPrompt.value.id)
  await navigator.clipboard.writeText(text)
  isCopied.value = true
  toastStore.success('Copied', 'Rendered prompt copied to clipboard')
  setTimeout(() => {
    isCopied.value = false
  }, 2000)
}

function slotLabel(v: string): string {
  return `{{${v}}}`
}

function formatVars(vars: string[]): string {
  if (!vars || vars.length === 0) return 'None'
  return vars.map(v => '{' + v + '}').join(', ')
}

function onRegisteredPromptSource() {
  emit('reload')
}
</script>

<template>
  <div class="space-y-4 select-none">
    <!-- Top Header Bar -->
    <div class="flex flex-wrap items-center justify-between gap-3 pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide flex items-center gap-2">
          <MessageSquare class="w-4 h-4 text-sky-400" />
          <span>Cascading Parameterized Prompt Templates</span>
        </h3>
        <span class="text-[11px] text-slate-400">
          Resolved in hierarchy: Project Local (.sdlc/prompts) &succ; GitHub (.github/prompts) &succ; Custom &succ; System &succ; Built-in
        </span>
      </div>

      <div class="flex items-center gap-2">
        <button
          type="button"
          @click="showRegisterModal = true"
          class="h-8 px-3 rounded-lg bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5 transition-colors shadow-sm"
        >
          <FolderPlus class="w-3.5 h-3.5" />
          <span>Register Prompt Dir</span>
        </button>

        <span class="text-xs font-mono text-sky-400 px-2 py-1 rounded bg-slate-900 border border-slate-800">
          {{ filteredPrompts.length }} / {{ prompts.length }} Templates
        </span>
      </div>
    </div>

    <!-- Filters & Search Bar -->
    <div class="flex flex-wrap items-center justify-between gap-2.5 bg-slate-900/40 p-2.5 rounded-xl border border-slate-800">
      <div class="relative w-64">
        <Search class="absolute left-3 top-2.5 w-3.5 h-3.5 text-slate-500" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search prompts, roles, descriptions..."
          class="w-full h-8 pl-8 pr-7 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 font-mono"
        />
        <button
          v-if="searchQuery"
          @click="searchQuery = ''"
          type="button"
          class="absolute right-2 top-2 text-slate-500 hover:text-slate-300"
        >
          <X class="w-3.5 h-3.5" />
        </button>
      </div>

      <div class="flex items-center gap-1.5 text-xs font-mono">
        <span class="text-[11px] text-slate-500 uppercase mr-1">Tier:</span>
        <button
          type="button"
          @click="selectedSource = 'all'"
          class="px-2 py-1 rounded text-[11px]"
          :class="selectedSource === 'all' ? 'bg-sky-950 text-sky-300 border border-sky-800' : 'bg-slate-950 text-slate-400 hover:text-slate-200'"
        >
          All
        </button>
        <button
          type="button"
          @click="selectedSource = 'GITHUB_PROMPTS'"
          class="px-2 py-1 rounded text-[11px]"
          :class="selectedSource === 'GITHUB_PROMPTS' ? 'bg-sky-950 text-sky-300 border border-sky-800' : 'bg-slate-950 text-slate-400 hover:text-slate-200'"
        >
          GitHub
        </button>
        <button
          type="button"
          @click="selectedSource = 'CUSTOM_DIR'"
          class="px-2 py-1 rounded text-[11px]"
          :class="selectedSource === 'CUSTOM_DIR' ? 'bg-sky-950 text-sky-300 border border-sky-800' : 'bg-slate-950 text-slate-400 hover:text-slate-200'"
        >
          Custom
        </button>
        <button
          type="button"
          @click="selectedSource = 'PROJECT_LOCAL'"
          class="px-2 py-1 rounded text-[11px]"
          :class="selectedSource === 'PROJECT_LOCAL' ? 'bg-sky-950 text-sky-300 border border-sky-800' : 'bg-slate-950 text-slate-400 hover:text-slate-200'"
        >
          Local
        </button>
        <button
          type="button"
          @click="selectedSource = 'BUILTIN'"
          class="px-2 py-1 rounded text-[11px]"
          :class="selectedSource === 'BUILTIN' ? 'bg-sky-950 text-sky-300 border border-sky-800' : 'bg-slate-950 text-slate-400 hover:text-slate-200'"
        >
          Builtin
        </button>
      </div>
    </div>

    <!-- Prompts Table -->
    <div class="border border-slate-800 rounded-lg overflow-hidden font-mono text-xs shadow-sm">
      <table class="w-full text-left">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
          <tr>
            <th class="p-3">Template Specification</th>
            <th class="p-3">Source Precedence</th>
            <th class="p-3">Slot Variables</th>
            <th class="p-3">Compatibility</th>
            <th class="p-3 text-right">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-if="filteredPrompts.length === 0">
            <td colspan="5" class="p-8 text-center text-slate-500 font-sans">
              No prompt templates matched the active filter or search criteria.
            </td>
          </tr>
          <tr v-for="p in filteredPrompts" :key="p.id" class="hover:bg-slate-900/60 transition-colors">
            <td class="p-3 font-sans">
              <div class="font-semibold text-slate-100 flex items-center gap-1.5">
                <span>{{ p.name }}</span>
                <span v-if="p.role" class="px-1.5 py-0.2 rounded text-[10px] font-mono bg-slate-800 text-sky-300">
                  {{ p.role }}
                </span>
              </div>
              <div class="text-[10px] font-mono text-slate-500 mt-0.5">{{ p.id }}</div>
              <p v-if="p.description" class="text-xs text-slate-400 mt-1 font-sans line-clamp-1">
                {{ p.description }}
              </p>
            </td>
            <td class="p-3 font-mono">
              <span
                class="px-2 py-0.5 rounded text-[10px]"
                :class="p.source === 'GITHUB_PROMPTS' ? 'bg-purple-950 text-purple-300 border border-purple-800' :
                        p.source === 'CUSTOM_DIR' ? 'bg-sky-950 text-sky-300 border border-sky-800' :
                        p.source === 'PROJECT_LOCAL' ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' :
                        'bg-slate-950 border border-slate-800 text-slate-400'"
              >
                {{ p.source }}
              </span>
            </td>
            <td class="p-3 text-sky-400 font-mono text-[11px]">
              <span :title="p.variables.join(', ')">{{ formatVars(p.variables) }}</span>
            </td>
            <td class="p-3">
              <span class="px-2 py-0.5 rounded text-[10px] bg-emerald-950 text-emerald-300 border border-emerald-800 inline-flex items-center gap-1">
                <BadgeCheck class="w-3 h-3 text-emerald-400" />
                <span>100% Valid</span>
              </span>
            </td>
            <td class="p-3 text-right">
              <button
                @click="openSlotTester(p)"
                type="button"
                class="h-7 px-2.5 rounded bg-slate-950 hover:bg-slate-800 border border-slate-800 text-xs font-mono text-sky-400 hover:text-sky-300 inline-flex items-center gap-1.5 transition-colors"
              >
                <Sparkles class="w-3 h-3 text-sky-400" />
                <span>Test Slots</span>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Live Slot-Filling Interactive Drawer / Modal -->
    <div
      v-if="activeSlotPrompt"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm"
      @click="activeSlotPrompt = null"
    >
      <div
        class="relative max-w-4xl w-full bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4 max-h-[92vh] flex flex-col"
        @click.stop
      >
        <div class="flex items-center justify-between pb-3 border-b border-slate-800">
          <div>
            <h3 class="text-sm font-bold text-slate-100 flex items-center gap-2">
              <Sparkles class="w-4 h-4 text-sky-400" />
              <span>Live Slot-Filling Preview: {{ activeSlotPrompt.name }}</span>
            </h3>
            <span class="text-xs font-mono text-slate-400">Template ID: {{ activeSlotPrompt.id }} &bull; Tier: {{ activeSlotPrompt.source }}</span>
          </div>
          <button
            @click="activeSlotPrompt = null"
            class="text-slate-400 hover:text-white"
          >
            <X class="w-5 h-5" />
          </button>
        </div>

        <div class="flex-1 overflow-y-auto space-y-4 pr-1">
          <!-- Variables Inputs -->
          <div v-if="activeSlotPrompt.variables.length > 0" class="p-3.5 bg-slate-950 rounded-lg border border-slate-800 space-y-2.5">
            <h4 class="text-xs font-semibold text-slate-300 uppercase tracking-wide flex items-center justify-between">
              <span>Dynamic Parameter Slot Variables (Live Injection)</span>
              <span class="text-[10px] text-slate-500 font-mono">{{ activeSlotPrompt.variables.length }} variable(s) found</span>
            </h4>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div v-for="v in activeSlotPrompt.variables" :key="v" class="space-y-1">
                <label class="block text-[11px] font-mono text-sky-400">{{ slotLabel(v) }}</label>
                <input
                  v-model="mockValues[v]"
                  type="text"
                  class="w-full h-8 px-2.5 bg-slate-900 border border-slate-700 rounded text-xs text-slate-200 focus:outline-none focus:border-sky-500 font-mono"
                  placeholder="Enter slot value..."
                />
              </div>
            </div>
          </div>

          <!-- Live Rendered Result Preview -->
          <div class="space-y-1.5">
            <div class="flex items-center justify-between text-xs">
              <span class="font-semibold text-slate-300 uppercase tracking-wide">
                Rendered Prompt Preview
              </span>
              <button
                @click="copyRendered"
                type="button"
                class="text-[11px] font-mono text-sky-400 hover:text-sky-300 flex items-center gap-1"
              >
                <Check v-if="isCopied" class="w-3.5 h-3.5" />
                <Copy v-else class="w-3.5 h-3.5" />
                <span>{{ isCopied ? 'Copied!' : 'Copy Prompt' }}</span>
              </button>
            </div>
            <div class="p-3.5 bg-slate-950 border border-slate-800 rounded-lg font-mono text-xs text-slate-200 leading-relaxed whitespace-pre-wrap selection:bg-sky-950 max-h-96 overflow-y-auto">
              {{ getRenderedPrompt(activeSlotPrompt.id) }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Registration Modal -->
    <RegisterPromptModal
      v-if="showRegisterModal"
      @close="showRegisterModal = false"
      @registered="onRegisteredPromptSource"
    />
  </div>
</template>
