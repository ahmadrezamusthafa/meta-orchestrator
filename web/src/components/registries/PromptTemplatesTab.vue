<script setup lang="ts">
import { ref } from 'vue'
import { MessageSquare, Sparkles, X, Copy, Check, Eye } from 'lucide-vue-next'
import { useToastStore } from '../../stores/toast'

const props = defineProps<{
  prompts: Array<{
    id: string
    name: string
    source: string
    variables: string[]
    system_override: boolean
  }>
}>()

const toastStore = useToastStore()

const activeSlotPrompt = ref<any | null>(null)
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

function openSlotTester(p: any) {
  activeSlotPrompt.value = p
  mockValues.value = {}
  p.variables.forEach((v: string) => {
    if (v.includes('architecture')) {
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
  return sampleTemplatesContent[id] || `You are an autonomous Meta-Orchestrator agent.\nExecuting task for {{feature_name}}.\nContext: {{system_architecture}}.\nTrace: {{failing_test_traces}}.`
}

function getRenderedPrompt(id: string): string {
  let content = getRawTemplate(id)
  for (const [key, val] of Object.entries(mockValues.value)) {
    content = content.split(`{{${key}}}`).join(val || `[MISSING: ${key}]`)
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
  return vars.map(v => '{{' + v + '}}').join(', ')
}
</script>

<template>
  <div class="space-y-4 select-none">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Cascading Parameterized Prompt Templates
        </h3>
        <span class="text-[11px] text-slate-400">
          Resolved in hierarchy: Project Local (.sdlc/prompts/) &succ; User System &succ; Built-in
        </span>
      </div>
      <span class="text-xs font-mono text-sky-400 px-2 py-0.5 rounded bg-slate-900 border border-slate-800">
        {{ prompts.length }} Templates
      </span>
    </div>

    <div class="border border-slate-800 rounded-lg overflow-hidden font-mono text-xs shadow-sm">
      <table class="w-full text-left">
        <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
          <tr>
            <th class="p-3">Template ID</th>
            <th class="p-3">Source Precedence</th>
            <th class="p-3">Slot Variables</th>
            <th class="p-3">Status</th>
            <th class="p-3 text-right">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/80 text-slate-300">
          <tr v-for="p in prompts" :key="p.id" class="hover:bg-slate-900/60 transition-colors">
            <td class="p-3 font-sans font-medium text-slate-100">
              <div class="font-semibold">{{ p.name }}</div>
              <div class="text-[10px] font-mono text-slate-500">{{ p.id }}</div>
            </td>
            <td class="p-3">
              <span class="px-2 py-0.5 rounded text-[10px] bg-slate-950 border border-slate-800 text-slate-400">
                {{ p.source }}
              </span>
            </td>
            <td class="p-3 text-sky-400">{{ formatVars(p.variables) }}</td>
            <td class="p-3">
              <span
                class="px-2 py-0.5 rounded text-[10px]"
                :class="p.system_override ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' : 'bg-slate-800 text-slate-400'"
              >
                {{ p.system_override ? 'Custom Override' : 'System Default' }}
              </span>
            </td>
            <td class="p-3 text-right">
              <button
                @click="openSlotTester(p)"
                type="button"
                class="h-7 px-2.5 rounded bg-slate-950 hover:bg-slate-800 border border-slate-800 text-xs font-mono text-emerald-400 inline-flex items-center gap-1.5 transition-colors"
              >
                <Sparkles class="w-3 h-3 text-emerald-400" />
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
        class="relative max-w-3xl w-full bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4 max-h-[90vh] flex flex-col"
        @click.stop
      >
        <div class="flex items-center justify-between pb-3 border-b border-slate-800">
          <div>
            <h3 class="text-sm font-bold text-slate-100 flex items-center gap-2">
              <Sparkles class="w-4 h-4 text-emerald-400" />
              <span>Live Slot-Filling Preview: {{ activeSlotPrompt.name }}</span>
            </h3>
            <span class="text-xs font-mono text-slate-400">Template ID: {{ activeSlotPrompt.id }}</span>
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
          <div class="p-3.5 bg-slate-950 rounded-lg border border-slate-800 space-y-2.5">
            <h4 class="text-xs font-semibold text-slate-300 uppercase tracking-wide">
              Semantic Slot Variables (Real-Time Injection)
            </h4>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div v-for="v in activeSlotPrompt.variables" :key="v" class="space-y-1">
                <label class="block text-[11px] font-mono text-sky-400">{{ slotLabel(v) }}</label>
                <input
                  v-model="mockValues[v]"
                  type="text"
                  class="w-full h-8 px-2.5 bg-slate-900 border border-slate-700 rounded text-xs text-slate-200 focus:outline-none focus:border-emerald-500 font-mono"
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
                class="text-[11px] font-mono text-emerald-400 hover:text-emerald-300 flex items-center gap-1"
              >
                <Check v-if="isCopied" class="w-3.5 h-3.5" />
                <Copy v-else class="w-3.5 h-3.5" />
                <span>{{ isCopied ? 'Copied!' : 'Copy Prompt' }}</span>
              </button>
            </div>
            <div class="p-3.5 bg-slate-950 border border-slate-800 rounded-lg font-mono text-xs text-slate-200 leading-relaxed whitespace-pre-wrap selection:bg-emerald-950">
              {{ getRenderedPrompt(activeSlotPrompt.id) }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
