<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useProjectStore } from '../stores/projects'
import { useToastStore } from '../stores/toast'
import type { ProjectRole, ProjectRepo } from '../types'
import {
  FolderGit2,
  FolderPlus,
  RefreshCw,
  Trash2,
  ExternalLink,
  Layers,
  Server,
  Layout,
  FlaskConical,
  FileCode2,
  FolderArchive,
  CheckCircle2,
  AlertTriangle,
  Search,
  Plus,
  Copy,
  Check,
  ArrowRight,
  ShieldCheck,
  X,
  Play
} from 'lucide-vue-next'
import { useRouter } from 'vue-router'

const router = useRouter()
const projectStore = useProjectStore()
const toast = useToastStore()

const isCreateModalOpen = ref(false)
const isScanning = ref(false)
const scanPath = ref('')
const scannedCandidates = ref<Array<{ name: string; path: string; role: ProjectRole; manifest: string; selected: boolean }>>([])

// Create form state
const formName = ref('')
const formId = ref('')
const formDesc = ref('')
const formRootDir = ref('')
const formActiveSDLC = ref('general-ai-sdlc')
const formRepos = ref<Array<{ name: string; path: string; role: ProjectRole; manifest: string }>>([
  { name: 'frontend-portal', path: '', role: 'frontend', manifest: 'package.json' },
  { name: 'backend-core', path: '', role: 'backend', manifest: 'go.mod' },
  { name: 'automation-test', path: '', role: 'automation-test', manifest: 'playwright.config.ts' },
  { name: 'api-contracts', path: '', role: 'contracts', manifest: 'openapi.yaml' }
])

const roleFilter = ref<string>('all')
const copiedPath = ref(false)

onMounted(async () => {
  await projectStore.fetchProjects()
})

const activeProject = computed(() => projectStore.activeProject)

const filteredRepos = computed(() => {
  if (!activeProject.value?.repos) return []
  if (roleFilter.value === 'all') return activeProject.value.repos
  return activeProject.value.repos.filter(r => r.role === roleFilter.value)
})

function onProjectNameInput() {
  formId.value = formName.value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  if (formId.value) {
    formRootDir.value = `workspaces/${formId.value}`
  }
}

function openCreateModal() {
  formName.value = ''
  formId.value = ''
  formDesc.value = ''
  formRootDir.value = ''
  formRepos.value = []
  scannedCandidates.value = []
  scanPath.value = ''
  isCreateModalOpen.value = true
}

async function handleScanDirectory() {
  isScanning.value = true
  try {
    const result = await projectStore.scanDirectory(scanPath.value)
    scannedCandidates.value = result.detected_repos.map(r => ({
      name: r.name,
      path: r.path,
      role: r.suggested_role,
      manifest: r.manifest_type,
      selected: true
    }))
    if (scannedCandidates.value.length === 0) {
      toast.info('No Repositories Found', 'Could not detect child codebases at the specified path.')
    } else {
      toast.success('Scan Completed', `Discovered ${scannedCandidates.value.length} potential repository services.`)
    }
  } catch (err: any) {
    // Toast handled in store
  } finally {
    isScanning.value = false
  }
}

function addScannedToForm() {
  const selected = scannedCandidates.value.filter(c => c.selected)
  for (const s of selected) {
    if (!formRepos.value.some(r => r.name === s.name)) {
      formRepos.value.push({
        name: s.name,
        path: s.path,
        role: s.role,
        manifest: s.manifest
      })
    }
  }
  scannedCandidates.value = []
  toast.info('Repositories Added', `Added ${selected.length} repositories to project form.`)
}

function addCustomRepoRow() {
  formRepos.value.push({
    name: `service-${formRepos.value.length + 1}`,
    path: '',
    role: 'backend',
    manifest: 'auto'
  })
}

function removeRepoRow(index: number) {
  formRepos.value.splice(index, 1)
}

async function handleCreateProject() {
  if (!formName.value.trim()) {
    toast.error('Validation Error', 'Project name is required')
    return
  }

  if (formRepos.value.length === 0) {
    toast.error('Validation Error', 'Please register at least one repository')
    return
  }

  const payload = {
    id: formId.value || undefined,
    name: formName.value,
    description: formDesc.value,
    root_dir: formRootDir.value || undefined,
    active_sdlc: formActiveSDLC.value,
    repos: formRepos.value.map(r => ({
      name: r.name,
      path: r.path,
      role: r.role,
      manifest_type: r.manifest === 'auto' ? '' : r.manifest
    })) as ProjectRepo[]
  }

  try {
    await projectStore.createProject(payload)
    isCreateModalOpen.value = false
  } catch (err: any) {
    // Handled by store
  }
}

async function handleResync() {
  if (!activeProject.value) return
  await projectStore.resyncProject(activeProject.value.id)
}

async function handleDelete(id: string) {
  if (confirm(`Are you sure you want to delete project ${id}?`)) {
    await projectStore.deleteProject(id)
  }
}

function copyRootDir() {
  if (!activeProject.value?.root_dir) return
  navigator.clipboard.writeText(activeProject.value.root_dir)
  copiedPath.value = true
  setTimeout(() => { copiedPath.value = false }, 2000)
  toast.info('Copied', 'Project root directory copied to clipboard')
}

function getRoleBadgeStyle(role: ProjectRole) {
  switch (role) {
    case 'frontend':
      return {
        bg: 'bg-sky-500/10 border-sky-500/30 text-sky-400',
        icon: Layout,
        label: 'Frontend UI'
      }
    case 'backend':
      return {
        bg: 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400',
        icon: Server,
        label: 'Backend Service'
      }
    case 'automation-test':
      return {
        bg: 'bg-purple-500/10 border-purple-500/30 text-purple-400',
        icon: FlaskConical,
        label: 'Automation Test'
      }
    case 'contracts':
      return {
        bg: 'bg-amber-500/10 border-amber-500/30 text-amber-400',
        icon: FileCode2,
        label: 'API Contracts'
      }
    case 'artifact':
      return {
        bg: 'bg-blue-500/10 border-blue-500/30 text-blue-400',
        icon: FolderArchive,
        label: 'Artifacts & PRD'
      }
    default:
      return {
        bg: 'bg-slate-500/10 border-slate-500/30 text-slate-400',
        icon: Layers,
        label: 'Other Service'
      }
  }
}

function launchTaskForProject() {
  router.push({ path: '/', query: { project: activeProject.value?.id } })
}
</script>

<template>
  <div class="h-full w-full flex flex-col bg-slate-950 text-slate-100 overflow-y-auto">
    <!-- View Header -->
    <header class="p-6 border-b border-slate-800 bg-slate-900/50 backdrop-blur flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <div class="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
            <FolderGit2 class="w-6 h-6" />
          </div>
          <div>
            <h1 class="text-xl font-bold tracking-tight text-white flex items-center gap-2">
              Projects & Multi-Repo Workspaces
              <span class="text-xs px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 font-mono">
                Auto-Symlink Ready
              </span>
            </h1>
            <p class="text-xs text-slate-400 mt-0.5">
              Register distributed multi-repos, tag functional roles (Frontend, Backend, Tests, Contracts), and synthesize unified project roots.
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <button
          v-if="activeProject"
          @click="handleResync"
          :disabled="projectStore.isLoading"
          class="px-3.5 py-2 rounded-lg border border-slate-700 bg-slate-800/80 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-2 transition"
        >
          <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': projectStore.isLoading }" />
          Resync Symlinks
        </button>

        <button
          @click="openCreateModal"
          class="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-xs font-semibold text-white flex items-center gap-2 shadow-lg shadow-emerald-900/30 transition"
        >
          <FolderPlus class="w-4 h-4" />
          Register New Project
        </button>
      </div>
    </header>

    <div class="p-6 space-y-6 max-w-7xl mx-auto w-full">
      <!-- Project Selection Ribbon -->
      <section class="space-y-3">
        <div class="flex items-center justify-between">
          <span class="text-xs font-bold uppercase tracking-wider text-slate-400 font-mono">Registered Projects ({{ projectStore.projects.length }})</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div
            v-for="p in projectStore.projects"
            :key="p.id"
            @click="projectStore.selectProject(p.id)"
            class="p-4 rounded-xl border transition-all cursor-pointer text-left relative overflow-hidden"
            :class="projectStore.activeProjectId === p.id 
              ? 'bg-slate-900/90 border-emerald-500/50 shadow-lg shadow-emerald-950/40 ring-1 ring-emerald-500/30' 
              : 'bg-slate-900/40 border-slate-800 hover:border-slate-700 hover:bg-slate-900/60'"
          >
            <div class="flex items-start justify-between gap-2">
              <div>
                <h3 class="font-bold text-sm text-white group-hover:text-emerald-400 transition">{{ p.name }}</h3>
                <span class="text-[10px] font-mono text-slate-400">{{ p.id }}</span>
              </div>
              <span
                class="px-2 py-0.5 rounded text-[10px] font-semibold uppercase tracking-wider font-mono border"
                :class="p.status === 'provisioned' ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-amber-500/10 text-amber-400 border-amber-500/20'"
              >
                {{ p.status }}
              </span>
            </div>

            <p class="text-xs text-slate-400 mt-2 line-clamp-2 leading-relaxed">
              {{ p.description || 'No project description provided.' }}
            </p>

            <div class="mt-4 pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs text-slate-400 font-mono">
              <span class="flex items-center gap-1.5">
                <Layers class="w-3.5 h-3.5 text-slate-400" />
                {{ p.repos?.length || 0 }} Repositories
              </span>
              <span class="text-[11px] text-slate-400 truncate max-w-[120px]">{{ p.active_sdlc }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Active Project Deep Dive -->
      <section v-if="activeProject" class="space-y-6">
        <!-- Project Banner -->
        <div class="p-5 rounded-2xl bg-gradient-to-r from-slate-900 via-slate-900/90 to-slate-950 border border-slate-800 space-y-4">
          <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div class="space-y-1">
              <div class="flex items-center gap-3">
                <h2 class="text-lg font-bold text-white">{{ activeProject.name }}</h2>
                <span class="px-2.5 py-0.5 rounded-full text-xs font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  {{ activeProject.active_sdlc }}
                </span>
              </div>
              <p class="text-xs text-slate-400 max-w-3xl">{{ activeProject.description }}</p>
            </div>

            <div class="flex items-center gap-2">
              <button
                @click="launchTaskForProject"
                class="px-3 py-1.5 rounded-lg bg-emerald-600/90 hover:bg-emerald-500 text-xs font-semibold text-white flex items-center gap-1.5 shadow transition"
              >
                <Play class="w-3.5 h-3.5 fill-current" />
                New Task in Project
              </button>
              <button
                @click="handleDelete(activeProject.id)"
                class="p-2 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 text-xs transition"
                title="Delete Project"
              >
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          </div>

          <!-- Root Dir Box -->
          <div class="p-3 rounded-xl bg-slate-950/70 border border-slate-800/80 flex items-center justify-between gap-4">
            <div class="flex items-center gap-2.5 min-w-0">
              <ShieldCheck class="w-4 h-4 text-emerald-400 flex-shrink-0" />
              <div class="text-xs truncate">
                <span class="text-slate-400 font-mono">Project Root (Symlink Mount Target): </span>
                <span class="text-slate-200 font-mono font-medium">{{ activeProject.root_dir }}</span>
              </div>
            </div>
            <button
              @click="copyRootDir"
              class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs flex items-center gap-1 flex-shrink-0 transition"
            >
              <Check v-if="copiedPath" class="w-3.5 h-3.5 text-emerald-400" />
              <Copy v-else class="w-3.5 h-3.5" />
              <span>{{ copiedPath ? 'Copied' : 'Copy Path' }}</span>
            </button>
          </div>
        </div>

        <!-- Repositories Grid & Filter -->
        <div class="space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <h3 class="text-sm font-bold text-white uppercase tracking-wider font-mono">
                Mapped Services & Multi-Repos ({{ filteredRepos.length }})
              </h3>
            </div>

            <!-- Role Filter Pills -->
            <div class="flex flex-wrap items-center gap-1.5 bg-slate-900 p-1 rounded-lg border border-slate-800 text-xs">
              <button
                @click="roleFilter = 'all'"
                class="px-2.5 py-1 rounded text-xs transition font-medium"
                :class="roleFilter === 'all' ? 'bg-slate-800 text-white shadow' : 'text-slate-400 hover:text-slate-200'"
              >
                All
              </button>
              <button
                @click="roleFilter = 'frontend'"
                class="px-2.5 py-1 rounded text-xs transition font-medium flex items-center gap-1"
                :class="roleFilter === 'frontend' ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30' : 'text-slate-400 hover:text-slate-200'"
              >
                <Layout class="w-3 h-3" /> Frontend
              </button>
              <button
                @click="roleFilter = 'backend'"
                class="px-2.5 py-1 rounded text-xs transition font-medium flex items-center gap-1"
                :class="roleFilter === 'backend' ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30' : 'text-slate-400 hover:text-slate-200'"
              >
                <Server class="w-3 h-3" /> Backend
              </button>
              <button
                @click="roleFilter = 'automation-test'"
                class="px-2.5 py-1 rounded text-xs transition font-medium flex items-center gap-1"
                :class="roleFilter === 'automation-test' ? 'bg-purple-500/20 text-purple-300 border border-purple-500/30' : 'text-slate-400 hover:text-slate-200'"
              >
                <FlaskConical class="w-3 h-3" /> Test Automation
              </button>
              <button
                @click="roleFilter = 'contracts'"
                class="px-2.5 py-1 rounded text-xs transition font-medium flex items-center gap-1"
                :class="roleFilter === 'contracts' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/30' : 'text-slate-400 hover:text-slate-200'"
              >
                <FileCode2 class="w-3 h-3" /> Contracts
              </button>
            </div>
          </div>

          <!-- Repos Table / Cards -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div
              v-for="repo in filteredRepos"
              :key="repo.id || repo.name"
              class="p-4 rounded-xl bg-slate-900/60 border border-slate-800 space-y-3 relative overflow-hidden group hover:border-slate-700 transition"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-2">
                  <div
                    class="p-1.5 rounded-lg border text-xs font-semibold flex items-center gap-1.5"
                    :class="getRoleBadgeStyle(repo.role).bg"
                  >
                    <component :is="getRoleBadgeStyle(repo.role).icon" class="w-3.5 h-3.5" />
                    <span>{{ getRoleBadgeStyle(repo.role).label }}</span>
                  </div>
                  <span class="font-bold text-sm text-white font-mono">{{ repo.name }}</span>
                </div>

                <div class="flex items-center gap-1.5 text-xs font-mono">
                  <span
                    class="flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold border"
                    :class="repo.status === 'linked' ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border-rose-500/20'"
                  >
                    <CheckCircle2 v-if="repo.status === 'linked'" class="w-3 h-3" />
                    <AlertTriangle v-else class="w-3 h-3" />
                    {{ repo.status === 'linked' ? 'Symlink Active' : (repo.status || 'Broken') }}
                  </span>
                </div>
              </div>

              <!-- Paths Details -->
              <div class="space-y-1.5 text-xs font-mono">
                <div class="flex items-center justify-between text-slate-400">
                  <span class="text-[11px] text-slate-400">Source Path:</span>
                  <span class="text-slate-300 truncate max-w-[320px]" :title="repo.path">{{ repo.path }}</span>
                </div>

                <div class="flex items-center justify-between text-slate-400">
                  <span class="text-[11px] text-slate-400">Project Symlink:</span>
                  <span class="text-emerald-400/90 truncate max-w-[320px]" :title="repo.symlink_path">{{ repo.symlink_path }}</span>
                </div>

                <div class="flex items-center justify-between text-slate-400 pt-1 border-t border-slate-800/60">
                  <span class="text-[11px] text-slate-400">Detected Manifest:</span>
                  <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-300 text-[10px] font-bold">{{ repo.manifest_type || 'unknown' }}</span>
                </div>
              </div>

              <div v-if="repo.error" class="p-2 rounded bg-rose-950/40 border border-rose-900/50 text-[11px] text-rose-300 font-mono">
                {{ repo.error }}
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <!-- Register Project Modal -->
    <div
      v-if="isCreateModalOpen"
      class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 overflow-y-auto"
    >
      <div class="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-4xl max-h-[90vh] flex flex-col shadow-2xl overflow-hidden my-8">
        <!-- Modal Header -->
        <div class="p-5 border-b border-slate-800 flex items-center justify-between bg-slate-900/90">
          <div class="flex items-center gap-2.5">
            <div class="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
              <FolderPlus class="w-5 h-5" />
            </div>
            <div>
              <h2 class="font-bold text-base text-white">Register New Multi-Repo Project</h2>
              <p class="text-xs text-slate-400">Configure project topology, auto-scan repositories, and create atomic symlinks</p>
            </div>
          </div>
          <button @click="isCreateModalOpen = false" class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition">
            <X class="w-5 h-5" />
          </button>
        </div>

        <!-- Modal Body -->
        <div class="p-6 space-y-6 overflow-y-auto flex-1 text-xs">
          <!-- Step 1: Project Metadata -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Project Name <span class="text-emerald-400">*</span></label>
              <input
                v-model="formName"
                @input="onProjectNameInput"
                type="text"
                placeholder="e.g. Fintech Payment Gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500 font-medium"
              />
            </div>

            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Project ID (Slug)</label>
              <input
                v-model="formId"
                type="text"
                placeholder="fintech-payment-gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-400 font-mono focus:outline-none"
              />
            </div>

            <div class="space-y-1.5 md:col-span-2">
              <label class="font-semibold text-slate-200">Description</label>
              <input
                v-model="formDesc"
                type="text"
                placeholder="Multi-repo microservice architecture connecting Vue frontend, Go API, and Playwright tests"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div class="space-y-1.5 md:col-span-2">
              <label class="font-semibold text-slate-200">Project Root Target Path (Where symlinks will be created)</label>
              <input
                v-model="formRootDir"
                type="text"
                placeholder="workspaces/fintech-payment-gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-emerald-400 font-mono focus:outline-none focus:border-emerald-500"
              />
            </div>
          </div>

          <!-- Step 2: Auto-Directory Scanner Section -->
          <div class="p-4 rounded-xl bg-slate-950 border border-slate-800/80 space-y-3">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Search class="w-4 h-4 text-emerald-400" />
                <h4 class="font-bold text-slate-200 uppercase tracking-wider text-[11px] font-mono">Quick Local Directory Auto-Scanner</h4>
              </div>
              <span class="text-[10px] text-slate-400">Detects manifests and auto-assigns roles</span>
            </div>

            <div class="flex gap-2">
              <input
                v-model="scanPath"
                type="text"
                placeholder="Paste parent folder path (leave empty for current workspace root)"
                class="flex-1 px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-slate-300 font-mono text-xs focus:outline-none focus:border-emerald-500"
              />
              <button
                @click="handleScanDirectory"
                :disabled="isScanning"
                class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isScanning }" />
                Scan Repos
              </button>
            </div>

            <!-- Scanned candidates -->
            <div v-if="scannedCandidates.length > 0" class="space-y-2 pt-2 border-t border-slate-800">
              <div class="text-[11px] font-mono text-emerald-400 flex items-center justify-between">
                <span>Discovered {{ scannedCandidates.length }} repositories:</span>
                <button @click="addScannedToForm" class="text-xs underline text-emerald-400 hover:text-emerald-300 font-bold">
                  + Add Selected Repositories
                </button>
              </div>

              <div class="space-y-1.5 max-h-40 overflow-y-auto pr-1">
                <div
                  v-for="(cand, idx) in scannedCandidates"
                  :key="idx"
                  class="flex items-center justify-between p-2 rounded-lg bg-slate-900/80 border border-slate-800 text-xs"
                >
                  <div class="flex items-center gap-2">
                    <input type="checkbox" v-model="cand.selected" class="rounded text-emerald-500 focus:ring-emerald-500" />
                    <span class="font-mono font-bold text-slate-200">{{ cand.name }}</span>
                    <span class="text-[10px] text-slate-400 font-mono">({{ cand.manifest }})</span>
                  </div>

                  <select
                    v-model="cand.role"
                    class="px-2 py-1 rounded bg-slate-950 border border-slate-700 text-slate-300 text-[11px] font-mono"
                  >
                    <option value="frontend">Frontend UI</option>
                    <option value="backend">Backend Service</option>
                    <option value="automation-test">Automation Test</option>
                    <option value="contracts">API Contracts</option>
                    <option value="artifact">Artifacts & PRD</option>
                    <option value="other">Other Service</option>
                  </select>
                </div>
              </div>
            </div>
          </div>

          <!-- Step 3: Repositories List to be Provisioned -->
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <h4 class="font-bold text-slate-200 uppercase tracking-wider text-[11px] font-mono">
                Repositories to Symlink ({{ formRepos.length }})
              </h4>
              <button
                @click="addCustomRepoRow"
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-emerald-400 text-xs font-semibold flex items-center gap-1 transition"
              >
                <Plus class="w-3.5 h-3.5" />
                Add Custom Repo
              </button>
            </div>

            <div class="space-y-2">
              <div
                v-for="(repo, idx) in formRepos"
                :key="idx"
                class="p-3 rounded-xl bg-slate-950 border border-slate-800/80 space-y-2"
              >
                <div class="grid grid-cols-1 md:grid-cols-12 gap-2 items-center">
                  <!-- Name -->
                  <div class="md:col-span-4">
                    <label class="text-[10px] text-slate-400 font-mono">Repo/Service Name</label>
                    <input
                      v-model="repo.name"
                      type="text"
                      placeholder="e.g. backend-auth"
                      class="w-full px-2.5 py-1.5 rounded bg-slate-900 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
                    />
                  </div>

                  <!-- Role Selector -->
                  <div class="md:col-span-3">
                    <label class="text-[10px] text-slate-400 font-mono">Role / Type</label>
                    <select
                      v-model="repo.role"
                      class="w-full px-2.5 py-1.5 rounded bg-slate-900 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
                    >
                      <option value="frontend">🎨 Frontend UI</option>
                      <option value="backend">⚙️ Backend Service</option>
                      <option value="automation-test">🧪 Automation Test</option>
                      <option value="contracts">📜 API Contracts</option>
                      <option value="artifact">📁 Artifacts & PRD</option>
                      <option value="other">🔌 Other Service</option>
                    </select>
                  </div>

                  <!-- Path -->
                  <div class="md:col-span-4">
                    <label class="text-[10px] text-slate-400 font-mono">Absolute Source Path</label>
                    <input
                      v-model="repo.path"
                      type="text"
                      placeholder="/Users/name/Projects/service-name"
                      class="w-full px-2.5 py-1.5 rounded bg-slate-900 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
                    />
                  </div>

                  <!-- Delete Row -->
                  <div class="md:col-span-1 flex justify-end pt-3">
                    <button
                      @click="removeRepoRow(idx)"
                      class="p-1.5 rounded text-slate-400 hover:text-rose-400 hover:bg-slate-900 transition"
                      title="Remove repository"
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Modal Footer -->
        <div class="p-4 border-t border-slate-800 bg-slate-900/90 flex items-center justify-between">
          <button
            @click="isCreateModalOpen = false"
            class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-semibold transition"
          >
            Cancel
          </button>

          <button
            @click="handleCreateProject"
            :disabled="projectStore.isLoading"
            class="px-5 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-xs font-semibold text-white flex items-center gap-2 shadow-lg shadow-emerald-950/50 transition"
          >
            <FolderPlus class="w-4 h-4" />
            Create Project & Synthesize Symlinks
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
