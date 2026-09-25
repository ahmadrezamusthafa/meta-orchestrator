<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../services/api'
import type { RegistryDTO } from '../types'
import { BookOpen, Wrench, MessageSquare, Anchor, GitBranch, ShieldCheck } from 'lucide-vue-next'

const activeTab = ref<'skills' | 'prompts' | 'hooks'>('skills')
const registries = ref<RegistryDTO>({ skills: [], prompts: [], hooks: [] })

function formatVars(vars: string[]): string {
  return vars.map(v => '{{' + v + '}}').join(', ')
}

onMounted(async () => {
  try {
    registries.value = await api.getRegistries()
  } catch (e) {
    console.error('Failed to load registries:', e)
  }
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Header -->
    <div class="h-14 px-6 bg-slate-900/50 border-b border-slate-800 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <BookOpen class="w-4 h-4 text-emerald-400" />
        <h2 class="text-xs font-semibold text-slate-100 uppercase tracking-wide">
          Multi-Source Registries & Lifecycle Hooks Hub
        </h2>
      </div>

      <!-- Tab Buttons -->
      <div class="flex items-center gap-1 bg-slate-900 p-1 rounded-lg border border-slate-800">
        <button
          @click="activeTab = 'skills'"
          class="h-7 px-3 rounded text-xs font-medium transition-colors flex items-center gap-1.5"
          :class="activeTab === 'skills' ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
        >
          <Wrench class="w-3 h-3" />
          <span>Skills Hub</span>
        </button>

        <button
          @click="activeTab = 'prompts'"
          class="h-7 px-3 rounded text-xs font-medium transition-colors flex items-center gap-1.5"
          :class="activeTab === 'prompts' ? 'bg-slate-800 text-sky-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
        >
          <MessageSquare class="w-3 h-3" />
          <span>Prompt Templates</span>
        </button>

        <button
          @click="activeTab = 'hooks'"
          class="h-7 px-3 rounded text-xs font-medium transition-colors flex items-center gap-1.5"
          :class="activeTab === 'hooks' ? 'bg-slate-800 text-purple-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
        >
          <Anchor class="w-3 h-3" />
          <span>Lifecycle Hooks</span>
        </button>
      </div>
    </div>

    <!-- Content -->
    <main class="flex-1 p-6 overflow-y-auto">
      <!-- 1. SKILLS HUB TAB -->
      <div v-if="activeTab === 'skills'" class="space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-slate-800">
          <div>
            <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
              Discovered Skills (Remote Git, Local, System & Built-in)
            </h3>
            <span class="text-[11px] text-slate-400">
              Compatible across BMAD, Claude SKILL.md, Superpower, MCP, and OpenAI standards
            </span>
          </div>
          <span class="text-xs font-mono text-emerald-400">{{ registries.skills.length }} Registered</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div
            v-for="s in registries.skills"
            :key="s.id"
            class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-2 hover:border-slate-700 transition-colors"
          >
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Wrench class="w-4 h-4 text-emerald-400" />
                <h4 class="text-xs font-bold text-slate-100">{{ s.name }}</h4>
              </div>
              <span class="px-2 py-0.5 rounded text-[10px] font-mono border"
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
      </div>

      <!-- 2. PROMPT TEMPLATES TAB -->
      <div v-else-if="activeTab === 'prompts'" class="space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-slate-800">
          <div>
            <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
              Cascading Parameterized Prompt Templates
            </h3>
            <span class="text-[11px] text-slate-400">
              Resolved in hierarchy: Project Local (.sdlc/prompts/) $\succ$ User System $\succ$ Built-in
            </span>
          </div>
        </div>

        <div class="border border-slate-800 rounded-lg overflow-hidden font-mono text-xs">
          <table class="w-full text-left">
            <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
              <tr>
                <th class="p-3">Template ID</th>
                <th class="p-3">Source Precedence</th>
                <th class="p-3">Slot Variables</th>
                <th class="p-3">Status</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/80 text-slate-300">
              <tr v-for="p in registries.prompts" :key="p.id" class="hover:bg-slate-900/40">
                <td class="p-3 font-sans font-medium text-slate-100">{{ p.name }} ({{ p.id }})</td>
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
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 3. LIFECYCLE HOOKS TAB -->
      <div v-else-if="activeTab === 'hooks'" class="space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-slate-800">
          <div>
            <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">
              Lifecycle Event Interceptor Hooks Engine
            </h3>
            <span class="text-[11px] text-slate-400">
              Hooks fire on pre-stage, on-gate, pre-commit, and failure boundaries with blocking policies
            </span>
          </div>
        </div>

        <div class="border border-slate-800 rounded-lg overflow-hidden font-mono text-xs">
          <table class="w-full text-left">
            <thead class="bg-slate-950 text-slate-400 border-b border-slate-800 text-[11px]">
              <tr>
                <th class="p-3">Hook ID</th>
                <th class="p-3">Trigger Event</th>
                <th class="p-3">Execution Engine</th>
                <th class="p-3">Command / URL</th>
                <th class="p-3">Policy</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/80 text-slate-300">
              <tr v-for="h in registries.hooks" :key="h.id" class="hover:bg-slate-900/40">
                <td class="p-3 font-semibold text-slate-200">{{ h.id }}</td>
                <td class="p-3 text-amber-400">{{ h.event }}</td>
                <td class="p-3 text-sky-400">{{ h.type }}</td>
                <td class="p-3 text-slate-300 truncate max-w-xs">{{ h.command }}</td>
                <td class="p-3">
                  <span
                    class="px-2 py-0.5 rounded text-[10px]"
                    :class="h.policy === 'BLOCK' ? 'bg-rose-950 text-rose-300 border border-rose-800' : 'bg-amber-950 text-amber-300 border border-amber-800'"
                  >
                    {{ h.policy }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </main>
  </div>
</template>
