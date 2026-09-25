<script setup lang="ts">
import { ref } from 'vue'
import { GitPullRequest, Plus } from 'lucide-vue-next'
import RuleBuilderModal from './RuleBuilderModal.vue'

const strategy = ref<'BEST_PRACTICE' | 'RULE_BASED'>('BEST_PRACTICE')
const showRuleModal = ref(false)

const customRules = ref([
  {
    id: 1,
    priority: 1,
    condition: 'Stage == "task_implementation" && Complexity == "HIGH"',
    method: 'Supervisor',
    provider: 'Anthropic Claude',
    model: 'claude-3-5-sonnet',
  },
  {
    id: 2,
    priority: 2,
    condition: 'Repo matches "frontend-.*" && Complexity == "LOW"',
    method: 'ReAct',
    provider: 'Google Antigravity',
    model: 'gemini-2.0-flash',
  },
])

function addRule(rule: any) {
  customRules.value.push({
    id: customRules.value.length + 1,
    priority: customRules.value.length + 1,
    ...rule,
  })
  showRuleModal.value = false
}
</script>

<template>
  <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-4">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Dual Router Strategy Configuration
        </h3>
        <span class="text-[11px] text-slate-400">Choose between automated heuristics or user-defined custom routing rules</span>
      </div>
      <GitPullRequest class="w-4 h-4 text-sky-400" />
    </div>

    <div class="grid grid-cols-2 gap-3">
      <label
        class="p-3 rounded-lg border cursor-pointer select-none transition-colors flex flex-col gap-1"
        :class="strategy === 'BEST_PRACTICE' ? 'bg-sky-950/40 border-sky-600 text-sky-200' : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
      >
        <div class="flex items-center gap-2 font-medium text-xs">
          <input type="radio" value="BEST_PRACTICE" v-model="strategy" class="text-sky-500" />
          <span>Best Practice Heuristics [BP]</span>
        </div>
        <span class="text-[11px] text-slate-500">Autonomous dynamic routing matching task complexity and SDLC stage</span>
      </label>

      <label
        class="p-3 rounded-lg border cursor-pointer select-none transition-colors flex flex-col gap-1"
        :class="strategy === 'RULE_BASED' ? 'bg-purple-950/40 border-purple-600 text-purple-200' : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
      >
        <div class="flex items-center gap-2 font-medium text-xs">
          <input type="radio" value="RULE_BASED" v-model="strategy" class="text-purple-500" />
          <span>Custom Rule-Based Router [RULE]</span>
        </div>
        <span class="text-[11px] text-slate-500">Deterministic rule evaluation based on regex, tags, and stage filters</span>
      </label>
    </div>

    <div v-if="strategy === 'RULE_BASED'" class="space-y-3 pt-2">
      <div class="flex items-center justify-between">
        <span class="text-xs font-semibold text-slate-300">Active Routing Rules (Evaluated in Order)</span>
        <button
          @click="showRuleModal = true"
          type="button"
          class="h-7 px-2.5 rounded bg-purple-950 border border-purple-800 text-purple-300 hover:bg-purple-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>Add Custom Rule</span>
        </button>
      </div>

      <div class="border border-slate-800 rounded-lg overflow-hidden font-mono text-xs">
        <table class="w-full text-left">
          <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
            <tr>
              <th class="p-2">#</th>
              <th class="p-2">Condition</th>
              <th class="p-2">Method</th>
              <th class="p-2">Model</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/80 text-slate-300">
            <tr v-for="r in customRules" :key="r.id" class="hover:bg-slate-900/40">
              <td class="p-2 text-slate-500">{{ r.priority }}</td>
              <td class="p-2 text-purple-300">{{ r.condition }}</td>
              <td class="p-2 text-emerald-400">{{ r.method }}</td>
              <td class="p-2 text-slate-400">{{ r.model }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <RuleBuilderModal
      v-if="showRuleModal"
      @close="showRuleModal = false"
      @save="addRule"
    />
  </div>
</template>
