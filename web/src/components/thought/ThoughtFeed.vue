<script setup lang="ts">
import { computed } from 'vue'
import ThoughtBubble from './ThoughtBubble.vue'
import { BrainCircuit } from 'lucide-vue-next'

const props = defineProps<{
  thoughts: any[]
}>()

const displayThoughts = computed(() => {
  if (props.thoughts && props.thoughts.length > 0) {
    return props.thoughts
  }
  return [
    {
      profile: 'Lead Architect',
      model: 'claude-3-5-sonnet',
      thought: 'Inspecting cross-repository dependency graph. AST Sharder confirmed schema contracts between frontend-portal and backend-core.',
      tool_call: {
        name: 'read_ast_node',
        arguments: { repository: 'frontend-portal', file: 'src/api/client.ts', symbol: 'StripeClient' },
        output: { status: 'RESOLVED', lines: '42-89' }
      },
      timestamp: new Date(Date.now() - 15 * 60000).toISOString(),
    },
    {
      profile: 'QA ATDD Engineer',
      model: 'claude-3-5-sonnet',
      thought: 'Authoring Playwright end-to-end specifications grounded in PRD acceptance criteria. Application source tree is write-locked.',
      tool_call: {
        name: 'generate_playwright_specs',
        arguments: { spec_path: 'tests/e2e/payment_checkout_spec.ts', assertions_count: 8 },
        output: { status: 'GENERATED', exit_code: 1, signature: 'AWAITING_CODE_GEN' }
      },
      timestamp: new Date(Date.now() - 8 * 60000).toISOString(),
    },
    {
      profile: 'Fullstack Developer',
      model: 'claude-3-5-sonnet',
      thought: 'Red phase failure verified. Write-locks disengaged. Implementing Stripe webhook idempotency handler with Redis distributed locking.',
      timestamp: new Date(Date.now() - 2 * 60000).toISOString(),
    }
  ]
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <div class="h-9 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <BrainCircuit class="w-3.5 h-3.5 text-sky-400" />
        <span class="font-mono text-slate-300 font-medium">REASONING & DECISION STREAM</span>
      </div>
      <span class="text-[11px] font-mono text-slate-500">{{ displayThoughts.length }} events</span>
    </div>

    <div class="flex-1 p-3 overflow-y-auto space-y-3">
      <ThoughtBubble
        v-for="(th, idx) in displayThoughts"
        :key="idx"
        :thought="th"
      />
    </div>
  </div>
</template>
