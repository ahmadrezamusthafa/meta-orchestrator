<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useProjectStore } from '../stores/projects'
import { useToastStore } from '../stores/toast'
import type { ProjectRole, ProjectRepo, Project, DirectoryItem } from '../types'
import {
  FolderGit2,
  FolderPlus,
  RefreshCw,
  Trash2,
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
  ArrowLeft,
  ShieldCheck,
  X,
  Play,
  GitBranch,
  Network,
  Eye,
  Terminal,
  Grid,
  FolderOpen,
  HelpCircle,
  ChevronDown,
  ChevronUp
} from 'lucide-vue-next'
import DirectoryPickerModal from '../components/common/DirectoryPickerModal.vue'
import ConfirmDeleteModal from '../components/common/ConfirmDeleteModal.vue'

const router = useRouter()
const projectStore = useProjectStore()
const toast = useToastStore()

// Navigation & Screen View: 'list' (default) vs 'edit' (detail & edit mode)
const currentView = ref<'list' | 'edit'>('list')
const projectSearchQuery = ref('')

// Dirty Tracking & Auto-Save State
const isDirty = ref(false)
const isSaving = ref(false)

function markDirty() {
  isDirty.value = true
}

// Modal States
const isCreateModalOpen = ref(false)
const isDetailDrawerOpen = ref(false)
const selectedRepoDetail = ref<ProjectRepo | null>(null)
const isScannerOpen = ref(false)

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

// View Toggle in Edit Mode: "grid" vs "topology"
const viewMode = ref<'grid' | 'topology'>('grid')

// Search & Filtering inside active project
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

// Edit Form State (active in edit mode)
const editFormId = ref('')
const editFormName = ref('')
const editFormDesc = ref('')
const editFormRootDir = ref('')
const editFormActiveSDLC = ref('general-ai-sdlc')
const editFormRepos = ref<Array<{ id?: string; name: string; path: string; role: ProjectRole; manifest: string; status?: string; symlink_path?: string; git_branch?: string; error?: string }>>([])

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
  | null
>(null)
const pickerRepoIndex = ref<number | null>(null)

function openDirectoryPicker(
  target:
    | 'create_root'
    | 'create_scan'
    | 'create_repo'
    | 'create_browse_add'
    | 'edit_root'
    | 'edit_scan'
    | 'edit_repo'
    | 'edit_browse_add',
  repoIndex: number | null = null,
  initialPath: string = '',
  title: string = 'Select Directory',
  helper: string = 'Choose a folder from your host filesystem'
) {
  pickerTarget.value = target
  pickerRepoIndex.value = repoIndex
  pickerInitialPath.value = initialPath
  pickerTitle.value = title
  pickerHelperText.value = helper
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
      markDirty()
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
        markDirty()
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
        manifest: directoryItem?.manifest || 'auto',
        status: 'pending'
      })
      markDirty()
      toast.success('Repository Added', `Added ${name} to project from filesystem`)
      break
    }
  }
}

onMounted(async () => {
  await projectStore.fetchProjects()
})

// Metrics for any project (used in list view cards)
function getProjectMetrics(p: Project) {
  const repos = p.repos || []
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
}

// Filtered Projects for List View
const filteredProjects = computed(() => {
  if (!projectStore.projects) return []
  if (!projectSearchQuery.value.trim()) return projectStore.projects
  const q = projectSearchQuery.value.toLowerCase().trim()
  return projectStore.projects.filter(
    (p) =>
      p.name.toLowerCase().includes(q) ||
      p.id.toLowerCase().includes(q) ||
      (p.description && p.description.toLowerCase().includes(q)) ||
      (p.root_dir && p.root_dir.toLowerCase().includes(q))
  )
})

// Open a project directly in Edit Mode
function openProjectEdit(projectId: string) {
  projectStore.selectProject(projectId)
  const p = projectStore.projects.find((proj) => proj.id === projectId)
  if (!p) return

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
    manifest: r.manifest_type || 'auto',
    status: r.status,
    symlink_path: r.symlink_path,
    git_branch: r.git_branch,
    error: r.error
  }))
  scannedCandidates.value = []
  scanPath.value = ''
  isScannerOpen.value = false
  isDirty.value = false
  currentView.value = 'edit'
}

// Auto-save logic
async function saveProjectChanges(showToast = true): Promise<boolean> {
  if (!editFormName.value.trim()) {
    toast.error('Validation Error', 'Project name cannot be empty')
    return false
  }

  if (editFormRepos.value.length === 0) {
    toast.error('Validation Error', 'Project must have at least one repository')
    return false
  }

  isSaving.value = true
  try {
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

    await projectStore.updateProject(editFormId.value, payload)
    isDirty.value = false
    if (showToast) {
      toast.success('Changes Saved', `Saved project "${editFormName.value}" and refreshed symlinks`)
    }
    return true
  } catch (err: any) {
    toast.error('Save Failed', err.message || 'Could not save project changes')
    return false
  } finally {
    isSaving.value = false
  }
}

// Handle Back button in Edit Mode: auto-saves if dirty and returns to list view
async function handleBack() {
  if (isDirty.value) {
    const ok = await saveProjectChanges(true)
    if (!ok) return // Validation failed, keep user in edit mode to fix
  }
  currentView.value = 'list'
}

// Handle Done button in Edit Mode: auto-saves if dirty and returns to list view
async function handleDone() {
  if (isDirty.value) {
    const ok = await saveProjectChanges(true)
    if (!ok) return
  }
  currentView.value = 'list'
}

// Filtered Repos for Matrix & Topology in Edit Mode
const filteredRepos = computed(() => {
  if (!editFormRepos.value) return []
  let list = editFormRepos.value

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
        (r.manifest && r.manifest.toLowerCase().includes(q))
    )
  }

  return list
})

// Metrics & Health Summaries for Active Edit Form
const editProjectMetrics = computed(() => {
  const repos = editFormRepos.value || []
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
        status: 'pending'
      })
    }
  }
  scannedCandidates.value = []
  markDirty()
  toast.info('Repositories Added', `Added ${selected.length} repositories to project.`)
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
    const created = await projectStore.createProject(payload)
    isCreateModalOpen.value = false
    openProjectEdit(created.id)
  } catch (err: any) {
    // Handled by store
  }
}

function addEditRepoRow() {
  editFormRepos.value.push({
    name: `service-${editFormRepos.value.length + 1}`,
    path: '',
    role: 'backend',
    manifest: 'auto',
    status: 'pending',
  })
  markDirty()
}

function removeEditRepoRow(idx: number) {
  const repo = editFormRepos.value[idx]
  if (!repo) return
  if (!repo.name && !repo.path) {
    editFormRepos.value.splice(idx, 1)
    markDirty()
    return
  }
  triggerConfirmDelete({
    title: 'Remove Repository from Project',
    message: `Are you sure you want to remove "${repo.name || 'this service'}" from the project setup? Upon saving, its workspace symlink will be pruned.`,
    itemName: repo.name || 'Unnamed Service',
    confirmText: 'Remove Row',
    onConfirm: async () => {
      editFormRepos.value.splice(idx, 1)
      markDirty()
    }
  })
}

// Detail Drawer Functions
function openRepoDetail(repo: any) {
  selectedRepoDetail.value = { ...repo }
  isDetailDrawerOpen.value = true
}

async function handleSaveRepoRoleChange(newRole: ProjectRole) {
  if (!selectedRepoDetail.value) return
  selectedRepoDetail.value.role = newRole
  const item = editFormRepos.value.find((r) => r.name === selectedRepoDetail.value?.name)
  if (item) {
    item.role = newRole
  }
  markDirty()
  await saveProjectChanges(false)
  toast.info('Role Updated', `Changed role of ${selectedRepoDetail.value.name} to ${newRole}`)
}

function handleRemoveRepoFromDrawer() {
  if (!selectedRepoDetail.value) return
  const repoName = selectedRepoDetail.value.name

  triggerConfirmDelete({
    title: 'Unregister Service Repository',
    message: `Are you sure you want to unregister "${repoName}" from this project? Its workspace symlink will be removed.`,
    itemName: repoName,
    note: 'Your source code on host disk remains completely safe and untouched.',
    confirmText: 'Unregister Service',
    onConfirm: async () => {
      editFormRepos.value = editFormRepos.value.filter((r) => r.name !== repoName)
      markDirty()
      isDetailDrawerOpen.value = false
      selectedRepoDetail.value = null
      await saveProjectChanges(false)
      toast.info('Repository Removed', `Unlinked ${repoName} from project workspace`)
    }
  })
}

async function handleResync() {
  if (!editFormId.value) return
  if (isDirty.value) {
    await saveProjectChanges(false)
  }
  await projectStore.resyncProject(editFormId.value)
  const updated = projectStore.projects.find((p) => p.id === editFormId.value)
  if (updated) {
    editFormRepos.value = (updated.repos || []).map((r) => ({
      id: r.id,
      name: r.name,
      path: r.path,
      role: r.role,
      manifest: r.manifest_type || 'auto',
      status: r.status,
      symlink_path: r.symlink_path,
      git_branch: r.git_branch,
      error: r.error,
    }))
  }
}

function handleDeleteProject(id: string) {
  const proj = projectStore.projects.find((p) => p.id === id) || (editFormId.value === id ? { name: editFormName.value } : null)
  const name = proj?.name || id

  triggerConfirmDelete({
    title: 'Delete Project & Workspace',
    message: `Are you sure you want to delete project "${name}" (${id})? All ephemeral symlinks and workspace configurations will be purged.`,
    itemName: name,
    note: 'Your original repositories on host disk are NOT modified or deleted.',
    confirmText: 'Yes, Delete Project',
    onConfirm: async () => {
      await projectStore.deleteProject(id)
      isDirty.value = false
      currentView.value = 'list'
    }
  })
}

function copyRootDir() {
  if (!editFormRootDir.value) return
  navigator.clipboard.writeText(editFormRootDir.value)
  copiedPath.value = true
  setTimeout(() => { copiedPath.value = false }, 2000)
  toast.info('Copied', 'Project root directory copied to clipboard')
}

function copyCliCommand() {
  if (!editFormRootDir.value) return
  const cmd = `cd ${editFormRootDir.value} && ls -la`
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
  router.push({ path: '/', query: { project: editFormId.value } })
}
</script>

<template>
  <div class="h-full w-full flex flex-col bg-slate-950 text-slate-100 overflow-y-auto">
    <!-- ========================================== -->
    <!-- VIEW 1: PROJECTS LIST VIEW (DEFAULT)       -->
    <!-- ========================================== -->
    <template v-if="currentView === 'list'">
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
                Click any project card to view and edit its workspace topology. Changes are automatically saved.
              </p>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2.5">
          <button
            @click="projectStore.fetchProjects"
            :disabled="projectStore.isLoading"
            class="p-2 rounded-lg border border-slate-700 bg-slate-800/80 hover:bg-slate-700 text-slate-300 hover:text-white transition"
            title="Refresh projects"
          >
            <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': projectStore.isLoading }" />
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
        <!-- Architectural Explainer Card -->
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
                Browse & select with 1-click dialog.
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
                Create new workspace folder right in browse dialog.
              </div>
            </div>
          </div>
        </div>

        <!-- Project Grid Section -->
        <section class="space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h2 class="text-sm font-bold uppercase tracking-wider text-slate-300 font-mono flex items-center gap-2">
                <span>Registered Projects</span>
                <span class="px-2 py-0.5 rounded-full bg-slate-800 text-emerald-400 text-xs">{{ filteredProjects.length }}</span>
              </h2>
              <p class="text-xs text-slate-400 mt-0.5">Click any card to enter edit mode and customize services or workspace paths.</p>
            </div>

            <!-- Search input -->
            <div class="relative w-full sm:w-72">
              <Search class="w-3.5 h-3.5 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                v-model="projectSearchQuery"
                type="text"
                placeholder="Search projects by name, ID, path..."
                class="w-full pl-9 pr-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-200 text-xs font-mono placeholder-slate-500 focus:outline-none focus:border-emerald-500"
              />
            </div>
          </div>

          <!-- Empty State -->
          <div
            v-if="filteredProjects.length === 0"
            class="py-16 px-4 rounded-2xl border border-slate-800 bg-slate-900/40 text-center space-y-3"
          >
            <FolderGit2 class="w-10 h-10 text-slate-500 mx-auto" />
            <h3 class="text-sm font-bold text-white">No Projects Found</h3>
            <p class="text-xs text-slate-400 max-w-sm mx-auto">
              {{ projectSearchQuery ? 'No registered project matches your search query.' : 'Register your first multi-repo project to organize your microservices.' }}
            </p>
            <button
              @click="openCreateModal"
              class="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-xs font-semibold text-white inline-flex items-center gap-2 shadow"
            >
              <FolderPlus class="w-4 h-4" />
              Register New Project
            </button>
          </div>

          <!-- Project Cards Grid -->
          <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
            <div
              v-for="p in filteredProjects"
              :key="p.id"
              @click="openProjectEdit(p.id)"
              class="p-5 rounded-2xl border border-slate-800 bg-slate-900/40 hover:bg-slate-900/80 hover:border-emerald-500/60 shadow-lg hover:shadow-emerald-950/30 transition-all duration-200 cursor-pointer flex flex-col justify-between group relative overflow-hidden"
            >
              <div class="space-y-3">
                <div class="flex items-start justify-between gap-2">
                  <div class="min-w-0 flex-1">
                    <h3 class="font-bold text-base text-white group-hover:text-emerald-400 transition truncate flex items-center gap-1.5">
                      {{ p.name }}
                    </h3>
                    <span class="text-[11px] font-mono text-slate-400">{{ p.id }}</span>
                  </div>

                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-semibold uppercase tracking-wider font-mono border flex-shrink-0"
                    :class="getProjectMetrics(p).isHealthy ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border-rose-500/20'"
                  >
                    {{ getProjectMetrics(p).isHealthy ? '● All Symlinks Healthy' : `● ${getProjectMetrics(p).brokenCount} Broken Link(s)` }}
                  </span>
                </div>

                <p class="text-xs text-slate-400 line-clamp-2 leading-relaxed">
                  {{ p.description || 'No project description provided.' }}
                </p>

                <!-- Role distribution badges -->
                <div class="flex flex-wrap items-center gap-1.5 pt-1">
                  <span
                    v-if="getProjectMetrics(p).roleCounts.frontend > 0"
                    class="px-2 py-0.5 rounded bg-sky-500/10 text-sky-400 border border-sky-500/20 text-[10px] font-medium flex items-center gap-1"
                  >
                    <Layout class="w-3 h-3" /> {{ getProjectMetrics(p).roleCounts.frontend }} Frontend
                  </span>
                  <span
                    v-if="getProjectMetrics(p).roleCounts.backend > 0"
                    class="px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 text-[10px] font-medium flex items-center gap-1"
                  >
                    <Server class="w-3 h-3" /> {{ getProjectMetrics(p).roleCounts.backend }} Backend
                  </span>
                  <span
                    v-if="getProjectMetrics(p).roleCounts['automation-test'] > 0"
                    class="px-2 py-0.5 rounded bg-purple-500/10 text-purple-400 border border-purple-500/20 text-[10px] font-medium flex items-center gap-1"
                  >
                    <FlaskConical class="w-3 h-3" /> {{ getProjectMetrics(p).roleCounts['automation-test'] }} Tests
                  </span>
                  <span
                    v-if="getProjectMetrics(p).roleCounts.contracts > 0"
                    class="px-2 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 text-[10px] font-medium flex items-center gap-1"
                  >
                    <FileCode2 class="w-3 h-3" /> {{ getProjectMetrics(p).roleCounts.contracts }} Contracts
                  </span>
                  <span
                    v-if="getProjectMetrics(p).roleCounts.artifact > 0"
                    class="px-2 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20 text-[10px] font-medium flex items-center gap-1"
                  >
                    <FolderArchive class="w-3 h-3" /> {{ getProjectMetrics(p).roleCounts.artifact }} Docs
                  </span>
                  <span
                    v-if="p.repos?.length === 0"
                    class="text-[10px] font-mono text-slate-500 italic"
                  >
                    No services attached
                  </span>
                </div>

                <!-- Workspace Root path -->
                <div class="p-2 rounded-lg bg-slate-950/70 border border-slate-800 text-[11px] font-mono text-slate-300 flex items-center gap-2 truncate">
                  <FolderOpen class="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" />
                  <span class="truncate text-emerald-300">{{ p.root_dir }}</span>
                </div>
              </div>

              <!-- Card Footer -->
              <div class="mt-4 pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs">
                <span class="text-emerald-400 font-semibold text-xs flex items-center gap-1 group-hover:underline">
                  <span>Open & Edit Project</span>
                  <ArrowRight class="w-3.5 h-3.5 group-hover:translate-x-1 transition" />
                </span>

                <div class="flex items-center gap-1.5" @click.stop>
                  <button
                    @click="router.push({ path: '/', query: { project: p.id } })"
                    class="p-1.5 rounded-lg bg-slate-800 hover:bg-emerald-600/30 text-slate-300 hover:text-emerald-300 border border-slate-700/60 transition"
                    title="Launch new task in this project"
                  >
                    <Play class="w-3.5 h-3.5 fill-current" />
                  </button>
                  <button
                    @click="projectStore.resyncProject(p.id)"
                    class="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/60 transition"
                    title="Resync symlinks"
                  >
                    <RefreshCw class="w-3.5 h-3.5" />
                  </button>
                  <button
                    @click="handleDeleteProject(p.id)"
                    class="p-1.5 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 transition"
                    title="Delete project"
                  >
                    <Trash2 class="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </template>

    <!-- ========================================== -->
    <!-- VIEW 2: PROJECT DETAIL & EDIT MODE         -->
    <!-- ========================================== -->
    <template v-else-if="currentView === 'edit'">
      <!-- Edit Mode Top Bar -->
      <header class="p-4 sm:p-6 border-b border-slate-800 bg-slate-900/60 backdrop-blur sticky top-0 z-30 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <button
            @click="handleBack"
            class="px-3 py-1.5 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold flex items-center gap-2 transition shadow-sm"
            title="Return to projects list (auto-saves any changes)"
          >
            <ArrowLeft class="w-4 h-4 text-emerald-400" />
            <span>Back to Projects</span>
          </button>

          <div class="h-5 w-px bg-slate-800"></div>

          <div>
            <div class="flex items-center gap-2">
              <h1 class="text-base sm:text-lg font-bold text-white flex items-center gap-2 truncate">
                {{ editFormName || 'Untitled Project' }}
                <span class="text-xs px-2 py-0.5 rounded bg-slate-800 text-slate-300 font-mono">
                  {{ editFormId }}
                </span>
              </h1>
            </div>

            <!-- Auto-save state indicator -->
            <div class="flex items-center gap-2 mt-0.5">
              <span v-if="isSaving" class="text-[11px] text-emerald-400 font-mono flex items-center gap-1.5">
                <RefreshCw class="w-3 h-3 animate-spin" />
                <span>Auto-saving changes...</span>
              </span>
              <span v-else-if="isDirty" class="text-[11px] text-amber-400 font-mono flex items-center gap-1.5">
                <span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
                <span>Unsaved changes (auto-saves on Back or Done)</span>
              </span>
              <span v-else class="text-[11px] text-slate-400 font-mono flex items-center gap-1.5">
                <Check class="w-3.5 h-3.5 text-emerald-400" />
                <span>All changes saved</span>
              </span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2.5">
          <button
            @click="handleResync"
            :disabled="projectStore.isLoading || isSaving"
            class="px-3 py-1.5 rounded-lg border border-slate-700 bg-slate-800/80 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-1.5 transition"
            title="Save and re-verify symlinks on disk"
          >
            <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': projectStore.isLoading }" />
            <span>Resync Symlinks</span>
          </button>

          <button
            @click="launchTaskForProject"
            class="px-3.5 py-1.5 rounded-lg bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-400 border border-emerald-500/30 text-xs font-semibold flex items-center gap-1.5 transition"
            title="Launch an AI task in this project"
          >
            <Play class="w-3.5 h-3.5 fill-current" />
            <span>Launch Task</span>
          </button>

          <button
            @click="handleDeleteProject(editFormId)"
            class="p-1.5 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 text-xs transition"
            title="Delete this project and workspace"
          >
            <Trash2 class="w-4 h-4" />
          </button>

          <!-- Done Button: auto-saves and returns to list view -->
          <button
            @click="handleDone"
            :disabled="isSaving"
            class="px-4 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-xs font-semibold text-white flex items-center gap-1.5 shadow-lg shadow-emerald-950/50 transition ml-1"
          >
            <Check class="w-4 h-4" />
            <span>Done</span>
          </button>
        </div>
      </header>

      <div class="p-6 space-y-6 max-w-7xl mx-auto w-full">
        <!-- Section 1: Project Settings Card -->
        <div class="p-6 rounded-2xl bg-gradient-to-r from-slate-900 via-slate-900/95 to-slate-950 border border-slate-800 space-y-5 shadow-lg">
          <div class="flex items-center justify-between border-b border-slate-800 pb-3">
            <div class="flex items-center gap-2">
              <Sliders class="w-4 h-4 text-emerald-400" />
              <h2 class="text-sm font-bold uppercase tracking-wider text-slate-200 font-mono">
                Project Configuration
              </h2>
            </div>
            <span class="text-xs text-slate-400">Edits are auto-saved on Back / Done</span>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs">
            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Project Name *</label>
              <input
                v-model="editFormName"
                @input="markDirty"
                type="text"
                placeholder="e.g. Fintech Payment Gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500 font-medium"
              />
            </div>

            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Active SDLC Workflow</label>
              <select
                v-model="editFormActiveSDLC"
                @change="markDirty"
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
                @input="markDirty"
                type="text"
                placeholder="Description of multi-repo architecture, target domain, and testing criteria..."
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500"
              />
            </div>

            <!-- Unified Workspace Directory with single Browse button (folder creation inside browse dialog) -->
            <div class="space-y-1.5 md:col-span-2">
              <label class="font-semibold text-slate-200">Unified Workspace Directory (Root Target)</label>
              <div class="flex gap-2">
                <input
                  v-model="editFormRootDir"
                  @input="markDirty"
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

          <!-- Role & Health Overview Chips -->
          <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2.5 pt-3 border-t border-slate-800/80">
            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-sky-400 font-medium">
                <Layout class="w-3.5 h-3.5" /> Frontend
              </span>
              <span class="font-mono font-bold text-white">{{ editProjectMetrics.roleCounts.frontend }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-emerald-400 font-medium">
                <Server class="w-3.5 h-3.5" /> Backend
              </span>
              <span class="font-mono font-bold text-white">{{ editProjectMetrics.roleCounts.backend }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-purple-400 font-medium">
                <FlaskConical class="w-3.5 h-3.5" /> Tests
              </span>
              <span class="font-mono font-bold text-white">{{ editProjectMetrics.roleCounts['automation-test'] }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-amber-400 font-medium">
                <FileCode2 class="w-3.5 h-3.5" /> Contracts
              </span>
              <span class="font-mono font-bold text-white">{{ editProjectMetrics.roleCounts.contracts }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-blue-400 font-medium">
                <FolderArchive class="w-3.5 h-3.5" /> Artifacts
              </span>
              <span class="font-mono font-bold text-white">{{ editProjectMetrics.roleCounts.artifact }}</span>
            </div>

            <div class="p-2.5 rounded-lg bg-slate-950/60 border border-slate-800/80 flex items-center justify-between text-xs">
              <span class="flex items-center gap-1.5 text-slate-400 font-medium">
                <Layers class="w-3.5 h-3.5" /> Total
              </span>
              <span class="font-mono font-bold text-white">{{ editProjectMetrics.total }}</span>
            </div>
          </div>

          <!-- Root Dir Box & Terminal Helper -->
          <div class="p-3.5 rounded-xl bg-slate-950/80 border border-slate-800/80 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs">
            <div class="flex items-center gap-2.5 min-w-0">
              <ShieldCheck class="w-4 h-4 text-emerald-400 flex-shrink-0" />
              <div class="truncate">
                <span class="text-slate-400 font-mono">Workspace Mount Path: </span>
                <span class="text-emerald-400 font-mono font-medium">{{ editFormRootDir }}</span>
              </div>
            </div>

            <div class="flex items-center gap-2 flex-shrink-0">
              <button
                @click="copyCliCommand"
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs flex items-center gap-1.5 transition"
                title="Copy cd command"
              >
                <Terminal class="w-3.5 h-3.5 text-sky-400" />
                <span>{{ copiedCliCmd ? 'Copied' : 'Copy CLI cd' }}</span>
              </button>

              <button
                @click="copyRootDir"
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs flex items-center gap-1.5 transition"
                title="Copy root path"
              >
                <Check v-if="copiedPath" class="w-3.5 h-3.5 text-emerald-400" />
                <Copy v-else class="w-3.5 h-3.5" />
                <span>{{ copiedPath ? 'Copied' : 'Copy Path' }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- Section 2: Registered Repositories & Services -->
        <section class="space-y-4">
          <!-- Toolbar -->
          <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
            <div class="flex items-center gap-3">
              <h3 class="text-sm font-bold uppercase tracking-wider text-slate-200 font-mono flex items-center gap-2">
                <span>Service Repositories</span>
                <span class="px-2 py-0.5 rounded-full bg-slate-800 text-emerald-400 text-xs">{{ editFormRepos.length }}</span>
              </h3>

              <!-- View Mode Toggle -->
              <div class="flex items-center bg-slate-900 p-0.5 rounded-lg border border-slate-800 text-xs">
                <button
                  @click="viewMode = 'grid'"
                  class="px-2.5 py-1 rounded flex items-center gap-1 font-medium transition"
                  :class="viewMode === 'grid' ? 'bg-slate-800 text-white shadow' : 'text-slate-400 hover:text-slate-200'"
                >
                  <Grid class="w-3.5 h-3.5" />
                  Services Table
                </button>
                <button
                  @click="viewMode = 'topology'"
                  class="px-2.5 py-1 rounded flex items-center gap-1 font-medium transition"
                  :class="viewMode === 'topology' ? 'bg-slate-800 text-white shadow' : 'text-slate-400 hover:text-slate-200'"
                >
                  <Network class="w-3.5 h-3.5 text-emerald-400" />
                  Topology Map
                </button>
              </div>
            </div>

            <!-- Action buttons: Add repo & Auto-scanner -->
            <div class="flex flex-wrap items-center gap-2">
              <button
                @click="isScannerOpen = !isScannerOpen"
                class="px-3 py-1.5 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-1.5 transition"
              >
                <Search class="w-3.5 h-3.5 text-emerald-400" />
                <span>{{ isScannerOpen ? 'Hide Auto-Scanner' : 'Auto-Scan Folder' }}</span>
              </button>

              <button
                @click="openDirectoryPicker('edit_browse_add', null, '', 'Select Repository to Add', 'Pick a repository codebase from your disk to add to this project')"
                class="px-3 py-1.5 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-1.5 transition"
              >
                <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                <span>Browse & Add</span>
              </button>

              <button
                @click="addEditRepoRow"
                class="px-3.5 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-xs font-semibold text-white flex items-center gap-1.5 shadow transition"
              >
                <Plus class="w-3.5 h-3.5" />
                <span>Add Service Row</span>
              </button>
            </div>
          </div>

          <!-- Collapsible Auto-Scanner Panel -->
          <div v-if="isScannerOpen" class="p-4 rounded-xl bg-slate-900/90 border border-slate-800 space-y-3 animate-in fade-in duration-150">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Search class="w-4 h-4 text-emerald-400" />
                <h4 class="font-bold text-slate-200 uppercase tracking-wider text-xs font-mono">Scan Local Directory for Services</h4>
              </div>
              <span class="text-[11px] text-slate-400">Detects package.json, go.mod, playwright, openapi and classifies roles</span>
            </div>

            <div class="flex gap-2 text-xs">
              <input
                v-model="scanPath"
                type="text"
                placeholder="Parent folder path to scan for child repositories..."
                class="flex-1 px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono focus:outline-none focus:border-emerald-500"
              />
              <button
                type="button"
                @click="openDirectoryPicker('edit_scan', null, scanPath, 'Select Parent Folder to Scan', 'Select a directory to automatically detect child repositories')"
                class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                Browse
              </button>
              <button
                @click="handleScanDirectory"
                :disabled="isScanning"
                class="px-4 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0"
              >
                <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isScanning }" />
                Scan
              </button>
            </div>

            <!-- Scanned Candidates -->
            <div v-if="scannedCandidates.length > 0" class="pt-2 border-t border-slate-800/80 space-y-2">
              <div class="flex items-center justify-between text-xs">
                <span class="font-semibold text-emerald-400">Discovered Repositories ({{ scannedCandidates.length }}):</span>
                <button
                  type="button"
                  @click="addScannedToEditForm"
                  class="px-3 py-1 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs flex items-center gap-1"
                >
                  <Plus class="w-3.5 h-3.5" />
                  Add Selected to Project
                </button>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-48 overflow-y-auto">
                <label
                  v-for="c in scannedCandidates"
                  :key="c.path"
                  class="flex items-center gap-2.5 p-2 rounded-lg bg-slate-950 border border-slate-800 cursor-pointer hover:border-slate-700 text-xs"
                >
                  <input type="checkbox" v-model="c.selected" class="rounded border-slate-700 text-emerald-500 focus:ring-0" />
                  <div class="truncate flex-1 font-mono">
                    <span class="font-bold text-white">{{ c.name }}</span>
                    <span class="text-[10px] text-slate-400 block truncate">{{ c.path }}</span>
                  </div>
                  <span class="text-[10px] uppercase font-bold px-1.5 py-0.5 rounded bg-slate-800 text-slate-300 font-mono flex-shrink-0">
                    {{ c.role }}
                  </span>
                </label>
              </div>
            </div>
          </div>

          <!-- VIEW MODE A: Services Rows/Cards Table -->
          <div v-if="viewMode === 'grid'" class="space-y-2.5">
            <div
              v-for="(repo, idx) in editFormRepos"
              :key="repo.id || idx"
              class="p-4 rounded-xl bg-slate-900/60 border border-slate-800 space-y-3 hover:border-slate-700 transition"
            >
              <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-3 text-xs">
                <!-- Repo Name & Role -->
                <div class="flex flex-wrap items-center gap-2.5 flex-1 min-w-0">
                  <div class="w-48">
                    <input
                      v-model="repo.name"
                      @input="markDirty"
                      type="text"
                      placeholder="Service name (e.g. backend-core)"
                      class="w-full px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-white font-mono text-xs focus:outline-none focus:border-emerald-500 font-bold"
                    />
                  </div>

                  <select
                    v-model="repo.role"
                    @change="markDirty"
                    class="px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 text-xs font-mono focus:outline-none focus:border-emerald-500"
                  >
                    <option value="frontend">🎨 Frontend UI</option>
                    <option value="backend">⚙️ Backend Service</option>
                    <option value="automation-test">🧪 Automation Test</option>
                    <option value="contracts">📜 API Contracts</option>
                    <option value="artifact">📁 Artifacts & PRD</option>
                    <option value="other">🔌 Other Service</option>
                  </select>

                  <select
                    v-model="repo.manifest"
                    @change="markDirty"
                    class="px-2 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-slate-300 text-[11px] font-mono focus:outline-none focus:border-emerald-500"
                  >
                    <option value="auto">Auto-detect Manifest</option>
                    <option value="package.json">package.json</option>
                    <option value="go.mod">go.mod</option>
                    <option value="playwright.config.ts">playwright.config.ts</option>
                    <option value="openapi.yaml">openapi.yaml</option>
                    <option value="composer.json">composer.json</option>
                    <option value="Cargo.toml">Cargo.toml</option>
                    <option value="requirements.txt">requirements.txt</option>
                  </select>
                </div>

                <!-- Status & Action buttons -->
                <div class="flex items-center gap-2 flex-shrink-0">
                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-mono uppercase font-semibold border"
                    :class="repo.status === 'linked' ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-amber-500/10 text-amber-400 border-amber-500/20'"
                  >
                    {{ repo.status || 'pending' }}
                  </span>

                  <button
                    type="button"
                    @click="openRepoDetail(repo)"
                    class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs flex items-center gap-1 transition"
                    title="View details & health"
                  >
                    <Eye class="w-3.5 h-3.5" />
                    <span>Inspect</span>
                  </button>

                  <button
                    type="button"
                    @click="removeEditRepoRow(idx)"
                    class="p-1.5 rounded bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 transition"
                    title="Remove repository"
                  >
                    <Trash2 class="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              <!-- Host Path with single Browse button -->
              <div class="flex items-center gap-2 text-xs">
                <span class="text-slate-400 font-mono text-[11px] flex-shrink-0">Host Source:</span>
                <input
                  v-model="repo.path"
                  @input="markDirty"
                  type="text"
                  placeholder="/Users/name/Projects/service-codebase"
                  class="flex-1 px-2.5 py-1 rounded bg-slate-950 border border-slate-800 text-slate-300 font-mono text-xs focus:outline-none focus:border-emerald-500"
                />
                <button
                  type="button"
                  @click="openDirectoryPicker('edit_repo', idx, repo.path, 'Select Repository Directory', 'Choose the local source repository folder on your machine')"
                  class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold flex items-center gap-1 transition flex-shrink-0"
                >
                  <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                  Browse
                </button>
              </div>

              <div v-if="repo.name && editFormRootDir" class="text-[11px] font-mono text-slate-400 flex items-center gap-2">
                <span>Mount Target:</span>
                <span class="text-emerald-400">{{ editFormRootDir }}/{{ repo.name }}</span>
              </div>
            </div>
          </div>

          <!-- VIEW MODE B: Topology Visualizer -->
          <div v-else class="p-6 rounded-2xl bg-slate-950 border border-slate-800 space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <h4 class="font-bold text-sm text-white flex items-center gap-2">
                  <Network class="w-4 h-4 text-emerald-400" />
                  Workspace Symlink & Topology Graph
                </h4>
                <p class="text-xs text-slate-400">Visual topology showing how distributed host repositories are mounted into the unified project workspace.</p>
              </div>
              <span class="text-xs font-mono text-slate-400">{{ editFormRepos.length }} Services Configured</span>
            </div>

            <div class="p-8 rounded-xl bg-slate-900/40 border border-slate-800/80 flex flex-col items-center justify-center space-y-8 relative overflow-hidden">
              <!-- Central Project Root Node -->
              <div class="relative z-10 flex flex-col items-center">
                <div class="px-5 py-3 rounded-xl bg-emerald-950/80 border-2 border-emerald-500 shadow-xl shadow-emerald-950/50 flex items-center gap-3 text-emerald-300 font-mono text-xs font-bold">
                  <FolderGit2 class="w-5 h-5 text-emerald-400" />
                  <div>
                    <div class="flex items-center gap-2">
                      <span>{{ editFormName || 'Unified Workspace' }}</span>
                      <span class="text-[9px] px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-mono">Workspace Root</span>
                    </div>
                    <div class="text-[10px] text-emerald-400/70 font-normal mt-0.5">{{ editFormRootDir }}</div>
                  </div>
                </div>
                <div class="w-0.5 h-8 bg-gradient-to-b from-emerald-500 to-slate-700"></div>
              </div>

              <!-- Radiating Service Nodes -->
              <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 w-full max-w-4xl relative z-10">
                <div
                  v-for="repo in editFormRepos"
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
                      :class="repo.status === 'linked' ? 'bg-emerald-400' : 'bg-amber-400'"
                      :title="repo.status || 'pending'"
                    ></span>
                  </div>

                  <div class="mt-2 text-[10px] font-mono text-slate-400 space-y-0.5 truncate">
                    <div class="truncate text-emerald-400">Mount: {{ editFormRootDir }}/{{ repo.name }}</div>
                    <div class="truncate text-slate-400">Source: {{ repo.path || 'No path configured' }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </template>

    <!-- ========================================== -->
    <!-- SLIDE-OVER DRAWER: REPO DETAIL & HEALTH    -->
    <!-- ========================================== -->
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
          :class="selectedRepoDetail.status === 'linked' ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300' : 'bg-amber-500/10 border-amber-500/30 text-amber-300'"
        >
          <div class="flex items-center gap-2">
            <CheckCircle2 v-if="selectedRepoDetail.status === 'linked'" class="w-4 h-4 text-emerald-400" />
            <AlertTriangle v-else class="w-4 h-4 text-amber-400" />
            <span>{{ selectedRepoDetail.status === 'linked' ? 'Symlink Active & Resolving' : 'Symlink Pending or Path Unverified' }}</span>
          </div>
          <span class="uppercase text-[10px] font-bold px-2 py-0.5 rounded bg-black/30 border border-current">
            {{ selectedRepoDetail.status || 'pending' }}
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
          <div>
            <span class="text-slate-400 block mb-1 text-[11px]">Host Source Path (Disk):</span>
            <div class="p-2.5 rounded bg-slate-950 border border-slate-800 text-slate-200 break-all select-all">
              {{ selectedRepoDetail.path }}
            </div>
          </div>

          <div>
            <span class="text-slate-400 block mb-1 text-[11px]">Unified Workspace Mount:</span>
            <div class="p-2.5 rounded bg-slate-950 border border-slate-800 text-emerald-400 break-all select-all">
              {{ editFormRootDir }}/{{ selectedRepoDetail.name }}
            </div>
          </div>

          <div v-if="selectedRepoDetail.git_branch">
            <span class="text-slate-400 block mb-1 text-[11px]">Active Git Branch:</span>
            <div class="flex items-center gap-2 p-2 rounded bg-slate-950 border border-slate-800 text-slate-200">
              <GitBranch class="w-3.5 h-3.5 text-emerald-400" />
              <span>{{ selectedRepoDetail.git_branch }}</span>
            </div>
          </div>
        </div>

        <div class="pt-4 border-t border-slate-800 flex items-center justify-between">
          <button
            @click="handleRemoveRepoFromDrawer"
            class="px-3 py-2 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/30 text-xs font-semibold flex items-center gap-1.5 transition"
          >
            <Trash2 class="w-3.5 h-3.5" />
            Unregister Service
          </button>
          <button
            @click="isDetailDrawerOpen = false"
            class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-semibold"
          >
            Close
          </button>
        </div>
      </div>
    </div>

    <!-- ========================================== -->
    <!-- CREATE PROJECT MODAL                       -->
    <!-- ========================================== -->
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
              <h3 class="font-bold text-base text-white">Register Unified Multi-Repo Project</h3>
              <p class="text-xs text-slate-400">Map multiple independent git repositories into a synchronized workspace.</p>
            </div>
          </div>
          <button @click="isCreateModalOpen = false" class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition">
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
                v-model="createFormName"
                @input="onCreateNameInput"
                type="text"
                placeholder="e.g. Fintech Payment Gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500 font-medium"
              />
            </div>

            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Project Identifier (Slug)</label>
              <input
                v-model="createFormId"
                type="text"
                placeholder="fintech-payment-gateway"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-400 font-mono text-xs focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Active SDLC Workflow</label>
              <select
                v-model="createFormActiveSDLC"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 font-mono focus:outline-none focus:border-emerald-500"
              >
                <option value="general-ai-sdlc">General AI SDLC (9 Stages)</option>
                <option value="microservice-api">Microservice API Contract Workflow (4 Stages)</option>
                <option value="hotfix-fast-track">Hotfix Fast-Track (3 Stages)</option>
              </select>
            </div>

            <div class="space-y-1.5">
              <label class="font-semibold text-slate-200">Description</label>
              <input
                v-model="createFormDesc"
                type="text"
                placeholder="Multi-repo microservice architecture connecting Vue frontend, Go API, and Playwright tests"
                class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-slate-200 focus:outline-none focus:border-emerald-500"
              />
            </div>

            <!-- Unified Workspace Directory with single Browse button -->
            <div class="space-y-1.5 md:col-span-2">
              <label class="font-semibold text-slate-200">Unified Workspace Directory (Root Target)</label>
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
                placeholder="Parent folder path (e.g. /Users/.../my-repos)"
                class="flex-1 px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-slate-300 font-mono text-xs focus:outline-none focus:border-emerald-500"
              />
              <button
                type="button"
                @click="openDirectoryPicker('create_scan', null, scanPath, 'Select Parent Folder to Scan', 'Select a directory to automatically detect child repositories')"
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

            <div v-if="scannedCandidates.length > 0" class="pt-2 border-t border-slate-800/80 space-y-2">
              <div class="flex items-center justify-between text-xs">
                <span class="font-semibold text-emerald-400">Discovered Services ({{ scannedCandidates.length }}):</span>
                <button
                  type="button"
                  @click="addScannedToCreateForm"
                  class="px-3 py-1 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs flex items-center gap-1"
                >
                  <Plus class="w-3.5 h-3.5" />
                  Add Selected
                </button>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-40 overflow-y-auto">
                <label
                  v-for="c in scannedCandidates"
                  :key="c.path"
                  class="flex items-center gap-2 p-2 rounded-lg bg-slate-900 border border-slate-800 cursor-pointer hover:border-slate-700 text-xs"
                >
                  <input type="checkbox" v-model="c.selected" class="rounded border-slate-700 text-emerald-500 focus:ring-0" />
                  <div class="truncate flex-1 font-mono">
                    <span class="font-bold text-white">{{ c.name }}</span>
                    <span class="text-[10px] text-slate-400 block truncate">{{ c.path }}</span>
                  </div>
                  <span class="text-[10px] uppercase font-bold px-1.5 py-0.5 rounded bg-slate-800 text-slate-300 font-mono flex-shrink-0">
                    {{ c.role }}
                  </span>
                </label>
              </div>
            </div>
          </div>

          <!-- Repos Table in Create Modal -->
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <label class="font-bold uppercase tracking-wider text-slate-300 text-xs font-mono">
                Component Repositories ({{ createFormRepos.length }})
              </label>
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  @click="openDirectoryPicker('create_browse_add', null, '', 'Select Repository to Add', 'Pick a repository codebase from your disk to add to this project')"
                  class="px-3 py-1.5 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs font-semibold text-slate-200 flex items-center gap-1.5 transition"
                >
                  <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                  Browse & Add
                </button>
                <button
                  type="button"
                  @click="createFormRepos.push({ name: `service-${createFormRepos.length + 1}`, path: '', role: 'backend', manifest: 'auto' })"
                  class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-semibold text-emerald-400 flex items-center gap-1.5 transition"
                >
                  <Plus class="w-3.5 h-3.5" />
                  Add Row
                </button>
              </div>
            </div>

            <div class="space-y-2">
              <div
                v-for="(repo, idx) in createFormRepos"
                :key="idx"
                class="p-3 rounded-xl bg-slate-950 border border-slate-800/80 space-y-2"
              >
                <div class="flex items-center gap-2">
                  <input
                    v-model="repo.name"
                    type="text"
                    placeholder="Repo name (e.g. frontend-portal)"
                    class="flex-1 px-2.5 py-1 rounded bg-slate-900 border border-slate-800 text-white font-mono text-xs focus:outline-none focus:border-emerald-500 font-medium"
                  />
                  <select
                    v-model="repo.role"
                    class="px-2.5 py-1 rounded bg-slate-900 border border-slate-800 text-slate-200 text-xs font-mono focus:outline-none focus:border-emerald-500"
                  >
                    <option value="frontend">🎨 Frontend</option>
                    <option value="backend">⚙️ Backend</option>
                    <option value="automation-test">🧪 Test</option>
                    <option value="contracts">📜 Contracts</option>
                    <option value="artifact">📁 Artifacts</option>
                    <option value="other">🔌 Other</option>
                  </select>
                  <button
                    type="button"
                    @click="createFormRepos.splice(idx, 1)"
                    class="p-1 rounded bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 transition"
                  >
                    <Trash2 class="w-3.5 h-3.5" />
                  </button>
                </div>

                <div class="flex items-center gap-2">
                  <input
                    v-model="repo.path"
                    type="text"
                    placeholder="/Users/.../absolute/path/to/repo"
                    class="flex-1 px-2.5 py-1 rounded bg-slate-900 border border-slate-800 text-slate-300 font-mono text-xs focus:outline-none focus:border-emerald-500"
                  />
                  <button
                    type="button"
                    @click="openDirectoryPicker('create_repo', idx, repo.path, 'Select Repository Directory', 'Choose the repository folder on your machine')"
                    class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold flex items-center gap-1 transition flex-shrink-0"
                  >
                    <FolderOpen class="w-3.5 h-3.5 text-emerald-400" />
                    Browse
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Modal Footer -->
        <div class="p-5 border-t border-slate-800 flex items-center justify-between bg-slate-900/90">
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

    <!-- ========================================== -->
    <!-- INTERACTIVE DIRECTORY PICKER MODAL         -->
    <!-- (Folder creation lives inside here)        -->
    <!-- ========================================== -->
    <DirectoryPickerModal
      :is-open="isDirectoryPickerOpen"
      :initial-path="pickerInitialPath"
      :title="pickerTitle"
      :helper-text="pickerHelperText"
      :can-create-folder="true"
      @select="handleDirectorySelected"
      @close="isDirectoryPickerOpen = false"
    />

    <!-- Confirmation Dialog Modal -->
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
