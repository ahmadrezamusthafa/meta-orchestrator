<script setup lang="ts">
import { ref } from 'vue'
import { X, GitBranch, Check } from 'lucide-vue-next'
import BtnPrimary from '../common/BtnPrimary.vue'

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'add', skill: any): void
}>()

const repoUrl = ref('')
const branchTag = ref('main')
const format = ref<'CLAUDE_SKILL' | 'BMAD_PACK' | 'MCP_SERVER' | 'SUPERPOWER'>('CLAUDE_SKILL')
const skillName = ref('')
const isAdding = ref(false)

function submit() {
  if (!repoUrl.value.trim() || !skillName.value.trim()) return
  isAdding.value = true
  emit('add', {
    id: skillName.value.toLowerCase().replace(/\s+/g, '-'),
    name: skillName.value,
    source: 'REMOTE_GIT',
    format: format.value,
    description: `Imported remote skill from ${repoUrl.value} (branch: ${branchTag.value})`,
    repo_url: repoUrl.value,
  })
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
    <div class="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
      <div class="flex items-center justify-between pb-2 border-b border-slate-800">
        <div>
          <h3 class="text-sm font-bold text-slate-100">Connect Remote Skill Repository</h3>
          <span class="text-xs text-slate-400">Git URL or Anthropic Model Context Protocol (MCP)</span>
        </div>
        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-3 text-xs">
        <div>
          <label class="block text-slate-400 mb-1 font-medium">Skill Display Name *</label>
          <input
            v-model="skillName"
            type="text"
            placeholder="e.g. Playwright E2E Runner"
            class="w-full h-8 px-2.5 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none focus:border-emerald-500"
          />
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Skill Format Standard</label>
          <select
            v-model="format"
            class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none"
          >
            <option value="CLAUDE_SKILL">Claude Code / Antigravity SKILL.md</option>
            <option value="BMAD_PACK">BMAD Multi-Agent Skill Pack</option>
            <option value="MCP_SERVER">Anthropic MCP Server (stdio / SSE)</option>
            <option value="SUPERPOWER">Superpower Shell Tool</option>
          </select>
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Remote Git Repository URL / Endpoint *</label>
          <div class="relative">
            <GitBranch class="absolute left-2.5 top-2 w-4 h-4 text-slate-500" />
            <input
              v-model="repoUrl"
              type="text"
              placeholder="https://github.com/org/ai-skills.git"
              class="w-full h-8 pl-8 pr-2.5 bg-slate-950 border border-slate-700 rounded text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
            />
          </div>
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Branch / Commit Ref</label>
          <input
            v-model="branchTag"
            type="text"
            placeholder="main"
            class="w-full h-8 px-2.5 bg-slate-950 border border-slate-700 rounded text-slate-200 font-mono text-xs focus:outline-none"
          />
        </div>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200"
        >
          Cancel
        </button>

        <BtnPrimary
          :disabled="!skillName.trim() || !repoUrl.trim()"
          :loading="isAdding"
          @click="submit"
        >
          <Check class="w-3.5 h-3.5" />
          <span>Connect & Ingest</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
