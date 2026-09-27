<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useSkillsStore } from '../stores/skills'
import { useToastStore } from '../stores/toast'
import type { UniversalSkillDTO } from '../types'
import SkillCard from '../components/skills/SkillCard.vue'
import CompatibilityModal from '../components/skills/CompatibilityModal.vue'
import RegisterSourceModal from '../components/skills/RegisterSourceModal.vue'
import {
  Search,
  X,
  Sparkles,
  RefreshCw,
  FolderPlus,
  Layers,
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Folder,
  Trash2,
  SlidersHorizontal,
} from 'lucide-vue-next'

const skillsStore = useSkillsStore()
const toastStore = useToastStore()

const searchQuery = ref('')
const selectedFormat = ref('all')
const selectedStatus = ref<'all' | 'enabled' | 'disabled'>('all')
const selectedCompat = ref<'all' | 'compatible' | 'issues'>('all')

const activeSkillModal = ref<UniversalSkillDTO | null>(null)
const showRegisterModal = ref(false)
const showSourcesDrawer = ref(false)

onMounted(async () => {
  await Promise.all([
    skillsStore.fetchSkills(),
    skillsStore.fetchSources(),
  ])
})

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
      `${skill.name} is ${enabled ? 'now active for AI process execution' : 'excluded from AI pipelines'}`
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

async function handleRescan() {
  try {
    await skillsStore.rescan()
    toastStore.success('Skills Rescanned', `Loaded ${skillsStore.skills.length} skills across all sources`)
  } catch (err: any) {
    toastStore.error('Rescan Failed', err.message || 'Could not rescan skills')
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Top Stats Banner -->
    <div class="px-6 py-4 bg-slate-900/60 border-b border-slate-800">
      <div class="grid grid-cols-2 sm:grid-cols-5 gap-3">
        <!-- Metric 1: Total Skills -->
        <div class="p-3.5 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-3">
          <div class="w-9 h-9 rounded-lg bg-slate-800 flex items-center justify-center text-slate-300">
            <Layers class="w-4 h-4" />
          </div>
          <div>
            <div class="text-xs text-slate-400 font-medium">Total Skills</div>
            <div class="text-lg font-bold text-slate-100 font-mono">{{ skillsStore.skills.length }}</div>
          </div>
        </div>

        <!-- Metric 2: Active & Enabled -->
        <div class="p-3.5 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-3">
          <div class="w-9 h-9 rounded-lg bg-emerald-950/80 border border-emerald-800 flex items-center justify-center text-emerald-400">
            <CheckCircle2 class="w-4 h-4" />
          </div>
          <div>
            <div class="text-xs text-slate-400 font-medium">Active Tools</div>
            <div class="text-lg font-bold text-emerald-400 font-mono">
              {{ skillsStore.enabledCount }} <span class="text-xs font-normal text-slate-500">/ {{ skillsStore.skills.length }}</span>
            </div>
          </div>
        </div>

        <!-- Metric 3: Claude Skills -->
        <div class="p-3.5 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-3">
          <div class="w-9 h-9 rounded-lg bg-amber-950/80 border border-amber-800 flex items-center justify-center text-amber-400">
            <Sparkles class="w-4 h-4" />
          </div>
          <div>
            <div class="text-xs text-slate-400 font-medium">Claude Adapted</div>
            <div class="text-lg font-bold text-amber-300 font-mono">{{ skillsStore.claudeCount }}</div>
          </div>
        </div>

        <!-- Metric 4: MCP Connectors -->
        <div class="p-3.5 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-3">
          <div class="w-9 h-9 rounded-lg bg-indigo-950/80 border border-indigo-800 flex items-center justify-center text-indigo-400">
            <Layers class="w-4 h-4" />
          </div>
          <div>
            <div class="text-xs text-slate-400 font-medium">MCP Connectors</div>
            <div class="text-lg font-bold text-indigo-300 font-mono">{{ skillsStore.mcpCount }}</div>
          </div>
        </div>

        <!-- Metric 5: Zero-Trust Health -->
        <div class="p-3.5 rounded-xl bg-slate-950/60 border border-slate-800 flex items-center gap-3">
          <div class="w-9 h-9 rounded-lg bg-emerald-950/80 border border-emerald-800 flex items-center justify-center text-emerald-400">
            <CheckCircle2 class="w-4 h-4" />
          </div>
          <div>
            <div class="text-xs text-slate-400 font-medium">Compatibility</div>
            <div class="text-xs font-bold text-slate-200 font-mono mt-0.5">
              <span class="text-emerald-400">{{ skillsStore.compatibleCount }} OK</span>
              <span v-if="skillsStore.warningCount > 0" class="text-amber-400 ml-1.5">{{ skillsStore.warningCount }} Warn</span>
              <span v-if="skillsStore.incompatibleCount > 0" class="text-rose-400 ml-1.5">{{ skillsStore.incompatibleCount }} Bad</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Toolbar: Search, Filters, and Actions -->
    <div class="px-6 py-3 bg-slate-900/40 border-b border-slate-800 flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-3 flex-wrap">
        <!-- Search Input -->
        <div class="relative w-64">
          <Search class="absolute left-3 top-2.5 w-4 h-4 text-slate-500" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search skills, roles, commands..."
            class="w-full h-9 pl-9 pr-8 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
          />
          <button
            v-if="searchQuery"
            @click="searchQuery = ''"
            type="button"
            class="absolute right-2.5 top-2.5 text-slate-500 hover:text-slate-300"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <!-- Format Filter Tabs -->
        <div class="flex items-center gap-1 bg-slate-950/60 p-1 rounded-lg border border-slate-800 text-xs">
          <button
            v-for="fmt in [
              { id: 'all', label: 'All' },
              { id: 'claude', label: 'Claude' },
              { id: 'mcp', label: 'MCP' },
              { id: 'native', label: 'Native' },
              { id: 'custom', label: 'Custom Dirs' }
            ]"
            :key="fmt.id"
            @click="selectedFormat = fmt.id"
            class="px-2.5 py-1 rounded-md transition-colors"
            :class="selectedFormat === fmt.id ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
          >
            {{ fmt.label }}
          </button>
        </div>

        <!-- Status Filter Tabs -->
        <div class="flex items-center gap-1 bg-slate-950/60 p-1 rounded-lg border border-slate-800 text-xs">
          <button
            @click="selectedStatus = 'all'"
            class="px-2 py-1 rounded-md transition-colors"
            :class="selectedStatus === 'all' ? 'bg-slate-800 text-slate-100 font-semibold' : 'text-slate-400 hover:text-slate-200'"
          >
            All
          </button>
          <button
            @click="selectedStatus = 'enabled'"
            class="px-2 py-1 rounded-md transition-colors"
            :class="selectedStatus === 'enabled' ? 'bg-emerald-950/80 text-emerald-300 font-semibold border border-emerald-800' : 'text-slate-400 hover:text-slate-200'"
          >
            Enabled
          </button>
          <button
            @click="selectedStatus = 'disabled'"
            class="px-2 py-1 rounded-md transition-colors"
            :class="selectedStatus === 'disabled' ? 'bg-rose-950/80 text-rose-300 font-semibold border border-rose-800' : 'text-slate-400 hover:text-slate-200'"
          >
            Disabled
          </button>
        </div>
      </div>

      <!-- Action Buttons -->
      <div class="flex items-center gap-2">
        <button
          type="button"
          @click="showSourcesDrawer = !showSourcesDrawer"
          class="h-9 px-3 rounded-lg border text-xs font-medium flex items-center gap-1.5 transition-colors"
          :class="showSourcesDrawer ? 'bg-slate-800 border-slate-700 text-slate-200' : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
        >
          <Folder class="w-4 h-4" />
          <span>Directories ({{ skillsStore.sources.length }})</span>
        </button>

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
          class="h-9 px-3.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5 transition-colors shadow-sm"
        >
          <FolderPlus class="w-4 h-4" />
          <span>Register Skill Dir</span>
        </button>
      </div>
    </div>

    <!-- Registered Sources Drawer / Bar (Collapsible) -->
    <div
      v-if="showSourcesDrawer"
      class="px-6 py-3 bg-slate-900 border-b border-slate-800 animate-in slide-in-from-top-2 duration-200 space-y-2"
    >
      <div class="flex items-center justify-between text-xs">
        <span class="font-semibold text-slate-300 uppercase tracking-wider">
          Registered External Skill Sources
        </span>
        <button
          @click="showSourcesDrawer = false"
          class="text-slate-500 hover:text-slate-300"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <div v-if="skillsStore.sources.length === 0" class="text-xs text-slate-500 py-2">
        No external directories registered yet. Click <span class="text-emerald-400 font-semibold cursor-pointer" @click="showRegisterModal = true">Register Skill Dir</span> to adapt Claude skills from <span class="font-mono">~/.claude/skills</span> or custom repositories.
      </div>

      <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2.5 pt-1">
        <div
          v-for="src in skillsStore.sources"
          :key="src.id"
          class="p-2.5 rounded-xl bg-slate-950 border border-slate-800 flex items-center justify-between gap-3 text-xs"
        >
          <div class="min-w-0">
            <div class="font-semibold text-slate-200 flex items-center gap-1.5">
              <span>{{ src.name }}</span>
              <span class="px-1.5 py-0.2 rounded text-[10px] font-mono uppercase bg-slate-800 text-slate-400">
                {{ src.format }}
              </span>
            </div>
            <div class="text-[11px] text-slate-500 font-mono truncate" :title="src.path">
              {{ src.path }}
            </div>
            <div class="text-[10px] text-emerald-400 mt-0.5">
              {{ src.skill_count }} skill(s) adapted
            </div>
          </div>

          <button
            type="button"
            @click="handleRemoveSource(src.id, src.name)"
            class="p-1.5 rounded-lg text-slate-500 hover:text-rose-400 hover:bg-slate-900 transition-colors"
            title="Unregister source"
          >
            <Trash2 class="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>

    <!-- Main Content: Modular Skills Grid -->
    <main class="flex-1 p-6 overflow-y-auto">
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
            Try adjusting your search query, clearing filters, or register an external Claude skill directory.
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
    </main>

    <!-- Compatibility Audit Report Modal -->
    <CompatibilityModal
      v-if="activeSkillModal"
      :skill="activeSkillModal"
      @close="activeSkillModal = null"
      @toggle="handleToggleSkill(activeSkillModal, $event); activeSkillModal.enabled = $event"
    />

    <!-- Register Directory Source Modal -->
    <RegisterSourceModal
      v-if="showRegisterModal"
      @close="showRegisterModal = false"
      @registered="skillsStore.fetchSkills()"
    />
  </div>
</template>
