<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../services/api'
import type { RegistryDTO } from '../types'
import SkillsHubTab from '../components/registries/SkillsHubTab.vue'
import PromptTemplatesTab from '../components/registries/PromptTemplatesTab.vue'
import LifecycleHooksTab from '../components/registries/LifecycleHooksTab.vue'
import { BookOpen, Wrench, MessageSquare, Anchor } from 'lucide-vue-next'

const activeTab = ref<'skills' | 'prompts' | 'hooks'>('skills')
const registries = ref<RegistryDTO>({ skills: [], prompts: [], hooks: [] })

onMounted(async () => {
  try {
    registries.value = await api.getRegistries()
  } catch (e) {
    console.error('Failed to load registries:', e)
  }
})

function handleAddSkill(newSkill: any) {
  registries.value.skills.unshift(newSkill)
}

function handleAddHook(newHook: any) {
  registries.value.hooks.unshift(newHook)
}
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
      <SkillsHubTab
        v-if="activeTab === 'skills'"
        :skills="registries.skills"
        @add-skill="handleAddSkill"
      />

      <PromptTemplatesTab
        v-else-if="activeTab === 'prompts'"
        :prompts="registries.prompts"
      />

      <LifecycleHooksTab
        v-else-if="activeTab === 'hooks'"
        :hooks="registries.hooks"
        @add-hook="handleAddHook"
      />
    </main>
  </div>
</template>
