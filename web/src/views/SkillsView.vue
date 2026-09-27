<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSkillsStore } from '../stores/skills'
import { useToastStore } from '../stores/toast'
import { api } from '../services/api'
import type { UniversalSkillDTO, PromptItemDTO, HookItemDTO, PromptSourceDTO } from '../types'
import SkillCard from '../components/skills/SkillCard.vue'
import CompatibilityModal from '../components/skills/CompatibilityModal.vue'
import RegisterSourceModal from '../components/skills/RegisterSourceModal.vue'
import RegisterPromptModal from '../components/registries/RegisterPromptModal.vue'
import PromptTemplatesTab from '../components/registries/PromptTemplatesTab.vue'
import LifecycleHooksTab from '../components/registries/LifecycleHooksTab.vue'
import AddRemoteSkillModal from '../components/registries/AddRemoteSkillModal.vue'
import {
  Search,
  X,
  Sparkles,
  RefreshCw,
  FolderPlus,
  Layers,
  CheckCircle2,
  AlertTriangle,
  Folder,
  Trash2,
  MessageSquare,
  Anchor,
  GitBranch,
  FolderGit2,
  Wrench,
  BookOpen,
} from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const skillsStore = useSkillsStore()
const toastStore = useToastStore()

type HubTab = 'skills' | 'prompts' | 'hooks' | 'sources'
const activeTab = ref<HubTab>('skills')

// Prompts & Hooks state
const prompts = ref<PromptItemDTO[]>([])
const promptSources = ref<PromptSourceDTO[]>([])
const hooks = ref<any[]>([])
const remoteGitRepos = ref<any[]>([

  {
    id: 'repo-anthropic-skills',
    name: 'Anthropic Official Skills Pack',
    url: 'https://github.com/anthropics/anthropic-quickstarts',
    branch: 'main',
    format: 'CLAUDE_SKILL',
    count: 4,
  },
  {
    id: 'repo-mcp-servers',
    name: 'Model Context Protocol Servers',
    url: 'https://github.com/modelcontextprotocol/servers',
    branch: 'main',
    format: 'MCP_SERVER',
    count: 6,
  },
])

// Skills filtering & search
const searchQuery = ref('')
const selectedFormat = ref('all')
const selectedStatus = ref<'all' | 'enabled' | 'disabled'>('all')
const selectedCompat = ref<'all' | 'compatible' | 'issues'>('all')

// Modals
const activeSkillModal = ref<UniversalSkillDTO | null>(null)
const showRegisterModal = ref(false)
const showRegisterPromptModal = ref(false)
const showAddRemoteModal = ref(false)

// Sync tab with URL query (?tab=prompts | hooks | sources | skills)
watch(
  () => route.query.tab,
  (newTab) => {
    if (newTab && ['skills', 'prompts', 'hooks', 'sources'].includes(newTab as string)) {
      activeTab.value = newTab as HubTab
    }
  },
  { immediate: true }
)

function setTab(tab: HubTab) {
  activeTab.value = tab
  router.replace({ query: { ...route.query, tab } })
}

onMounted(async () => {
  await Promise.all([
    skillsStore.fetchSkills(),
    skillsStore.fetchSources(),
    loadRegistriesData(),
    loadPromptSources(),
  ])
})

async function loadPromptSources() {
  try {
    promptSources.value = await api.getPromptSources()
  } catch (err) {
    console.error('Failed to load prompt sources:', err)
  }
}

async function loadRegistriesData() {
  try {
    const [regData, promptList] = await Promise.all([
      api.getRegistries(),
      api.getPrompts().catch(() => [])
    ])
    if (promptList && promptList.length > 0) {
      prompts.value = promptList
    } else if (regData.prompts) {
      prompts.value = regData.prompts
    }
    if (regData.hooks) hooks.value = regData.hooks
  } catch (err) {
    console.error('Failed to load prompts/hooks registries:', err)
  }
}

async function handleRemovePromptSource(sourceId: string, sourceName: string) {
  try {
    await api.removePromptSource(sourceId)
    toastStore.success('Source Removed', `${sourceName} unregistered`)
    await Promise.all([loadPromptSources(), loadRegistriesData()])
  } catch (err: any) {
    toastStore.error('Removal Failed', err.message || 'Could not unregister prompt source')
  }
}

const filteredSkills = computed(() => {
  return skillsStore.skills.filter((s) => {
    // Format filter
    if (selectedFormat.value !== 'all') {
      if (selectedFormat.value === 'custom') {
        if (s.source_type !== 'custom_dir') return false
      } else if (s.source_format !== selectedFormat.value) {
        return false
      }
    }

    // Status filter
    if (selectedStatus.value === 'enabled' && !s.enabled) return false
    if (selectedStatus.value === 'disabled' && s.enabled) return false

    // Compatibility filter
    if (selectedCompat.value === 'compatible' && s.compatibility?.status !== 'compatible') return false
    if (selectedCompat.value === 'issues' && s.compatibility?.status === 'compatible') return false

    // Search query
    if (searchQuery.value.trim()) {
      const q = searchQuery.value.toLowerCase()
      const matchName = s.name.toLowerCase().includes(q)
      const matchDesc = (s.description || '').toLowerCase().includes(q)
      const matchCmd = (s.command || '').toLowerCase().includes(q)
      const matchRoles = (s.required_roles || []).some((r) => r.toLowerCase().includes(q))
      if (!matchName && !matchDesc && !matchCmd && !matchRoles) return false
    }

    return true
  })
})

async function handleToggleSkill(skill: UniversalSkillDTO, enabled: boolean) {
  try {
    await skillsStore.toggleSkill(skill.name, enabled)
    toastStore.success(
      enabled ? 'Skill Enabled' : 'Skill Disabled',
      `${skill.name} is ${enabled ? 'now active for AI execution' : 'excluded from AI pipelines'}`
    )
  } catch (err: any) {
    toastStore.error('Update Failed', err.message || 'Could not update skill status')
  }
}

async function handleRemoveSource(sourceId: string, sourceName: string) {
  try {
    await skillsStore.removeSource(sourceId)
    toastStore.success('Source Removed', `${sourceName} unregistered`)
  } catch (err: any) {
    toastStore.error('Removal Failed', err.message || 'Could not unregister source')
  }
}

function handleAddRemoteRepo(newRepo: any) {
  remoteGitRepos.value.unshift({
    id: newRepo.id,
    name: newRepo.name,
    url: newRepo.repo_url,
    branch: 'main',
    format: newRepo.format,
    count: 1,
  })
  showAddRemoteModal.value = false
  toastStore.success('Remote Git Repo Connected', `${newRepo.name} registered as skill source`)
}

function handleAddHook(newHook: any) {
  hooks.value.unshift(newHook)
  toastStore.success('Lifecycle Hook Active', `${newHook.id} bound to stage events`)
}

async function handleRescan() {
  try {
    await skillsStore.rescan()
    await loadRegistriesData()
    toastStore.success('Skills & Registries Rescanned', `Loaded ${skillsStore.skills.length} skills across all sources`)
  } catch (err: any) {
    toastStore.error('Rescan Failed', err.message || 'Could not rescan skills')
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Top Master Header: Unified Skills & Registries Hub -->
    <header class="h-14 px-6 bg-slate-900/60 border-b border-slate-800 flex items-center justify-between gap-4 flex-shrink-0">
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 rounded-lg bg-emerald-600/20 border border-emerald-500/40 flex items-center justify-center text-emerald-400">
          <Sparkles class="w-4 h-4" />
        </div>
        <div>
          <h2 class="text-xs font-bold text-slate-100 uppercase tracking-wider flex items-center gap-2">
            <span>Skills & Registries Hub</span>
            <span class="px-2 py-0.5 rounded text-[10px] font-mono font-semibold bg-emerald-950/80 text-emerald-300 border border-emerald-800">
              Multi-Source Unified
            </span>
          </h2>
          <p class="text-[10px] text-slate-400 font-mono">
            Claude SKILL.md &bull; Dynamic MCP Connectors &bull; Parameterized Prompts &bull; Stage Hooks
          </p>
        </div>
      </div>

      <!-- Unified Hub Navigation Tabs -->
      <div class="flex items-center gap-1 bg-slate-950 p-1 rounded-xl border border-slate-800 text-xs">
        <button
          type="button"
          @click="setTab('skills')"
          class="h-8 px-3 rounded-lg font-medium transition-colors flex items-center gap-1.5"
          :class="activeTab === 'skills'
            ? 'bg-slate-800 text-emerald-400 font-semibold shadow-sm'
            : 'text-slate-400 hover:text-slate-200'"
        >
          <Wrench class="w-3.5 h-3.5" />
          <span>Modular Skills</span>
          <span class="px-1.5 py-0.2 rounded-full text-[10px] font-mono" :class="activeTab === 'skills' ? 'bg-emerald-950 text-emerald-300' : 'bg-slate-900 text-slate-500'">
            {{ skillsStore.skills.length }}
          </span>
        </button>

        <button
          type="button"
          @click="setTab('prompts')"
          class="h-8 px-3 rounded-lg font-medium transition-colors flex items-center gap-1.5"
          :class="activeTab === 'prompts'
            ? 'bg-slate-800 text-sky-400 font-semibold shadow-sm'
            : 'text-slate-400 hover:text-slate-200'"
        >
          <MessageSquare class="w-3.5 h-3.5" />
          <span>Prompt Templates</span>
          <span class="px-1.5 py-0.2 rounded-full text-[10px] font-mono" :class="activeTab === 'prompts' ? 'bg-sky-950 text-sky-300' : 'bg-slate-900 text-slate-500'">
            {{ prompts.length }}
          </span>
        </button>

        <button
          type="button"
          @click="setTab('hooks')"
          class="h-8 px-3 rounded-lg font-medium transition-colors flex items-center gap-1.5"
          :class="activeTab === 'hooks'
            ? 'bg-slate-800 text-purple-400 font-semibold shadow-sm'
            : 'text-slate-400 hover:text-slate-200'"
        >
          <Anchor class="w-3.5 h-3.5" />
          <span>Lifecycle Hooks</span>
          <span class="px-1.5 py-0.2 rounded-full text-[10px] font-mono" :class="activeTab === 'hooks' ? 'bg-purple-950 text-purple-300' : 'bg-slate-900 text-slate-500'">
            {{ hooks.length }}
          </span>
        </button>

        <button
          type="button"
          @click="setTab('sources')"
          class="h-8 px-3 rounded-lg font-medium transition-colors flex items-center gap-1.5"
          :class="activeTab === 'sources'
            ? 'bg-slate-800 text-amber-400 font-semibold shadow-sm'
            : 'text-slate-400 hover:text-slate-200'"
        >
          <FolderGit2 class="w-3.5 h-3.5" />
          <span>Sources & Git</span>
          <span class="px-1.5 py-0.2 rounded-full text-[10px] font-mono" :class="activeTab === 'sources' ? 'bg-amber-950 text-amber-300' : 'bg-slate-900 text-slate-500'">
            {{ skillsStore.sources.length + remoteGitRepos.length + promptSources.length }}
          </span>
        </button>

      </div>

      <!-- Quick Actions -->
      <div class="flex items-center gap-2">
        <button
          type="button"
          @click="handleRescan"
          :disabled="skillsStore.isRescanning"
          title="Rescan all directories and flush cache"
          class="h-9 w-9 flex items-center justify-center rounded-lg bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
        >
          <RefreshCw class="w-4 h-4" :class="{ 'animate-spin text-emerald-400': skillsStore.isRescanning }" />
        </button>

        <button
          type="button"
          @click="showRegisterModal = true"
          class="h-9 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5 transition-colors shadow-sm"
        >
          <FolderPlus class="w-4 h-4" />
          <span>Register Dir</span>
        </button>
      </div>
    </header>

    <!-- TAB 1: MODULAR SKILLS CATALOG -->
    <div v-if="activeTab === 'skills'" class="flex-1 flex flex-col overflow-hidden">
      <!-- Zero-Trust Metrics Bar -->
      <div class="px-6 py-3 bg-slate-900/40 border-b border-slate-800 flex-shrink-0">
        <div class="grid grid-cols-2 sm:grid-cols-5 gap-3">
          <div class="p-3 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-slate-800 flex items-center justify-center text-slate-300">
              <Layers class="w-4 h-4" />
            </div>
            <div>
              <div class="text-[11px] text-slate-400">Total Skills</div>
              <div class="text-base font-bold text-slate-100 font-mono">{{ skillsStore.skills.length }}</div>
            </div>
          </div>

          <div class="p-3 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-emerald-950/80 border border-emerald-800 flex items-center justify-center text-emerald-400">
              <CheckCircle2 class="w-4 h-4" />
            </div>
            <div>
              <div class="text-[11px] text-slate-400">Active Tools</div>
              <div class="text-base font-bold text-emerald-400 font-mono">
                {{ skillsStore.enabledCount }} <span class="text-[10px] text-slate-500">/ {{ skillsStore.skills.length }}</span>
              </div>
            </div>
          </div>

          <div class="p-3 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-amber-950/80 border border-amber-800 flex items-center justify-center text-amber-400">
              <Sparkles class="w-4 h-4" />
            </div>
            <div>
              <div class="text-[11px] text-slate-400">Claude Adapted</div>
              <div class="text-base font-bold text-amber-300 font-mono">{{ skillsStore.claudeCount }}</div>
            </div>
          </div>

          <div class="p-3 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-indigo-950/80 border border-indigo-800 flex items-center justify-center text-indigo-400">
              <Layers class="w-4 h-4" />
            </div>
            <div>
              <div class="text-[11px] text-slate-400">MCP Connectors</div>
              <div class="text-base font-bold text-indigo-300 font-mono">{{ skillsStore.mcpCount }}</div>
            </div>
          </div>

          <div class="p-3 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-emerald-950/80 border border-emerald-800 flex items-center justify-center text-emerald-400">
              <CheckCircle2 class="w-4 h-4" />
            </div>
            <div>
              <div class="text-[11px] text-slate-400">Zero-Trust Health</div>
              <div class="text-[11px] font-bold text-slate-200 font-mono">
                <span class="text-emerald-400">{{ skillsStore.compatibleCount }} OK</span>
                <span v-if="skillsStore.warningCount > 0" class="text-amber-400 ml-1">{{ skillsStore.warningCount }} Warn</span>
                <span v-if="skillsStore.incompatibleCount > 0" class="text-rose-400 ml-1">{{ skillsStore.incompatibleCount }} Bad</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Toolbar: Filters and Search -->
      <div class="px-6 py-2.5 bg-slate-900/30 border-b border-slate-800 flex flex-wrap items-center justify-between gap-3 flex-shrink-0">
        <div class="flex items-center gap-2.5 flex-wrap">
          <div class="relative w-60">
            <Search class="absolute left-3 top-2.5 w-3.5 h-3.5 text-slate-500" />
            <input
              v-model="searchQuery"
              type="text"
              placeholder="Search skills, roles, commands..."
              class="w-full h-8 pl-8 pr-7 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500 font-mono"
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

          <div class="flex items-center gap-1 bg-slate-950 p-1 rounded-lg border border-slate-800 text-xs">
            <button
              v-for="fmt in [
                { id: 'all', label: 'All' },
                { id: 'claude', label: 'Claude' },
                { id: 'mcp', label: 'MCP' },
                { id: 'native', label: 'Native' },
                { id: 'custom', label: 'Custom' }
              ]"
              :key="fmt.id"
              @click="selectedFormat = fmt.id"
              class="px-2 py-0.5 rounded text-[11px] transition-colors"
              :class="selectedFormat === fmt.id ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
            >
              {{ fmt.label }}
            </button>
          </div>

          <div class="flex items-center gap-1 bg-slate-950 p-1 rounded-lg border border-slate-800 text-xs">
            <button
              @click="selectedStatus = 'all'"
              class="px-2 py-0.5 rounded text-[11px] transition-colors"
              :class="selectedStatus === 'all' ? 'bg-slate-800 text-slate-100 font-semibold' : 'text-slate-400 hover:text-slate-200'"
            >
              All
            </button>
            <button
              @click="selectedStatus = 'enabled'"
              class="px-2 py-0.5 rounded text-[11px] transition-colors"
              :class="selectedStatus === 'enabled' ? 'bg-emerald-950/80 text-emerald-300 font-semibold border border-emerald-800' : 'text-slate-400 hover:text-slate-200'"
            >
              Active
            </button>
            <button
              @click="selectedStatus = 'disabled'"
              class="px-2 py-0.5 rounded text-[11px] transition-colors"
              :class="selectedStatus === 'disabled' ? 'bg-rose-950/80 text-rose-300 font-semibold border border-rose-800' : 'text-slate-400 hover:text-slate-200'"
            >
              Disabled
            </button>
          </div>
        </div>

        <div class="text-xs text-slate-500 font-mono">
          Showing {{ filteredSkills.length }} of {{ skillsStore.skills.length }} skills
        </div>
      </div>

      <!-- Skills Cards Grid -->
      <div class="flex-1 p-6 overflow-y-auto">
        <div v-if="skillsStore.isLoading" class="flex flex-col items-center justify-center py-20 text-slate-400">
          <RefreshCw class="w-8 h-8 text-emerald-400 animate-spin mb-3" />
          <span class="text-sm">Auditing and discovering modular skills...</span>
        </div>

        <div
          v-else-if="filteredSkills.length === 0"
          class="flex flex-col items-center justify-center py-20 text-center space-y-3"
        >
          <div class="w-12 h-12 rounded-2xl bg-slate-900 border border-slate-800 flex items-center justify-center text-slate-500">
            <Layers class="w-6 h-6" />
          </div>
          <div class="space-y-1">
            <h4 class="text-sm font-semibold text-slate-300">No matching skills found</h4>
            <p class="text-xs text-slate-500 max-w-sm">
              Adjust search keywords, clear filters, or register an external Claude skill directory.
            </p>
          </div>
          <button
            type="button"
            @click="showRegisterModal = true"
            class="px-4 py-2 rounded-xl bg-slate-900 border border-slate-800 hover:border-emerald-600 text-xs text-emerald-400 font-semibold transition-colors flex items-center gap-1.5"
          >
            <FolderPlus class="w-4 h-4" />
            <span>Register Skill Directory</span>
          </button>
        </div>

        <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <SkillCard
            v-for="s in filteredSkills"
            :key="s.name"
            :skill="s"
            @toggle="handleToggleSkill(s, $event)"
            @inspect-compatibility="activeSkillModal = s"
          />
        </div>
      </div>
    </div>

    <!-- TAB 2: PROMPT TEMPLATES -->
    <div v-else-if="activeTab === 'prompts'" class="flex-1 p-6 overflow-y-auto">
      <PromptTemplatesTab :prompts="prompts" @reload="loadRegistriesData(); loadPromptSources()" />
    </div>


    <!-- TAB 3: LIFECYCLE HOOKS -->
    <div v-else-if="activeTab === 'hooks'" class="flex-1 p-6 overflow-y-auto">
      <LifecycleHooksTab :hooks="hooks" @add-hook="handleAddHook" />
    </div>

    <!-- TAB 4: SOURCES & GIT REPOSITORIES -->
    <div v-else-if="activeTab === 'sources'" class="flex-1 p-6 overflow-y-auto space-y-6">
      <!-- Section 1: Registered Local & Claude Directories -->
      <div class="space-y-3">
        <div class="flex items-center justify-between pb-2 border-b border-slate-800">
          <div>
            <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide flex items-center gap-2">
              <Folder class="w-4 h-4 text-emerald-400" />
              <span>Registered Local & Claude Skill Directories</span>
            </h3>
            <span class="text-[11px] text-slate-400">
              Directories parsed for <span class="font-mono text-emerald-400">SKILL.md</span> bundles and MCP JSON manifests
            </span>
          </div>

          <button
            type="button"
            @click="showRegisterModal = true"
            class="h-7 px-2.5 rounded bg-emerald-950 border border-emerald-800 text-emerald-300 hover:bg-emerald-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
          >
            <FolderPlus class="w-3.5 h-3.5" />
            <span>Register Local Dir</span>
          </button>
        </div>

        <div v-if="skillsStore.sources.length === 0" class="p-6 rounded-xl bg-slate-900/60 border border-slate-800 text-center text-xs text-slate-400 space-y-2">
          <p>No custom local directories registered yet.</p>
          <p class="text-[11px] text-slate-500 font-mono">
            Default auto-discovery checks: <span class="text-slate-300">.claude/skills</span>, <span class="text-slate-300">~/.claude/skills</span>, <span class="text-slate-300">.sdlc/skills</span>
          </p>
        </div>

        <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div
            v-for="src in skillsStore.sources"
            :key="src.id"
            class="p-4 rounded-xl bg-slate-900/80 border border-slate-800 flex items-center justify-between gap-3 shadow-sm hover:border-slate-700 transition-colors"
          >
            <div class="min-w-0 space-y-1">
              <div class="flex items-center gap-2">
                <span class="font-bold text-slate-100 text-xs">{{ src.name }}</span>
                <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase bg-slate-800 text-slate-300">
                  {{ src.format }}
                </span>
              </div>
              <div class="text-xs text-slate-400 font-mono truncate" :title="src.path">
                {{ src.path }}
              </div>
              <div class="text-[11px] text-emerald-400 font-mono">
                {{ src.skill_count }} skill(s) adapted and verified
              </div>
            </div>

            <button
              type="button"
              @click="handleRemoveSource(src.id, src.name)"
              class="p-2 rounded-lg text-slate-500 hover:text-rose-400 hover:bg-slate-950 transition-colors"
              title="Unregister source"
            >
              <Trash2 class="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      <!-- Section 2: Remote Git Repositories -->
      <div class="space-y-3 pt-4">
        <div class="flex items-center justify-between pb-2 border-b border-slate-800">
          <div>
            <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide flex items-center gap-2">
              <GitBranch class="w-4 h-4 text-purple-400" />
              <span>Remote Git Skill Repositories</span>
            </h3>
            <span class="text-[11px] text-slate-400">
              Synchronize polyglot skill packs from public or private Git repositories
            </span>
          </div>

          <button
            type="button"
            @click="showAddRemoteModal = true"
            class="h-7 px-2.5 rounded bg-purple-950 border border-purple-800 text-purple-300 hover:bg-purple-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
          >
            <FolderPlus class="w-3.5 h-3.5" />
            <span>Connect Remote Git Repo</span>
          </button>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div
            v-for="repo in remoteGitRepos"
            :key="repo.id"
            class="p-4 rounded-xl bg-slate-900/80 border border-slate-800 flex items-center justify-between gap-3 shadow-sm hover:border-slate-700 transition-colors"
          >
            <div class="min-w-0 space-y-1">
              <div class="flex items-center gap-2">
                <span class="font-bold text-slate-100 text-xs">{{ repo.name }}</span>
                <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase bg-purple-950 text-purple-300 border border-purple-800">
                  {{ repo.format }}
                </span>
              </div>
              <div class="text-xs text-purple-400 font-mono truncate" :title="repo.url">
                {{ repo.url }}
              </div>
              <div class="text-[11px] text-slate-500 font-mono">
                Branch: <span class="text-slate-300">{{ repo.branch }}</span> &bull; {{ repo.count }} active tools
              </div>
            </div>

            <button
              type="button"
              @click="remoteGitRepos = remoteGitRepos.filter(r => r.id !== repo.id)"
              class="p-2 rounded-lg text-slate-500 hover:text-rose-400 hover:bg-slate-950 transition-colors"
              title="Disconnect repository"
            >
              <Trash2 class="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      <!-- Section 3: Registered Prompt Template Directories -->
      <div class="space-y-3 pt-4">
        <div class="flex items-center justify-between pb-2 border-b border-slate-800">
          <div>
            <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide flex items-center gap-2">
              <MessageSquare class="w-4 h-4 text-sky-400" />
              <span>Registered Prompt Template Directories</span>
            </h3>
            <span class="text-[11px] text-slate-400">
              Scanned for <span class="font-mono text-sky-400">*.prompt.md</span> and markdown prompt specifications
            </span>
          </div>

          <button
            type="button"
            @click="showRegisterPromptModal = true"
            class="h-7 px-2.5 rounded bg-sky-950 border border-sky-800 text-sky-300 hover:bg-sky-900 text-[11px] font-mono flex items-center gap-1.5 transition-colors"
          >
            <FolderPlus class="w-3.5 h-3.5" />
            <span>Register Prompt Dir</span>
          </button>
        </div>

        <div v-if="promptSources.length === 0" class="p-6 rounded-xl bg-slate-900/60 border border-slate-800 text-center text-xs text-slate-400 space-y-2">
          <p>No custom prompt directories registered yet.</p>
          <p class="text-[11px] text-slate-500 font-mono">
            Default auto-discovery checks: <span class="text-slate-300">.sdlc/prompts</span>, <span class="text-slate-300">.github/prompts</span>, <span class="text-slate-300">~/.config/meta-orchestrator/prompts</span>
          </p>
        </div>

        <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div
            v-for="src in promptSources"
            :key="src.id"
            class="p-4 rounded-xl bg-slate-900/80 border border-slate-800 flex items-center justify-between gap-3 shadow-sm hover:border-slate-700 transition-colors"
          >
            <div class="min-w-0 space-y-1">
              <div class="flex items-center gap-2">
                <span class="font-bold text-slate-100 text-xs">{{ src.name }}</span>
                <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase bg-sky-950 text-sky-300 border border-sky-800">
                  PROMPT DIRECTORY
                </span>
              </div>
              <div class="text-xs text-slate-400 font-mono truncate" :title="src.path">
                {{ src.path }}
              </div>
              <div class="text-[11px] text-sky-400 font-mono">
                {{ src.template_count }} prompt template(s) active & ready
              </div>
            </div>

            <button
              type="button"
              @click="handleRemovePromptSource(src.id, src.name)"
              class="p-2 rounded-lg text-slate-500 hover:text-rose-400 hover:bg-slate-950 transition-colors"
              title="Unregister prompt directory"
            >
              <Trash2 class="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Modals -->
    <CompatibilityModal
      v-if="activeSkillModal"
      :skill="activeSkillModal"
      @close="activeSkillModal = null"
      @toggle="handleToggleSkill(activeSkillModal, $event); activeSkillModal.enabled = $event"
    />

    <RegisterSourceModal
      v-if="showRegisterModal"
      @close="showRegisterModal = false"
      @registered="skillsStore.fetchSkills()"
    />

    <RegisterPromptModal
      v-if="showRegisterPromptModal"
      @close="showRegisterPromptModal = false"
      @registered="loadRegistriesData(); loadPromptSources()"
    />

    <AddRemoteSkillModal
      v-if="showAddRemoteModal"
      @close="showAddRemoteModal = false"
      @add="handleAddRemoteRepo"
    />
  </div>
</template>

