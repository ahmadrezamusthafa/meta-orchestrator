<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useProjectStore } from '../stores/projects'
import { useToastStore } from '../stores/toast'
import type { ProjectRole, ProjectRepo, Project } from '../types'
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
  Play,
  Edit3,
  Sliders,
  GitBranch,
  FileText,
  Network,
  Eye,
  Terminal,
  Grid,
  Info,
  FolderOpen,
  HelpCircle,
  ChevronDown,
  ChevronUp
} from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import DirectoryPickerModal from '../components/common/DirectoryPickerModal.vue'
import ConfirmDeleteModal from '../components/common/ConfirmDeleteModal.vue'
import type { DirectoryItem } from '../types'

const router = useRouter()
const projectStore = useProjectStore()
const toast = useToastStore()

// Modal and Drawer States
const isCreateModalOpen = ref(false)
const isEditModalOpen = ref(false)
const isQuickAddModalOpen = ref(false)
const isDetailDrawerOpen = ref(false)
const selectedRepoDetail = ref<ProjectRepo | null>(null)

// Confirmation Dialog State
const isConfirmDeleteOpen = ref(false)
const confirmDeleteTitle = ref('Confirm Deletion')
const confirmDeleteMessage = ref('')
const confirmDeleteItemName = ref('')
const confirmDeleteNote = ref('Your physical host source code remains completely safe on disk.')
const confirmDeleteBtnText = ref('Delete')
const isConfirmDeleteLoading = ref(false)
let onConfirmDeleteCallback: (() => Promise<void>) | null = null

function triggerConfirmDelete(options: {
  title: string
  message: string
  itemName: string
  note?: string
  confirmText?: string
  onConfirm: () => Promise<void>
}) {
  confirmDeleteTitle.value = options.title
  confirmDeleteMessage.value = options.message
  confirmDeleteItemName.value = options.itemName
  confirmDeleteNote.value = options.note || 'Your physical host source code remains completely safe on disk.'
  confirmDeleteBtnText.value = options.confirmText || 'Delete'
  onConfirmDeleteCallback = options.onConfirm
  isConfirmDeleteOpen.value = true
}

async function handleExecuteConfirmDelete() {
  if (!onConfirmDeleteCallback) return
  isConfirmDeleteLoading.value = true
  try {
    await onConfirmDeleteCallback()
    isConfirmDeleteOpen.value = false
  } catch (err: any) {
    // Handled by store
  } finally {
    isConfirmDeleteLoading.value = false
  }
}

// View Toggle: "grid" vs "topology"
const viewMode = ref<'grid' | 'topology'>('grid')

// Search & Filtering
const searchQuery = ref('')
const roleFilter = ref<string>('all')
const statusFilter = ref<string>('all')
const copiedPath = ref(false)
const copiedCliCmd = ref(false)

// Local Scanner State
const isScanning = ref(false)
const scanPath = ref('')
const scannedCandidates = ref<Array<{ name: string; path: string; role: ProjectRole; manifest: string; selected: boolean }>>([])

// Create Form State
const createFormName = ref('')
const createFormId = ref('')
const createFormDesc = ref('')
const createFormRootDir = ref('')
const createFormActiveSDLC = ref('general-ai-sdlc')
const createFormRepos = ref<Array<{ name: string; path: string; role: ProjectRole; manifest: string }>>([])

// Edit Form State
const editFormId = ref('')
const editFormName = ref('')
const editFormDesc = ref('')
const editFormRootDir = ref('')
const editFormActiveSDLC = ref('general-ai-sdlc')
const editFormRepos = ref<Array<{ id?: string; name: string; path: string; role: ProjectRole; manifest: string }>>([])

// Quick Add Repo State
const quickAddName = ref('')
const quickAddPath = ref('')
const quickAddRole = ref<ProjectRole>('backend')

// Architecture Explainer Card State
const showArchitectureExplainer = ref(true)

// Directory Picker State & Helpers
const isDirectoryPickerOpen = ref(false)
const pickerTitle = ref('Select Directory')
const pickerHelperText = ref('Navigate your filesystem and choose a directory')
const pickerInitialPath = ref('')
const pickerTarget = ref<
  | 'create_root'
  | 'create_scan'
  | 'create_repo'
  | 'create_browse_add'
  | 'edit_root'
  | 'edit_scan'
  | 'edit_repo'
  | 'edit_browse_add'
  | 'quick_add'
  | null
>(null)
const pickerRepoIndex = ref<number | null>(null)
const pickerInitialCreateFolder = ref(false)

function openDirectoryPicker(
  target:
    | 'create_root'
    | 'create_scan'
    | 'create_repo'
    | 'create_browse_add'
    | 'edit_root'
    | 'edit_scan'
    | 'edit_repo'
    | 'edit_browse_add'
    | 'quick_add',
  repoIndex: number | null = null,
  initialPath: string = '',
  title: string = 'Select Directory',
  helper: string = 'Choose a folder from your host filesystem',
  initialCreateFolder: boolean = false
) {
  pickerTarget.value = target
  pickerRepoIndex.value = repoIndex
  pickerInitialPath.value = initialPath
  pickerTitle.value = title
  pickerHelperText.value = helper
  pickerInitialCreateFolder.value = initialCreateFolder
  isDirectoryPickerOpen.value = true
}

function handleDirectorySelected(chosenPath: string, directoryItem?: DirectoryItem) {
  if (!pickerTarget.value) return

  switch (pickerTarget.value) {
    case 'create_root':
      createFormRootDir.value = chosenPath
      break
    case 'create_scan':
      scanPath.value = chosenPath
      handleScanDirectory()
      break
    case 'create_repo':
      if (pickerRepoIndex.value !== null && createFormRepos.value[pickerRepoIndex.value]) {
        const row = createFormRepos.value[pickerRepoIndex.value]
        row.path = chosenPath
        if (!row.name || row.name.startsWith('service-')) {
          row.name = chosenPath.split('/').filter(Boolean).pop() || row.name
        }
        if (directoryItem?.suggested_role && directoryItem.suggested_role !== 'other') {
          row.role = directoryItem.suggested_role
        }
        if (directoryItem?.manifest && directoryItem.manifest !== 'unknown') {
          row.manifest = directoryItem.manifest
        }
      }
      break
    case 'create_browse_add': {
      const name = chosenPath.split('/').filter(Boolean).pop() || `service-${createFormRepos.value.length + 1}`
      const role =
        directoryItem?.suggested_role && directoryItem.suggested_role !== 'other'
          ? directoryItem.suggested_role
          : 'backend'
      createFormRepos.value.push({
        name,
        path: chosenPath,
        role,
        manifest: directoryItem?.manifest || 'auto'
      })
      toast.success('Repository Added', `Added ${name} to project from filesystem`)
      break
    }
    case 'edit_root':
      editFormRootDir.value = chosenPath
      break
    case 'edit_scan':
      scanPath.value = chosenPath
      handleScanDirectory()
      break
    case 'edit_repo':
      if (pickerRepoIndex.value !== null && editFormRepos.value[pickerRepoIndex.value]) {
        const row = editFormRepos.value[pickerRepoIndex.value]
        row.path = chosenPath
        if (!row.name || row.name.startsWith('service-')) {
          row.name = chosenPath.split('/').filter(Boolean).pop() || row.name
        }
        if (directoryItem?.suggested_role && directoryItem.suggested_role !== 'other') {
          row.role = directoryItem.suggested_role
        }
        if (directoryItem?.manifest && directoryItem.manifest !== 'unknown') {
          row.manifest = directoryItem.manifest
        }
      }
      break
    case 'edit_browse_add': {
      const name = chosenPath.split('/').filter(Boolean).pop() || `service-${editFormRepos.value.length + 1}`
      const role =
        directoryItem?.suggested_role && directoryItem.suggested_role !== 'other'
          ? directoryItem.suggested_role
          : 'backend'
      editFormRepos.value.push({
        name,
        path: chosenPath,
        role,
        manifest: directoryItem?.manifest || 'auto'
      })
      toast.success('Repository Added', `Added ${name} to project from filesystem`)
      break
    }
    case 'quick_add':
      quickAddPath.value = chosenPath
      if (!quickAddName.value) {
        quickAddName.value = chosenPath.split('/').filter(Boolean).pop() || ''
      }
      if (directoryItem?.suggested_role && directoryItem.suggested_role !== 'other') {
        quickAddRole.value = directoryItem.suggested_role
      }
      break
  }
}

onMounted(async () => {
  await projectStore.fetchProjects()
})

const activeProject = computed(() => projectStore.activeProject)

// Filtered Repos for Matrix & Topology
const filteredRepos = computed(() => {
  if (!activeProject.value?.repos) return []
  let list = activeProject.value.repos

  // Role filter
  if (roleFilter.value !== 'all') {
    list = list.filter((r) => r.role === roleFilter.value)
  }

  // Status filter
  if (statusFilter.value === 'linked') {
    list = list.filter((r) => r.status === 'linked')
  } else if (statusFilter.value === 'broken') {
    list = list.filter((r) => r.status !== 'linked')
  }

  // Search filter
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase().trim()
    list = list.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        r.path.toLowerCase().includes(q) ||
        (r.git_branch && r.git_branch.toLowerCase().includes(q)) ||
        (r.manifest_type && r.manifest_type.toLowerCase().includes(q))
    )
  }

  return list
})

// Metrics & Health Summaries
const projectMetrics = computed(() => {
  const repos = activeProject.value?.repos || []
  const total = repos.length
  const linkedCount = repos.filter((r) => r.status === 'linked').length
  const brokenCount = total - linkedCount

  const roleCounts: Record<string, number> = {
    frontend: 0,
    backend: 0,
    'automation-test': 0,
    contracts: 0,
    artifact: 0,
    other: 0,
  }

  for (const r of repos) {
    if (roleCounts[r.role] !== undefined) {
      roleCounts[r.role]++
    } else {
      roleCounts.other++
    }
  }

  return {
    total,
    linkedCount,
    brokenCount,
    isHealthy: total > 0 && brokenCount === 0,
    roleCounts,
  }
})

// Create Modal Helpers
function onCreateNameInput() {
  createFormId.value = createFormName.value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  if (createFormId.value) {
    createFormRootDir.value = `workspaces/${createFormId.value}`
  }
}

function openCreateModal() {
  createFormName.value = ''
  createFormId.value = ''
  createFormDesc.value = ''
  createFormRootDir.value = ''
  createFormActiveSDLC.value = 'general-ai-sdlc'
  createFormRepos.value = [
    { name: 'frontend-portal', path: '', role: 'frontend', manifest: 'package.json' },
    { name: 'backend-core', path: '', role: 'backend', manifest: 'go.mod' },
    { name: 'automation-test', path: '', role: 'automation-test', manifest: 'playwright.config.ts' },
    { name: 'api-contracts', path: '', role: 'contracts', manifest: 'openapi.yaml' }
  ]
  scannedCandidates.value = []
  scanPath.value = ''
  isCreateModalOpen.value = true
}

async function handleScanDirectory() {
  isScanning.value = true
  try {
    const result = await projectStore.scanDirectory(scanPath.value)
    scannedCandidates.value = result.detected_repos.map((r) => ({
      name: r.name,
      path: r.path,
      role: r.suggested_role,
      manifest: r.manifest_type,
      selected: true,
    }))
    if (scannedCandidates.value.length === 0) {
      toast.info('No Repositories Found', 'Could not detect child codebases at the specified path.')
    } else {
      toast.success('Scan Completed', `Discovered ${scannedCandidates.value.length} potential repository services.`)
    }
  } catch (err: any) {
    // Handled in store
  } finally {
    isScanning.value = false
  }
}

function addScannedToCreateForm() {
  const selected = scannedCandidates.value.filter((c) => c.selected)
  for (const s of selected) {
    if (!createFormRepos.value.some((r) => r.name === s.name)) {
      createFormRepos.value.push({
        name: s.name,
        path: s.path,
        role: s.role,
        manifest: s.manifest,
      })
    }
  }
  scannedCandidates.value = []
  toast.info('Repositories Added', `Added ${selected.length} repositories to project form.`)
}

function addScannedToEditForm() {
  const selected = scannedCandidates.value.filter((c) => c.selected)
  for (const s of selected) {
    if (!editFormRepos.value.some((r) => r.name === s.name)) {
      editFormRepos.value.push({
        name: s.name,
        path: s.path,
        role: s.role,
        manifest: s.manifest,
      })
    }
  }
  scannedCandidates.value = []
  toast.info('Repositories Added', `Added ${selected.length} repositories to edit form.`)
}

async function handleCreateProject() {
  if (!createFormName.value.trim()) {
    toast.error('Validation Error', 'Project name is required')
    return
  }

  if (createFormRepos.value.length === 0) {
    toast.error('Validation Error', 'Please register at least one repository')
    return
  }

  const payload = {
    id: createFormId.value || undefined,
    name: createFormName.value,
    description: createFormDesc.value,
    root_dir: createFormRootDir.value || undefined,
    active_sdlc: createFormActiveSDLC.value,
    repos: createFormRepos.value.map((r) => ({
      name: r.name,
      path: r.path,
      role: r.role,
      manifest_type: r.manifest === 'auto' ? '' : r.manifest,
    })) as ProjectRepo[],
  }

  try {
    await projectStore.createProject(payload)
    isCreateModalOpen.value = false
  } catch (err: any) {
    // Handled by store
  }
}

// Edit Modal Functions
function openEditModal() {
  if (!activeProject.value) return
  const p = activeProject.value
  editFormId.value = p.id
  editFormName.value = p.name
  editFormDesc.value = p.description || ''
  editFormRootDir.value = p.root_dir
  editFormActiveSDLC.value = p.active_sdlc || 'general-ai-sdlc'
  editFormRepos.value = (p.repos || []).map((r) => ({
    id: r.id,
    name: r.name,
    path: r.path,
    role: r.role,
    manifest: r.manifest_type,
  }))
  scannedCandidates.value = []
  scanPath.value = ''
  isEditModalOpen.value = true
}

function addEditRepoRow() {
  editFormRepos.value.push({
    name: `service-${editFormRepos.value.length + 1}`,
    path: '',
    role: 'backend',
    manifest: 'auto',
  })
}

function removeEditRepoRow(idx: number) {
  const repo = editFormRepos.value[idx]
  if (!repo) return
  if (!repo.name && !repo.path) {
    editFormRepos.value.splice(idx, 1)
    return
  }
  triggerConfirmDelete({
    title: 'Remove Repository from Project',
    message: `Are you sure you want to remove "${repo.name || 'this service'}" from the project setup? Upon saving, its workspace symlink will be pruned.`,
    itemName: repo.name || 'Unnamed Service',
    confirmText: 'Remove Row',
    onConfirm: async () => {
      editFormRepos.value.splice(idx, 1)
    }
  })
}

async function handleSaveProjectEdit() {
  if (!editFormName.value.trim()) {
    toast.error('Validation Error', 'Project name cannot be empty')
    return
  }

  if (editFormRepos.value.length === 0) {
    toast.error('Validation Error', 'Project must have at least one repository')
    return
  }

  const payload: Partial<Project> = {
    id: editFormId.value,
    name: editFormName.value,
    description: editFormDesc.value,
    root_dir: editFormRootDir.value,
    active_sdlc: editFormActiveSDLC.value,
    repos: editFormRepos.value.map((r) => ({
      id: r.id,
      name: r.name,
      path: r.path,
      role: r.role,
      manifest_type: r.manifest === 'auto' ? '' : r.manifest,
    })) as ProjectRepo[],
  }

  try {
    await projectStore.updateProject(editFormId.value, payload)
    isEditModalOpen.value = false
  } catch (err: any) {
    // Handled by store
  }
}

// Quick Add Repo
function openQuickAddModal() {
  quickAddName.value = ''
  quickAddPath.value = ''
  quickAddRole.value = 'backend'
  isQuickAddModalOpen.value = true
}

async function handleQuickAddRepo() {
  if (!activeProject.value) return
  if (!quickAddName.value.trim() || !quickAddPath.value.trim()) {
    toast.error('Validation Error', 'Repository name and source path are required')
    return
  }

  const updatedRepos = [...activeProject.value.repos, {
    name: quickAddName.value.trim(),
    path: quickAddPath.value.trim(),
    role: quickAddRole.value,
  } as ProjectRepo]

  try {
    await projectStore.updateProject(activeProject.value.id, {
      ...activeProject.value,
      repos: updatedRepos,
    })
    isQuickAddModalOpen.value = false
    toast.success('Repository Added', `Added ${quickAddName.value} to project ${activeProject.value.name}`)
  } catch (err: any) {
    // Handled by store
  }
}

// Detail Drawer Functions
function openRepoDetail(repo: ProjectRepo) {
  selectedRepoDetail.value = { ...repo }
  isDetailDrawerOpen.value = true
}

async function handleSaveRepoRoleChange(newRole: ProjectRole) {
  if (!activeProject.value || !selectedRepoDetail.value) return
  const repoName = selectedRepoDetail.value.name
  const updatedRepos = activeProject.value.repos.map((r) => {
    if (r.name === repoName) {
      return { ...r, role: newRole }
    }
    return r
  })

  try {
    await projectStore.updateProject(activeProject.value.id, {
      ...activeProject.value,
      repos: updatedRepos,
    })
    selectedRepoDetail.value.role = newRole
    toast.success('Role Updated', `Changed ${repoName} role to ${newRole}`)
  } catch (err: any) {
    // Handled by store
  }
}

function handleRemoveRepoFromDrawer() {
  if (!activeProject.value || !selectedRepoDetail.value) return
  const repoName = selectedRepoDetail.value.name
  const projectId = activeProject.value.id
  const projectName = activeProject.value.name

  triggerConfirmDelete({
    title: 'Unregister Service Repository',
    message: `Are you sure you want to unregister "${repoName}" from project "${projectName}"? Its workspace symlink will be removed.`,
    itemName: repoName,
    note: 'Your source code on host disk remains completely safe and untouched.',
    confirmText: 'Unregister Service',
    onConfirm: async () => {
      const updatedRepos = activeProject.value!.repos.filter((r) => r.name !== repoName)
      await projectStore.updateProject(projectId, {
        ...activeProject.value!,
        repos: updatedRepos,
      })
      isDetailDrawerOpen.value = false
      selectedRepoDetail.value = null
      toast.info('Repository Removed', `Unlinked ${repoName} from project workspace`)
    }
  })
}

async function handleResync() {
  if (!activeProject.value) return
  await projectStore.resyncProject(activeProject.value.id)
}

function handleDeleteProject(id: string) {
  const proj = projectStore.projects.find((p) => p.id === id) || activeProject.value
  const name = proj?.name || id

  triggerConfirmDelete({
    title: 'Delete Project & Workspace',
    message: `Are you sure you want to delete project "${name}" (${id})? All ephemeral symlinks and workspace configurations will be purged.`,
    itemName: name,
    note: 'Your original repositories on host disk are NOT modified or deleted.',
    confirmText: 'Yes, Delete Project',
    onConfirm: async () => {
      await projectStore.deleteProject(id)
    }
  })
}

function copyRootDir() {
  if (!activeProject.value?.root_dir) return
  navigator.clipboard.writeText(activeProject.value.root_dir)
  copiedPath.value = true
  setTimeout(() => { copiedPath.value = false }, 2000)
  toast.info('Copied', 'Project root directory copied to clipboard')
}

function copyCliCommand() {
  if (!activeProject.value?.root_dir) return
  const cmd = `cd ${activeProject.value.root_dir} && ls -la`
  navigator.clipboard.writeText(cmd)
  copiedCliCmd.value = true
  setTimeout(() => { copiedCliCmd.value = false }, 2000)
  toast.info('Copied CLI Command', 'CLI traversal command copied to clipboard')
}

function getRoleBadgeStyle(role: ProjectRole) {
  switch (role) {
    case 'frontend':
      return {
        bg: 'bg-sky-500/10 border-sky-500/30 text-sky-400',
        dot: 'bg-sky-400',
        icon: Layout,
        label: 'Frontend UI',
      }
    case 'backend':
      return {
        bg: 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400',
        dot: 'bg-emerald-400',
        icon: Server,
        label: 'Backend Service',
      }
    case 'automation-test':
      return {
        bg: 'bg-purple-500/10 border-purple-500/30 text-purple-400',
        dot: 'bg-purple-400',
        icon: FlaskConical,
        label: 'Automation Test',
      }
    case 'contracts':
      return {
        bg: 'bg-amber-500/10 border-amber-500/30 text-amber-400',
        dot: 'bg-amber-400',
        icon: FileCode2,
        label: 'API Contracts',
      }
    case 'artifact':
      return {
        bg: 'bg-blue-500/10 border-blue-500/30 text-blue-400',
        dot: 'bg-blue-400',
        icon: FolderArchive,
        label: 'Artifacts & PRD',
      }
    default:
      return {
        bg: 'bg-slate-500/10 border-slate-500/30 text-slate-400',
        dot: 'bg-slate-400',
        icon: Layers,
        label: 'Other Service',
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
              <span class="text-xs px-2.5 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 font-mono">
                Auto-Symlink Ready
              </span>
            </h1>
            <p class="text-xs text-slate-400 mt-0.5">
              Register distributed multi-repos, tag functional roles (Frontend, Backend, Tests, Contracts), and synthesize unified project roots.
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-2.5">
        <button
          v-if="activeProject"
          @click="openEditModal"
          class="px-3.5 py-2 rounded-lg border border-slate-700 bg-slate-800/80 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-2 transition"
        >
          <Edit3 class="w-4 h-4 text-emerald-400" />
          Edit Project & Topology
        </button>

        <button
          v-if="activeProject"
          @click="handleResync"
          :disabled="projectStore.isLoading"
          class="px-3.5 py-2 rounded-lg border border-slate-700 bg-slate-800/80 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-2 transition"
          title="Re-verify source paths and recreate symlinks"
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
      <!-- Architectural Explainer Card: Clarifies Source vs Workspace vs Symlink -->
      <div class="p-4 rounded-2xl bg-gradient-to-r from-slate-900 via-slate-900/90 to-slate-950 border border-slate-800 text-xs space-y-3 shadow-lg">
        <div class="flex items-center justify-between cursor-pointer select-none" @click="showArchitectureExplainer = !showArchitectureExplainer">
          <div class="flex items-center gap-2 text-slate-200 font-semibold">
            <HelpCircle class="w-4 h-4 text-emerald-400" />
            <span class="text-sm">Architecture Mental Model: Host Source vs. Unified Workspace vs. Symlink Mounts</span>
          </div>
          <button
            type="button"
            class="flex items-center gap-1.5 text-slate-400 hover:text-slate-200 text-[11px] font-mono transition"
          >
            <span>{{ showArchitectureExplainer ? 'Hide Guide' : 'Show Guide' }}</span>
            <ChevronUp v-if="showArchitectureExplainer" class="w-4 h-4 text-slate-400" />
            <ChevronDown v-else class="w-4 h-4 text-slate-400" />
          </button>
        </div>

        <div v-if="showArchitectureExplainer" class="grid grid-cols-1 md:grid-cols-3 gap-3 pt-3 border-t border-slate-800/80 animate-in fade-in duration-200">
          <!-- Pillar 1: Host Source Repos -->
          <div class="p-3.5 rounded-xl bg-slate-950/80 border border-slate-800 space-y-1.5">
            <div class="flex items-center justify-between">
              <span class="flex items-center gap-1.5 text-sky-400 font-bold font-mono">
                <FolderGit2 class="w-4 h-4" /> 1. Host Source Repos
              </span>
              <span class="px-1.5 py-0.5 rounded text-[10px] font-mono bg-sky-500/10 text-sky-400 border border-sky-500/20">Disk Storage</span>
            </div>
            <p class="text-slate-400 text-[11px] leading-relaxed">
              Your real repositories on your machine (e.g. <code class="text-slate-300 font-mono">/Users/.../frontend</code>). Where your git history and commits live.
            </p>
            <div class="text-[10px] text-slate-400 font-mono pt-1">
              Browse & pick these folders with 1-click.
            </div>
          </div>

          <!-- Pillar 2: Workspace Symlinks -->
          <div class="p-3.5 rounded-xl bg-slate-950/80 border border-slate-800 space-y-1.5">
            <div class="flex items-center justify-between">
              <span class="flex items-center gap-1.5 text-emerald-400 font-bold font-mono">
                <Network class="w-4 h-4" /> 2. Workspace Symlinks
              </span>
              <span class="px-1.5 py-0.5 rounded text-[10px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">Zero-Copy Bridge</span>
            </div>
            <p class="text-slate-400 text-[11px] leading-relaxed">
              Virtual symbolic links created inside the workspace. Any edits made by AI agents or tests update your real host source files instantly in real time.
            </p>
            <div class="text-[10px] text-emerald-400/90 font-mono pt-1">
              Auto-wired by Meta-Orchestrator.
            </div>
          </div>

          <!-- Pillar 3: Unified Workspace -->
          <div class="p-3.5 rounded-xl bg-slate-950/80 border border-slate-800 space-y-1.5">
            <div class="flex items-center justify-between">
              <span class="flex items-center gap-1.5 text-purple-400 font-bold font-mono">
                <Layers class="w-4 h-4" /> 3. Unified Workspace Root
              </span>
              <span class="px-1.5 py-0.5 rounded text-[10px] font-mono bg-purple-500/10 text-purple-400 border border-purple-500/20">Orchestrator Hub</span>
            </div>
            <p class="text-slate-400 text-[11px] leading-relaxed">
              The project's dedicated orchestrator root (e.g. <code class="text-slate-300 font-mono">workspaces/my-project/</code>) where agents, tasks, PRDs, and ATDD tests operate across all repos together.
            </p>
            <div class="text-[10px] text-purple-400/90 font-mono pt-1">
              Houses all mounts in one single context.
            </div>
          </div>
        </div>
      </div>

      <!-- Project Selection Ribbon -->
      <section class="space-y-3">
        <div class="flex items-center justify-between">
          <span class="text-xs font-bold uppercase tracking-wider text-slate-400 font-mono">
            Registered Projects ({{ projectStore.projects.length }})
          </span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div
            v-for="p in projectStore.projects"
            :key="p.id"
            @click="projectStore.selectProject(p.id)"
            class="p-4 rounded-xl border transition-all cursor-pointer text-left relative overflow-hidden group"
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
        <!-- Project Banner & Health Stats -->
        <div class="p-6 rounded-2xl bg-gradient-to-r from-slate-900 via-slate-900/95 to-slate-950 border border-slate-800 space-y-5">
          <div class="flex flex-col md:flex-row md:items-start justify-between gap-4">
            <div class="space-y-1.5">
              <div class="flex flex-wrap items-center gap-3">
                <h2 class="text-xl font-bold text-white">{{ activeProject.name }}</h2>
                <span class="px-2.5 py-0.5 rounded-full text-xs font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  {{ activeProject.active_sdlc }}
                </span>
                <span
                  class="px-2 py-0.5 rounded text-[10px] font-semibold uppercase tracking-wider font-mono border"
                  :class="projectMetrics.isHealthy ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border-rose-500/20'"
                >
                  {{ projectMetrics.isHealthy ? '● All Symlinks Healthy' : `● ${projectMetrics.brokenCount} Broken Link(s)` }}
                </span>
              </div>
              <p class="text-xs text-slate-400 max-w-3xl leading-relaxed">{{ activeProject.description }}</p>
            </div>

            <!-- Action Controls -->
            <div class="flex flex-wrap items-center gap-2">
              <button
                @click="openQuickAddModal"
                class="px-3 py-1.5 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-1.5 transition"
              >
                <Plus class="w-3.5 h-3.5 text-emerald-400" />
                Add Service
              </button>
              <button
                @click="launchTaskForProject"
                class="px-3.5 py-1.5 rounded-lg bg-emerald-600/90 hover:bg-emerald-500 text-xs font-semibold text-white flex items-center gap-1.5 shadow transition"
              >
                <Play class="w-3.5 h-3.5 fill-current" />
                New Task in Project
              </button>
              <button
                @click="handleDeleteProject(activeProject.id)"
                class="p-2 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 text-xs transition"
                title="Delete Project"
              >
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          </div>

          <!-- Role Distribution Quick Chips -->
          <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2.5 pt-2 border-t border-slate-800/80">
            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-sky-400 font-medium">
                <Layout class="w-3.5 h-3.5" /> Frontend
              </span>
              <span class="font-mono font-bold text-white">{{ projectMetrics.roleCounts.frontend }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-emerald-400 font-medium">
                <Server class="w-3.5 h-3.5" /> Backend
              </span>
              <span class="font-mono font-bold text-white">{{ projectMetrics.roleCounts.backend }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-purple-400 font-medium">
                <FlaskConical class="w-3.5 h-3.5" /> Tests
              </span>
              <span class="font-mono font-bold text-white">{{ projectMetrics.roleCounts['automation-test'] }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-amber-400 font-medium">
                <FileCode2 class="w-3.5 h-3.5" /> Contracts
              </span>
              <span class="font-mono font-bold text-white">{{ projectMetrics.roleCounts.contracts }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-blue-400 font-medium">
                <FolderArchive class="w-3.5 h-3.5" /> Artifacts
              </span>
              <span class="font-mono font-bold text-white">{{ projectMetrics.roleCounts.artifact }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-slate-400 font-medium">
                <Layers class="w-3.5 h-3.5" /> Total
              </span>
              <span class="font-mono font-bold text-white">{{ projectMetrics.total }}</span>
            </div>
          </div>

          <!-- Root Dir Box & Terminal helper -->
          <div class="p-3.5 rounded-xl bg-slate-950/80 border border-slate-800/80 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs">
            <div class="flex items-center gap-2.5 min-w-0">
              <ShieldCheck class="w-4 h-4 text-emerald-400 flex-shrink-0" />
              <div class="truncate">
                <span class="text-slate-400 font-mono">Unified Workspace Root: </span>
                <span class="text-emerald-400 font-mono font-medium">{{ activeProject.root_dir }}</span>
                <span class="ml-2 text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-mono">
                  Orchestrator Base
                </span>
              </div>
            </div>

            <div class="flex items-center gap-2 flex-shrink-0">
              <button
                @click="copyCliCommand"
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs flex items-center gap-1.5 transition"
                title="Copy shell command to cd into unified workspace directory"
              >
                <Terminal class="w-3.5 h-3.5 text-sky-400" />
                <span>{{ copiedCliCmd ? 'Copied' : 'Copy CLI cd' }}</span>
              </button>

              <button
                @click="copyRootDir"
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs flex items-center gap-1.5 transition"
                title="Copy workspace root path"
              >
                <Check v-if="copiedPath" class="w-3.5 h-3.5 text-emerald-400" />
                <Copy v-else class="w-3.5 h-3.5" />
                <span>{{ copiedPath ? 'Copied' : 'Copy Path' }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- Toolbar: View Mode Toggle, Search & Filters -->
        <div class="space-y-4">
          <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
            <!-- Search & Status Filter -->
            <div class="flex flex-wrap items-center gap-2.5">
              <div class="relative w-64">
                <Search class="w-3.5 h-3.5 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  v-model="searchQuery"
                  type="text"
                  placeholder="Search repos, paths, branches..."
                  class="w-full pl-9 pr-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-200 text-xs font-mono placeholder-slate-500 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <!-- Status filter dropdown -->
              <select
                v-model="statusFilter"
                class="px-2.5 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-300 text-xs font-mono focus:outline-none focus:border-emerald-500"
              >
                <option value="all">All Statuses</option>
                <option value="linked">Linked Only</option>
                <option value="broken">Broken / Missing Only</option>
              </select>

              <!-- View Mode Toggle -->
              <div class="flex items-center bg-slate-900 p-0.5 rounded-lg border border-slate-800 text-xs">
                <button
                  @click="viewMode = 'grid'"
                  class="px-2.5 py-1 rounded flex items-center gap-1 font-medium transition"
                  :class="viewMode === 'grid' ? 'bg-slate-800 text-white shadow' : 'text-slate-400 hover:text-slate-200'"
                >
                  <Grid class="w-3.5 h-3.5" />
                  Matrix
                </button>
                <button
                  @click="viewMode = 'topology'"
                  class="px-2.5 py-1 rounded flex items-center gap-1 font-medium transition"
                  :class="viewMode === 'topology' ? 'bg-slate-800 text-white shadow' : 'text-slate-400 hover:text-slate-200'"
                >
                  <Network class="w-3.5 h-3.5 text-emerald-400" />
                  Topology Tree
                </button>
              </div>
            </div>

            <!-- Role Filter Pills -->
            <div class="flex flex-wrap items-center gap-1.5 bg-slate-900 p-1 rounded-lg border border-slate-800 text-xs">
              <button
                @click="roleFilter = 'all'"
                class="px-2.5 py-1 rounded text-xs transition font-medium"
                :class="roleFilter === 'all' ? 'bg-slate-800 text-white shadow' : 'text-slate-400 hover:text-slate-200'"
              >
                All ({{ activeProject.repos?.length || 0 }})
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
                <FlaskConical class="w-3 h-3" /> Tests
              </button>
              <button
                @click="roleFilter = 'contracts'"
                class="px-2.5 py-1 rounded text-xs transition font-medium flex items-center gap-1"
                :class="roleFilter === 'contracts' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/30' : 'text-slate-400 hover:text-slate-200'"
              >
                <FileCode2 class="w-3 h-3" /> Contracts
              </button>
              <button
                @click="roleFilter = 'artifact'"
                class="px-2.5 py-1 rounded text-xs transition font-medium flex items-center gap-1"
                :class="roleFilter === 'artifact' ? 'bg-blue-500/20 text-blue-300 border border-blue-500/30' : 'text-slate-400 hover:text-slate-200'"
              >
                <FolderArchive class="w-3 h-3" /> Artifacts
              </button>
            </div>
          </div>

          <!-- View Mode A: Matrix Cards View -->
          <div v-if="viewMode === 'grid'" class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div
              v-for="repo in filteredRepos"
              :key="repo.id || repo.name"
              @click="openRepoDetail(repo)"
              class="p-4 rounded-xl bg-slate-900/60 border border-slate-800 space-y-3 relative overflow-hidden group hover:border-slate-700 hover:bg-slate-900/90 transition cursor-pointer"
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
                  <span class="font-bold text-sm text-white font-mono group-hover:text-emerald-400 transition">
                    {{ repo.name }}
                  </span>
                </div>

                <div class="flex items-center gap-2 text-xs font-mono">
                  <span
                    v-if="repo.git_branch"
                    class="flex items-center gap-1 px-2 py-0.5 rounded bg-slate-950 border border-slate-800 text-[10px] text-slate-300"
                    title="Git active branch"
                  >
                    <GitBranch class="w-3 h-3 text-emerald-400" />
                    {{ repo.git_branch }}
                  </span>

                  <span
                    class="flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold border"
                    :class="repo.status === 'linked' ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border-rose-500/20'"
                  >
                    <CheckCircle2 v-if="repo.status === 'linked'" class="w-3 h-3" />
                    <AlertTriangle v-else class="w-3 h-3" />
                    {{ repo.status === 'linked' ? 'Linked' : (repo.status || 'Broken') }}
                  </span>
                </div>
              </div>

              <!-- Paths Details -->
              <div class="space-y-1.5 text-xs font-mono">
                <div class="flex items-center justify-between text-slate-400 gap-2">
                  <span class="text-[11px] text-slate-400 flex-shrink-0" title="Physical source path on host machine">Host Source:</span>
                  <span class="text-slate-300 truncate max-w-[280px]" :title="repo.path">{{ repo.path }}</span>
                </div>

                <div class="flex items-center justify-between text-slate-400 gap-2">
                  <span class="text-[11px] text-emerald-400/90 flex-shrink-0" title="Unified symlink mount point inside workspace root">Workspace Mount:</span>
                  <span class="text-emerald-400 font-medium truncate max-w-[280px]" :title="repo.symlink_path">{{ repo.symlink_path }}</span>
                </div>

                <div class="flex items-center justify-between text-slate-400 pt-1.5 border-t border-slate-800/60">
                  <div class="flex items-center gap-2">
                    <span class="text-[10px] text-slate-400">Manifest:</span>
                    <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-300 text-[10px] font-bold">
                      {{ repo.manifest_type || 'unknown' }}
                    </span>
                  </div>

                  <span class="text-[11px] text-emerald-400/80 group-hover:text-emerald-300 font-sans flex items-center gap-1">
                    <Eye class="w-3.5 h-3.5" /> View Details →
                  </span>
                </div>
              </div>

              <div v-if="repo.error" class="p-2 rounded bg-rose-950/40 border border-rose-900/50 text-[11px] text-rose-300 font-mono">
                {{ repo.error }}
              </div>
            </div>
          </div>

          <!-- View Mode B: Interactive Topology Visualizer -->
          <div v-else class="p-6 rounded-2xl bg-slate-950 border border-slate-800 space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <h4 class="font-bold text-sm text-white flex items-center gap-2">
                  <Network class="w-4 h-4 text-emerald-400" />
                  Workspace Symlink & Topology Graph
                </h4>
                <p class="text-xs text-slate-400">Visual topology showing how distributed host repositories are mounted into the unified project workspace.</p>
              </div>
              <span class="text-xs font-mono text-slate-400">{{ filteredRepos.length }} Services Connected</span>
            </div>

            <div class="p-8 rounded-xl bg-slate-900/40 border border-slate-800/80 flex flex-col items-center justify-center space-y-8 relative overflow-hidden">
              <!-- Central Project Root Node -->
              <div class="relative z-10 flex flex-col items-center">
                <div class="px-5 py-3 rounded-xl bg-emerald-950/80 border-2 border-emerald-500 shadow-xl shadow-emerald-950/50 flex items-center gap-3 text-emerald-300 font-mono text-xs font-bold">
                  <FolderGit2 class="w-5 h-5 text-emerald-400" />
                  <div>
                    <div class="flex items-center gap-2">
                      <span>{{ activeProject.name }}</span>
                      <span class="text-[9px] px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-mono">Workspace Root</span>
                    </div>
                    <div class="text-[10px] text-emerald-400/70 font-normal mt-0.5">{{ activeProject.root_dir }}</div>
                  </div>
                </div>
                <div class="w-0.5 h-8 bg-gradient-to-b from-emerald-500 to-slate-700"></div>
              </div>

              <!-- Radiating Service Nodes -->
              <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 w-full max-w-4xl relative z-10">
                <div
                  v-for="repo in filteredRepos"
                  :key="repo.name"
                  @click="openRepoDetail(repo)"
                  class="p-3.5 rounded-xl border bg-slate-900/90 shadow-md cursor-pointer hover:scale-[1.02] transition"
                  :class="getRoleBadgeStyle(repo.role).bg"
                >
                  <div class="flex items-start justify-between gap-2">
                    <div class="flex items-center gap-2">
                      <component :is="getRoleBadgeStyle(repo.role).icon" class="w-4 h-4" />
                      <span class="font-mono font-bold text-xs text-white">{{ repo.name }}</span>
                    </div>
                    <span
                      class="w-2 h-2 rounded-full"
                      :class="repo.status === 'linked' ? 'bg-emerald-400' : 'bg-rose-400 animate-pulse'"
                      :title="repo.status"
                    ></span>
                  </div>

                  <div class="mt-2 text-[10px] font-mono text-slate-400 space-y-0.5 truncate">
                    <div class="truncate text-emerald-400">Mount: {{ repo.symlink_path }}</div>
                    <div class="truncate text-slate-400">Source: {{ repo.path }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <!-- Repository Detail Slide-Over Drawer -->
    <div
      v-if="isDetailDrawerOpen && selectedRepoDetail"
      class="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex justify-end transition-opacity"
      @click.self="isDetailDrawerOpen = false"
    >
      <div class="w-full max-w-md bg-slate-900 border-l border-slate-800 h-full flex flex-col shadow-2xl p-6 overflow-y-auto space-y-6">
        <div class="flex items-center justify-between border-b border-slate-800 pb-4">
          <div class="flex items-center gap-2.5">
            <component :is="getRoleBadgeStyle(selectedRepoDetail.role).icon" class="w-5 h-5 text-emerald-400" />
            <div>
              <h3 class="font-bold text-base text-white font-mono">{{ selectedRepoDetail.name }}</h3>
              <span class="text-xs text-slate-400">Repository Details & Health</span>
            </div>
          </div>
          <button @click="isDetailDrawerOpen = false" class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition">
            <X class="w-5 h-5" />
          </button>
        </div>

        <!-- Symlink Status Callout -->
        <div
          class="p-3.5 rounded-xl border flex items-center justify-between text-xs font-mono"
          :class="selectedRepoDetail.status === 'linked' ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300' : 'bg-rose-500/10 border-rose-500/30 text-rose-300'"
        >
          <div class="flex items-center gap-2">
            <CheckCircle2 v-if="selectedRepoDetail.status === 'linked'" class="w-4 h-4 text-emerald-400" />
            <AlertTriangle v-else class="w-4 h-4 text-rose-400" />
            <span>{{ selectedRepoDetail.status === 'linked' ? 'Symlink Active & Resolving' : 'Source Path Missing or Broken' }}</span>
          </div>
          <span class="uppercase text-[10px] font-bold px-2 py-0.5 rounded bg-black/30 border border-current">
            {{ selectedRepoDetail.status }}
          </span>
        </div>

        <!-- Role Changer -->
        <div class="space-y-2">
          <label class="text-xs font-semibold text-slate-300">Functional Role & Classification</label>
          <select
            v-model="selectedRepoDetail.role"
            @change="handleSaveRepoRoleChange(selectedRepoDetail.role)"
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 text-xs font-mono focus:outline-none focus:border-emerald-500"
          >
            <option value="frontend">🎨 Frontend UI</option>
            <option value="backend">⚙️ Backend Service</option>
            <option value="automation-test">🧪 Automation Test</option>
            <option value="contracts">📜 API Contracts</option>
            <option value="artifact">📁 Artifacts & PRD</option>
            <option value="other">🔌 Other Service</option>
          </select>
          <p class="text-[11px] text-slate-400">Updating role immediately adjusts task routing and AST inspection profiles.</p>
        </div>

        <!-- Deep Specs Grid -->
        <div class="space-y-3 border-t border-slate-800 pt-4 text-xs font-mono">
          <div class="space-y-1">
            <span class="text-slate-400 text-[11px] font-sans font-semibold">Host Source Directory (Real Code on Disk):</span>
            <div class="p-2.5 rounded-lg bg-slate-950 border border-slate-800/80 text-slate-300 break-all select-all">
              {{ selectedRepoDetail.path }}
            </div>
          </div>

          <div class="space-y-1">
            <span class="text-emerald-400/90 text-[11px] font-sans font-semibold">Workspace Symlink Mount (Virtual Link):</span>
            <div class="p-2.5 rounded-lg bg-slate-950 border border-slate-800/80 text-emerald-400 break-all select-all">
              {{ selectedRepoDetail.symlink_path }}
            </div>
            <p class="text-[10px] text-slate-400 font-sans">
              Tasks and tools executing inside the unified workspace read and write to this mount, syncing directly to host source files.
            </p>
          </div>

          <div class="grid grid-cols-2 gap-2 pt-1">
            <div class="p-2.5 rounded-lg bg-slate-950 border border-slate-800/80 space-y-0.5">
              <span class="text-slate-400 text-[10px]">Detected Manifest:</span>
              <div class="font-bold text-slate-200 text-xs">{{ selectedRepoDetail.manifest_type || 'unknown' }}</div>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950 border border-slate-800/80 space-y-0.5">
              <span class="text-slate-400 text-[10px]">Git Branch:</span>
              <div class="font-bold text-emerald-400 text-xs">{{ selectedRepoDetail.git_branch || 'main / detached' }}</div>
            </div>
          </div>
        </div>

        <!-- Drawer Action Footer -->
        <div class="pt-6 border-t border-slate-800 space-y-2 mt-auto">
          <button
            @click="handleRemoveRepoFromDrawer"
            class="w-full py-2.5 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-300 border border-rose-500/30 text-xs font-semibold flex items-center justify-center gap-2 transition"
          >
            <Trash2 class="w-4 h-4" />
            Unregister Repository
          </button>
        </div>
      </div>
    </div>

    <!-- Quick Add Single Repo Modal -->
    <div
      v-if="isQuickAddModalOpen"
      class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 overflow-y-auto"
    >
      <div class="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-lg p-6 shadow-2xl space-y-5">
        <div class="flex items-center justify-between border-b border-slate-800 pb-3">
          <div class="flex items-center gap-2">
            <Plus class="w-5 h-5 text-emerald-400" />
            <h3 class="font-bold text-base text-white">Add Service to Project</h3>
          </div>
          <button @click="isQuickAddModalOpen = false" class="text-slate-400 hover:text-white">
            <X class="w-5 h-5" />
          </button>
        </div>

        <div class="space-y-4 text-xs">
          <div class="space-y-1.5">
            <label class="font-semibold text-slate-300">Service / Repo Name *</label>
            <input
              v-model="quickAddName"
              type="text"
              placeholder="e.g. billing-service"
              class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono focus:outline-none focus:border-emerald-500"
            />
          </div>

          <div class="space-y-1.5">
            <label class="font-semibold text-slate-300">Role / Type *</label>
            <select
              v-model="quickAddRole"
              class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono focus:outline-none focus:border-emerald-500"
            >
              <option value="frontend">🎨 Frontend UI</option>
              <option value="backend">⚙️ Backend Service</option>
              <option value="automation-test">🧪 Automation Test</option>
              <option value="contracts">📜 API Contracts</option>
              <option value="artifact">📁 Artifacts & PRD</option>
              <option value="other">🔌 Other Service</option>
            </select>
          </div>

          <div class="space-y-1.5">
            <label class="font-semibold text-slate-300">Host Source Directory *</label>
            <div class="flex gap-2">
              <input
                v-model="quickAddPath"
                type="text"
                placeholder="/Users/name/Projects/service-name"
                class="flex-1 px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
              />
              <button
                type="button"
                @click="openDirectoryPicker('quick_add', null, quickAddPath, 'Select Service Source Directory', 'Choose the repository folder on your machine')"
                class="px-3 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                Browse
              </button>
            </div>
            <div v-if="quickAddName && activeProject" class="text-[11px] font-mono text-slate-400 pt-0.5">
              Workspace Mount: <span class="text-emerald-400">{{ activeProject.root_dir }}/{{ quickAddName }}</span>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between pt-4 border-t border-slate-800">
          <button
            @click="isQuickAddModalOpen = false"
            class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 text-xs font-semibold hover:bg-slate-700 transition"
          >
            Cancel
          </button>
          <button
            @click="handleQuickAddRepo"
            class="px-5 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold flex items-center gap-2 shadow transition"
          >
            <Plus class="w-4 h-4" />
            Add & Symlink
          </button>
        </div>
      </div>
    </div>

    <!-- Complete Project Edit Modal -->
    <div
      v-if="isEditModalOpen"
      class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 overflow-y-auto"
    >
      <div class="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-4xl max-h-[90vh] flex flex-col shadow-2xl overflow-hidden my-8">
        <!-- Modal Header -->
        <div class="p-5 border-b border-slate-800 flex items-center justify-between bg-slate-900/90">
          <div class="flex items-center gap-2.5">
            <div class="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
              <Edit3 class="w-5 h-5" />
            </div>
            <div>
              <h2 class="font-bold text-base text-white">Edit Project Configuration & Repositories</h2>
              <p class="text-xs text-slate-400">Modify project topology, adjust service roles, and re-provision filesystem symlinks</p>
            </div>
          </div>
          <button @click="isEditModalOpen = false" class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition">
            <X class="w-5 h-5" />
          </button>
        </div>

        <!-- Modal Body -->
        <div class="p-6 space-y-6 overflow-y-auto flex-1 text-xs">
          <!-- Metadata Fields -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Project Name *</label>
              <input
                v-model="editFormName"
                type="text"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500 font-medium"
              />
            </div>

            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Active SDLC Workflow</label>
              <select
                v-model="editFormActiveSDLC"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono focus:outline-none focus:border-emerald-500"
              >
                <option value="general-ai-sdlc">General AI SDLC (9 Stages)</option>
                <option value="microservice-api">Microservice API Contract Workflow (4 Stages)</option>
                <option value="hotfix-fast-track">Hotfix Fast-Track (3 Stages)</option>
              </select>
            </div>

            <div class="space-y-1.5 md:col-span-2">
              <label class="font-semibold text-slate-200">Description</label>
              <input
                v-model="editFormDesc"
                type="text"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div class="space-y-1.5 md:col-span-2">
              <div class="flex items-center justify-between">
                <label class="font-semibold text-slate-200">Unified Workspace Directory (Root Target)</label>
                <button
                  type="button"
                  @click="openDirectoryPicker('edit_root', null, editFormRootDir, 'Create & Select Root Directory', 'Create a new workspace folder or choose an existing root directory', true)"
                  class="text-[11px] text-emerald-400 hover:text-emerald-300 font-medium flex items-center gap-1 hover:underline"
                >
                  <FolderPlus class="w-3.5 h-3.5" />
                  <span>+ Create New Folder</span>
                </button>
              </div>
              <div class="flex gap-2">
                <input
                  v-model="editFormRootDir"
                  type="text"
                  placeholder="workspaces/project-id"
                  class="flex-1 px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-emerald-400 font-mono text-xs focus:outline-none focus:border-emerald-500"
                />
                <button
                  type="button"
                  @click="openDirectoryPicker('edit_root', null, editFormRootDir, 'Select Workspace Directory', 'Choose where the unified project workspace and symlinks will reside')"
                  class="px-3.5 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
                >
                  <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                  Browse
                </button>
              </div>
              <p class="text-[11px] text-slate-400">The root orchestrator folder where tasks, tools, and symlink mounts reside.</p>
            </div>
          </div>

          <!-- Directory Auto-Scanner inside Edit Modal -->
          <div class="p-4 rounded-xl bg-slate-950 border border-slate-800/80 space-y-3">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Search class="w-4 h-4 text-emerald-400" />
                <h4 class="font-bold text-slate-200 uppercase tracking-wider text-[11px] font-mono">Scan Local Directory for Additional Services</h4>
              </div>
              <span class="text-[10px] text-slate-400">Detects manifests and auto-assigns roles</span>
            </div>

            <div class="flex gap-2">
              <input
                v-model="scanPath"
                type="text"
                placeholder="Parent folder path to discover repos..."
                class="flex-1 px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-slate-300 font-mono text-xs focus:outline-none focus:border-emerald-500"
              />
              <button
                type="button"
                @click="openDirectoryPicker('edit_scan', null, scanPath, 'Select Parent Folder to Scan', 'Select a directory to automatically detect child repositories and service manifests')"
                class="px-3 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                Browse
              </button>
              <button
                @click="handleScanDirectory"
                :disabled="isScanning"
                class="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isScanning }" />
                Scan
              </button>
            </div>

            <!-- Scanned candidates -->
            <div v-if="scannedCandidates.length > 0" class="space-y-2 pt-2 border-t border-slate-800">
              <div class="text-[11px] font-mono text-emerald-400 flex items-center justify-between">
                <span>Discovered {{ scannedCandidates.length }} repositories:</span>
                <button @click="addScannedToEditForm" class="text-xs underline text-emerald-400 hover:text-emerald-300 font-bold">
                  + Add Selected Repositories to Project
                </button>
              </div>

              <div class="space-y-1.5 max-h-36 overflow-y-auto pr-1">
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

          <!-- Repositories List -->
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <h4 class="font-bold text-slate-200 uppercase tracking-wider text-[11px] font-mono">
                Project Repositories & Role Mappings ({{ editFormRepos.length }})
              </h4>
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  @click="openDirectoryPicker('edit_browse_add', null, '', 'Browse & Add Service Repository', 'Choose a service folder from host disk to add it directly')"
                  class="px-2.5 py-1 rounded bg-emerald-600/90 hover:bg-emerald-600 text-white text-xs font-semibold flex items-center gap-1 transition shadow-sm"
                >
                  <FolderOpen class="w-3.5 h-3.5" />
                  Browse & Add Repo
                </button>
                <button
                  type="button"
                  @click="addEditRepoRow"
                  class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-emerald-400 text-xs font-semibold flex items-center gap-1 transition"
                >
                  <Plus class="w-3.5 h-3.5" />
                  Add Row
                </button>
              </div>
            </div>

            <div class="space-y-2.5">
              <div
                v-for="(repo, idx) in editFormRepos"
                :key="idx"
                class="p-3.5 rounded-xl bg-slate-950 border border-slate-800/80 space-y-2.5"
              >
                <div class="grid grid-cols-1 md:grid-cols-12 gap-2.5 items-center">
                  <div class="md:col-span-3">
                    <label class="text-[10px] text-slate-400 font-mono">Service Name</label>
                    <input
                      v-model="repo.name"
                      type="text"
                      class="w-full px-2.5 py-1.5 rounded bg-slate-900 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
                    />
                  </div>

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

                  <div class="md:col-span-5">
                    <label class="text-[10px] text-slate-400 font-mono">Host Source Path</label>
                    <div class="flex gap-1.5 mt-0.5">
                      <input
                        v-model="repo.path"
                        type="text"
                        placeholder="/path/to/source"
                        class="flex-1 px-2.5 py-1.5 rounded bg-slate-900 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
                      />
                      <button
                        type="button"
                        @click="openDirectoryPicker('edit_repo', idx, repo.path, `Select Source Folder for ${repo.name}`, 'Pick the source code repository folder on your machine')"
                        class="px-2.5 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
                        title="Browse filesystem for this repository"
                      >
                        <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                        Browse
                      </button>
                    </div>
                  </div>

                  <div class="md:col-span-1 flex justify-end pt-3">
                    <button
                      @click="removeEditRepoRow(idx)"
                      class="p-1.5 rounded text-slate-400 hover:text-rose-400 hover:bg-slate-900 transition"
                      title="Remove repository"
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>
                </div>

                <div class="flex items-center justify-between text-[10px] font-mono text-slate-400 px-1 pt-1.5 border-t border-slate-800/40">
                  <div class="truncate">
                    <span>Workspace Mount: </span>
                    <span class="text-emerald-400">{{ editFormRootDir }}/{{ repo.name || 'service' }}</span>
                  </div>
                  <div v-if="repo.manifest && repo.manifest !== 'auto'" class="flex-shrink-0">
                    <span>Manifest: </span>
                    <span class="text-slate-300">{{ repo.manifest }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Modal Footer -->
        <div class="p-4 border-t border-slate-800 bg-slate-900/90 flex items-center justify-between">
          <button
            @click="isEditModalOpen = false"
            class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-semibold transition"
          >
            Cancel
          </button>

          <button
            @click="handleSaveProjectEdit"
            :disabled="projectStore.isLoading"
            class="px-5 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-xs font-semibold text-white flex items-center gap-2 shadow-lg shadow-emerald-950/50 transition"
          >
            <Check class="w-4 h-4" />
            Save & Update Symlinks
          </button>
        </div>
      </div>
    </div>

    <!-- Register Project Modal (Original creation wizard) -->
    <div
      v-if="isCreateModalOpen"
      class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 overflow-y-auto"
    >
      <div class="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-4xl max-h-[90vh] flex flex-col shadow-2xl overflow-hidden my-8">
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

        <div class="p-6 space-y-6 overflow-y-auto flex-1 text-xs">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Project Name <span class="text-emerald-400">*</span></label>
              <input
                v-model="createFormName"
                @input="onCreateNameInput"
                type="text"
                placeholder="e.g. Fintech Payment Gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500 font-medium"
              />
            </div>

            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Project ID (Slug)</label>
              <input
                v-model="createFormId"
                type="text"
                placeholder="fintech-payment-gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-400 font-mono focus:outline-none"
              />
            </div>

            <div class="space-y-1.5 md:col-span-2">
              <label class="font-semibold text-slate-200">Description</label>
              <input
                v-model="createFormDesc"
                type="text"
                placeholder="Multi-repo microservice architecture connecting Vue frontend, Go API, and Playwright tests"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div class="space-y-1.5 md:col-span-2">
              <div class="flex items-center justify-between">
                <label class="font-semibold text-slate-200">Unified Workspace Directory (Root Target)</label>
                <button
                  type="button"
                  @click="openDirectoryPicker('create_root', null, createFormRootDir, 'Create & Select Root Directory', 'Create a new workspace folder or choose an existing root directory', true)"
                  class="text-[11px] text-emerald-400 hover:text-emerald-300 font-medium flex items-center gap-1 hover:underline"
                >
                  <FolderPlus class="w-3.5 h-3.5" />
                  <span>+ Create New Folder</span>
                </button>
              </div>
              <div class="flex gap-2">
                <input
                  v-model="createFormRootDir"
                  type="text"
                  placeholder="workspaces/fintech-payment-gateway"
                  class="flex-1 px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-emerald-400 font-mono text-xs focus:outline-none focus:border-emerald-500"
                />
                <button
                  type="button"
                  @click="openDirectoryPicker('create_root', null, createFormRootDir, 'Select Workspace Directory', 'Choose where the unified project workspace and symlinks will reside')"
                  class="px-3.5 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
                >
                  <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                  Browse
                </button>
              </div>
              <p class="text-[11px] text-slate-400">The root orchestrator folder where tasks, tools, and symlink mounts reside.</p>
            </div>
          </div>

          <!-- Directory Auto-Scanner -->
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
                placeholder="Parent folder path to discover repos..."
                class="flex-1 px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-slate-300 font-mono text-xs focus:outline-none focus:border-emerald-500"
              />
              <button
                type="button"
                @click="openDirectoryPicker('create_scan', null, scanPath, 'Select Parent Folder to Scan', 'Select a directory to automatically detect child repositories and service manifests')"
                class="px-3 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                Browse
              </button>
              <button
                @click="handleScanDirectory"
                :disabled="isScanning"
                class="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isScanning }" />
                Scan Repos
              </button>
            </div>

            <div v-if="scannedCandidates.length > 0" class="space-y-2 pt-2 border-t border-slate-800">
              <div class="text-[11px] font-mono text-emerald-400 flex items-center justify-between">
                <span>Discovered {{ scannedCandidates.length }} repositories:</span>
                <button @click="addScannedToCreateForm" class="text-xs underline text-emerald-400 hover:text-emerald-300 font-bold">
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

          <!-- Repositories List -->
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <h4 class="font-bold text-slate-200 uppercase tracking-wider text-[11px] font-mono">
                Repositories to Mount in Workspace ({{ createFormRepos.length }})
              </h4>
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  @click="openDirectoryPicker('create_browse_add', null, '', 'Browse & Add Service Repository', 'Choose a service folder from host disk to add it directly')"
                  class="px-2.5 py-1 rounded bg-emerald-600/90 hover:bg-emerald-600 text-white text-xs font-semibold flex items-center gap-1 transition shadow-sm"
                >
                  <FolderOpen class="w-3.5 h-3.5" />
                  Browse & Add Repo
                </button>
                <button
                  type="button"
                  @click="createFormRepos.push({ name: `service-${createFormRepos.length + 1}`, path: '', role: 'backend', manifest: 'auto' })"
                  class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-emerald-400 text-xs font-semibold flex items-center gap-1 transition"
                >
                  <Plus class="w-3.5 h-3.5" />
                  Add Row
                </button>
              </div>
            </div>

            <div class="space-y-2.5">
              <div
                v-for="(repo, idx) in createFormRepos"
                :key="idx"
                class="p-3.5 rounded-xl bg-slate-950 border border-slate-800/80 space-y-2.5"
              >
                <div class="grid grid-cols-1 md:grid-cols-12 gap-2.5 items-center">
                  <div class="md:col-span-3">
                    <label class="text-[10px] text-slate-400 font-mono">Service Name</label>
                    <input
                      v-model="repo.name"
                      type="text"
                      class="w-full px-2.5 py-1.5 rounded bg-slate-900 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
                    />
                  </div>

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

                  <div class="md:col-span-5">
                    <label class="text-[10px] text-slate-400 font-mono">Host Source Path</label>
                    <div class="flex gap-1.5 mt-0.5">
                      <input
                        v-model="repo.path"
                        type="text"
                        placeholder="/path/to/source"
                        class="flex-1 px-2.5 py-1.5 rounded bg-slate-900 border border-slate-800 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
                      />
                      <button
                        type="button"
                        @click="openDirectoryPicker('create_repo', idx, repo.path, `Select Source Folder for ${repo.name}`, 'Pick the source code repository folder on your machine')"
                        class="px-2.5 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
                        title="Browse filesystem for this repository"
                      >
                        <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                        Browse
                      </button>
                    </div>
                  </div>

                  <div class="md:col-span-1 flex justify-end pt-3">
                    <button
                      @click="createFormRepos.splice(idx, 1)"
                      class="p-1.5 rounded text-slate-400 hover:text-rose-400 hover:bg-slate-900 transition"
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>
                </div>

                <div class="flex items-center justify-between text-[10px] font-mono text-slate-400 px-1 pt-1.5 border-t border-slate-800/40">
                  <div class="truncate">
                    <span>Workspace Mount: </span>
                    <span class="text-emerald-400">{{ createFormRootDir || 'workspaces/<project-id>' }}/{{ repo.name || 'service' }}</span>
                  </div>
                  <div v-if="repo.manifest && repo.manifest !== 'auto'" class="flex-shrink-0">
                    <span>Manifest: </span>
                    <span class="text-slate-300">{{ repo.manifest }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

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

    <!-- Interactive Directory Picker Modal -->
    <DirectoryPickerModal
      :is-open="isDirectoryPickerOpen"
      :initial-path="pickerInitialPath"
      :title="pickerTitle"
      :helper-text="pickerHelperText"
      :initial-create-folder="pickerInitialCreateFolder"
      @select="handleDirectorySelected"
      @close="isDirectoryPickerOpen = false"
    />

    <!-- Confirmation Dialog Modal for Project and Service Deletions -->
    <ConfirmDeleteModal
      :is-open="isConfirmDeleteOpen"
      :title="confirmDeleteTitle"
      :message="confirmDeleteMessage"
      :item-name="confirmDeleteItemName"
      :note="confirmDeleteNote"
      :confirm-text="confirmDeleteBtnText"
      :loading="isConfirmDeleteLoading"
      @confirm="handleExecuteConfirmDelete"
      @close="isConfirmDeleteOpen = false"
    />
  </div>
</template>
