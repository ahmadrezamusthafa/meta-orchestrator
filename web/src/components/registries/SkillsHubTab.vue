<script setup lang="ts">
import { ref } from 'vue'
import { Wrench, Plus, GitBranch, RefreshCw } from 'lucide-vue-next'
import AddRemoteSkillModal from './AddRemoteSkillModal.vue'

const props = defineProps<{
  skills: Array<{
    id: string
    name: string
    source: string
    format: string
    description: string
    repo_url?: string
  }>
}>()

const emit = defineEmits<{
  (e: 'add-skill', skill: any): void
}>()

const showAddModal = ref(false)

function handleAddSkill(newSkill: any) {
  emit('add-skill', newSkill)
  showAddModal.value = false
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
          Discovered Skills (Remote Git, Local, System & Built-in)
        </h3>
        <span class="text-[11px] text-slate-400">
          Compatible across BMAD, Claude SKILL.md, Superpower, MCP, and OpenAI standards
        </span>
      </div>

      <div class="flex items-center gap-2">
        <button
          @click="showAddModal = true"
          type="button"
          class="h-7 px-2.5 rounded bg-purple-950 border border-purple-800 text-purple-300 hover:bg-purple-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>Add Remote Git Skill Repo</span>
        </button>

        <span class="text-xs font-mono text-emerald-400 px-2 py-0.5 rounded bg-slate-900 border border-slate-800">
          {{ skills.length }} Registered
        </span>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div
        v-for="s in skills"
        :key="s.id"
        class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-2 hover:border-slate-700 transition-colors shadow-sm"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <Wrench class="w-4 h-4 text-emerald-400" />
            <h4 class="text-xs font-bold text-slate-100">{{ s.name }}</h4>
          </div>
          <span
            class="px-2 py-0.5 rounded text-[10px] font-mono border"
            :class="{
              'bg-emerald-950 text-emerald-300 border-emerald-800': s.source === 'BUILTIN',
              'bg-sky-950 text-sky-300 border-sky-800': s.source === 'PROJECT_LOCAL',
              'bg-purple-950 text-purple-300 border-purple-800': s.source === 'REMOTE_GIT',
              'bg-slate-800 text-slate-300 border-slate-700': s.source === 'USER_SYSTEM',
            }"
          >
            {{ s.source }}
          </span>
        </div>

        <p class="text-xs text-slate-400 leading-relaxed">{{ s.description }}</p>

        <div class="pt-2 border-t border-slate-800/80 flex items-center justify-between text-[10px] font-mono text-slate-500">
          <span>Format: <strong class="text-slate-300">{{ s.format }}</strong></span>
          <span v-if="s.repo_url" class="text-purple-400 truncate max-w-xs">{{ s.repo_url }}</span>
        </div>
      </div>
    </div>

    <AddRemoteSkillModal
      v-if="showAddModal"
      @close="showAddModal = false"
      @add="handleAddSkill"
    />
  </div>
</template>
