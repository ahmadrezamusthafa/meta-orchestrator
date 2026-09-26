<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { api } from '../../services/api'
import type { BrowseFSResponse, DirectoryItem } from '../../types'
import {
  Folder,
  FolderPlus,
  FolderGit2,
  FolderOpen,
  ArrowUp,
  Search,
  Check,
  X,
  Home,
  Briefcase,
  Layers,
  ChevronRight,
  RefreshCw,
  AlertCircle,
  CornerLeftUp,
  Layout,
  Server,
  FlaskConical,
  FileCode2,
  FolderArchive
} from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    isOpen: boolean
    initialPath?: string
    title?: string
    helperText?: string
    canCreateFolder?: boolean
    initialCreateFolder?: boolean
  }>(),
  {
    isOpen: false,
    initialPath: '',
    title: 'Select Directory',
    helperText: 'Navigate your host filesystem and choose a directory',
    canCreateFolder: true,
    initialCreateFolder: false
  }
)

const emit = defineEmits<{
  (e: 'select', path: string, directoryItem?: DirectoryItem): void
  (e: 'close'): void
}>()

const isLoading = ref(false)
const errorMsg = ref('')
const filterQuery = ref('')
const fsData = ref<BrowseFSResponse | null>(null)
const selectedPath = ref('')
const selectedItem = ref<DirectoryItem | null>(null)
const isManualEditing = ref(false)
const manualInputPath = ref('')

// New folder creation state
const isCreatingFolder = ref(false)
const newFolderName = ref('')
const isSubmittingFolder = ref(false)
const createFolderError = ref('')
const createFolderSuccess = ref('')
const newFolderInputRef = ref<HTMLInputElement | null>(null)

async function loadDirectory(path?: string) {
  isLoading.value = true
  errorMsg.value = ''
  try {
    const data = await api.browseDirectory(path)
    fsData.value = data
    selectedPath.value = data.current_path
    selectedItem.value = null
    manualInputPath.value = data.current_path
  } catch (err: any) {
    errorMsg.value = err.message || 'Failed to open directory'
  } finally {
    isLoading.value = false
  }
}

watch(
  () => props.isOpen,
  (open) => {
    if (open) {
      filterQuery.value = ''
      isManualEditing.value = false
      isCreatingFolder.value = false
      newFolderName.value = ''
      createFolderError.value = ''
      createFolderSuccess.value = ''
      loadDirectory(props.initialPath || undefined)
      if (props.initialCreateFolder) {
        openCreateFolder()
      }
    }
  },
  { immediate: true }
)

function openCreateFolder() {
  isCreatingFolder.value = true
  newFolderName.value = ''
  createFolderError.value = ''
  createFolderSuccess.value = ''
  setTimeout(() => {
    newFolderInputRef.value?.focus()
  }, 50)
}

function cancelCreateFolder() {
  isCreatingFolder.value = false
  newFolderName.value = ''
  createFolderError.value = ''
}

async function handleCreateFolder() {
  const name = newFolderName.value.trim()
  if (!name) {
    createFolderError.value = 'Please enter a folder name'
    return
  }
  if (name.includes('/') || name.includes('\\') || name === '..' || name === '.') {
    createFolderError.value = 'Folder name cannot contain slashes or relative path segments'
    return
  }

  const parent = fsData.value?.current_path
  if (!parent) {
    createFolderError.value = 'No directory currently active'
    return
  }

  isSubmittingFolder.value = true
  createFolderError.value = ''
  createFolderSuccess.value = ''

  try {
    const res = await api.createFolder(parent, name)
    createFolderSuccess.value = `Created folder "${res.name}"`
    isCreatingFolder.value = false
    newFolderName.value = ''

    // Reload directory to show the newly created folder
    await loadDirectory(parent)

    // Automatically select the new folder
    selectedPath.value = res.path
    const createdItem = fsData.value?.directories.find(
      (d) => d.name === res.name || d.path === res.path
    )
    if (createdItem) {
      selectedItem.value = createdItem
    }
  } catch (err: any) {
    createFolderError.value = err.message || 'Failed to create folder'
  } finally {
    isSubmittingFolder.value = false
  }
}


const filteredDirectories = computed(() => {
  if (!fsData.value?.directories) return []
  if (!filterQuery.value.trim()) return fsData.value.directories

  const q = filterQuery.value.toLowerCase().trim()
  return fsData.value.directories.filter(
    (d) =>
      d.name.toLowerCase().includes(q) ||
      (d.manifest && d.manifest.toLowerCase().includes(q)) ||
      (d.suggested_role && d.suggested_role.toLowerCase().includes(q))
  )
})

function handleBookmarkClick(bookmarkPath: string) {
  loadDirectory(bookmarkPath)
}

function handleBreadcrumbClick(path: string) {
  loadDirectory(path)
}

function handleGoUp() {
  if (fsData.value?.parent_path) {
    loadDirectory(fsData.value.parent_path)
  }
}

function handleDirectoryClick(item: DirectoryItem) {
  selectedPath.value = item.path
  selectedItem.value = item
}

function handleDirectoryDblClick(item: DirectoryItem) {
  loadDirectory(item.path)
}

function handleNavigateInto(item: DirectoryItem) {
  loadDirectory(item.path)
}

function handleManualSubmit() {
  if (manualInputPath.value.trim()) {
    isManualEditing.value = false
    loadDirectory(manualInputPath.value.trim())
  }
}

function handleConfirmSelection() {
  const chosenPath = selectedPath.value || fsData.value?.current_path || ''
  if (!chosenPath) return
  emit('select', chosenPath, selectedItem.value || undefined)
  emit('close')
}

function getBookmarkIcon(iconName: string) {
  switch (iconName) {
    case 'home':
      return Home
    case 'briefcase':
      return Briefcase
    case 'layers':
      return Layers
    case 'arrow-up':
      return ArrowUp
    default:
      return Folder
  }
}

function getRoleIcon(role: string) {
  switch (role) {
    case 'frontend':
      return Layout
    case 'backend':
      return Server
    case 'automation-test':
      return FlaskConical
    case 'contracts':
      return FileCode2
    case 'artifact':
      return FolderArchive
    default:
      return Folder
  }
}

function getRoleBadgeClass(role: string) {
  switch (role) {
    case 'frontend':
      return 'bg-sky-500/10 text-sky-400 border-sky-500/20'
    case 'backend':
      return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
    case 'automation-test':
      return 'bg-purple-500/10 text-purple-400 border-purple-500/20'
    case 'contracts':
      return 'bg-amber-500/10 text-amber-400 border-amber-500/20'
    case 'artifact':
      return 'bg-blue-500/10 text-blue-400 border-blue-500/20'
    default:
      return 'bg-slate-800 text-slate-400 border-slate-700'
  }
}
</script>

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 overflow-y-auto"
    @click.self="emit('close')"
  >
    <div
      class="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-3xl max-h-[85vh] flex flex-col shadow-2xl overflow-hidden my-4 ring-1 ring-slate-800/80 animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Modal Header -->
      <div class="p-4 border-b border-slate-800 flex items-center justify-between bg-slate-900/95">
        <div class="flex items-center gap-2.5">
          <div class="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
            <FolderOpen class="w-5 h-5" />
          </div>
          <div>
            <h2 class="font-bold text-sm text-white flex items-center gap-2">
              {{ title }}
              <span class="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-mono">
                Host Filesystem Browser
              </span>
            </h2>
            <p class="text-[11px] text-slate-400">{{ helperText }}</p>
          </div>
        </div>
        <button
          @click="emit('close')"
          class="text-slate-400 hover:text-white p-1.5 rounded-lg hover:bg-slate-800 transition"
          title="Close browser"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Quick Bookmarks Bar -->
      <div
        v-if="fsData?.quick_bookmarks && fsData.quick_bookmarks.length > 0"
        class="px-4 py-2 bg-slate-950/70 border-b border-slate-800/80 flex items-center gap-2 overflow-x-auto text-xs"
      >
        <span class="text-[10px] uppercase font-mono text-slate-400 font-semibold tracking-wider flex-shrink-0">
          Bookmarks:
        </span>
        <div class="flex items-center gap-1.5 flex-nowrap">
          <button
            v-for="bm in fsData.quick_bookmarks"
            :key="bm.path"
            @click="handleBookmarkClick(bm.path)"
            class="px-2.5 py-1 rounded-md bg-slate-900 hover:bg-slate-800 border border-slate-800 hover:border-slate-700 text-slate-300 hover:text-white text-[11px] font-mono flex items-center gap-1.5 transition flex-shrink-0"
            :class="{ 'border-emerald-500/40 text-emerald-400': fsData.current_path === bm.path }"
          >
            <component :is="getBookmarkIcon(bm.icon)" class="w-3.5 h-3.5 text-emerald-400" />
            <span>{{ bm.name }}</span>
          </button>
        </div>
      </div>

      <!-- Navigation & Breadcrumb Bar -->
      <div class="p-3 bg-slate-950/90 border-b border-slate-800 flex items-center gap-2">
        <button
          @click="handleGoUp"
          :disabled="!fsData?.parent_path || isLoading"
          class="p-1.5 rounded-lg bg-slate-900 border border-slate-800 hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed text-slate-300 hover:text-white transition flex-shrink-0"
          title="Go Up one directory"
        >
          <CornerLeftUp class="w-4 h-4" />
        </button>

        <!-- Breadcrumbs or Manual Edit -->
        <div class="flex-1 min-w-0">
          <div v-if="!isManualEditing" class="flex items-center gap-1 overflow-x-auto py-0.5 text-xs font-mono scrollbar-none">
            <template v-for="(bc, idx) in fsData?.breadcrumbs || []" :key="bc.path">
              <span v-if="idx > 0" class="text-slate-400 flex-shrink-0">/</span>
              <button
                @click="handleBreadcrumbClick(bc.path)"
                class="px-1.5 py-0.5 rounded text-slate-300 hover:text-emerald-400 hover:bg-slate-800/80 transition flex-shrink-0"
                :class="{ 'font-bold text-emerald-400': idx === (fsData?.breadcrumbs.length || 0) - 1 }"
              >
                {{ bc.name }}
              </button>
            </template>

            <button
              @click="isManualEditing = true"
              class="ml-auto text-[10px] text-slate-400 hover:text-slate-200 px-2 py-0.5 rounded hover:bg-slate-800 transition flex-shrink-0 font-sans"
              title="Edit path directly"
            >
              Edit path
            </button>
          </div>

          <form v-else @submit.prevent="handleManualSubmit" class="flex items-center gap-1.5">
            <input
              v-model="manualInputPath"
              type="text"
              class="flex-1 px-2.5 py-1 rounded bg-slate-900 border border-emerald-500/50 text-slate-200 text-xs font-mono focus:outline-none"
              placeholder="/absolute/path/to/folder"
              autofocus
            />
            <button
              type="submit"
              class="px-2.5 py-1 rounded bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold"
            >
              Go
            </button>
            <button
              type="button"
              @click="isManualEditing = false"
              class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs"
            >
              Cancel
            </button>
          </form>
        </div>

        <button
          @click="loadDirectory(fsData?.current_path)"
          :disabled="isLoading"
          class="p-1.5 rounded-lg bg-slate-900 border border-slate-800 hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition flex-shrink-0"
          title="Reload directory"
        >
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': isLoading }" />
        </button>

        <!-- New Folder Action Button -->
        <button
          v-if="canCreateFolder"
          type="button"
          @click="isCreatingFolder ? cancelCreateFolder() : openCreateFolder()"
          class="px-2.5 py-1.5 rounded-lg bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-400 hover:text-emerald-300 border border-emerald-500/30 text-xs font-semibold flex items-center gap-1.5 transition flex-shrink-0 shadow-sm"
          :class="{ 'bg-emerald-500/30 text-emerald-200 border-emerald-400': isCreatingFolder }"
          title="Create a new folder in this directory"
        >
          <FolderPlus class="w-3.5 h-3.5" />
          <span>New Folder</span>
        </button>
      </div>

      <!-- Inline Folder Creation Drawer -->
      <div
        v-if="isCreatingFolder"
        class="p-3 bg-emerald-950/40 border-b border-emerald-500/30 space-y-2 animate-in fade-in slide-in-from-top-2 duration-150"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <FolderPlus class="w-4 h-4 text-emerald-400" />
            <span class="text-xs font-semibold text-emerald-200">
              Create New Folder in
              <span class="font-mono text-emerald-400 bg-slate-950/80 px-1.5 py-0.5 rounded border border-slate-800 text-[11px]">
                {{ fsData?.current_path }}
              </span>
            </span>
          </div>
          <button
            type="button"
            @click="cancelCreateFolder"
            class="text-slate-400 hover:text-slate-200 p-1 rounded hover:bg-slate-800/60 transition"
            title="Cancel"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <form @submit.prevent="handleCreateFolder" class="flex items-center gap-2">
          <div class="relative flex-1">
            <input
              ref="newFolderInputRef"
              v-model="newFolderName"
              type="text"
              placeholder="Enter folder name (e.g. workspace-core, my-services)..."
              class="w-full px-3 py-1.5 rounded-lg bg-slate-950 border border-emerald-500/60 text-slate-100 placeholder-slate-500 text-xs font-mono focus:outline-none focus:ring-1 focus:ring-emerald-400"
              :disabled="isSubmittingFolder"
              @keydown.esc="cancelCreateFolder"
            />
          </div>
          <button
            type="submit"
            :disabled="isSubmittingFolder || !newFolderName.trim()"
            class="px-3.5 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 disabled:cursor-not-allowed text-white font-semibold text-xs flex items-center gap-1.5 transition flex-shrink-0 shadow-sm"
          >
            <RefreshCw v-if="isSubmittingFolder" class="w-3.5 h-3.5 animate-spin" />
            <Check v-else class="w-3.5 h-3.5" />
            <span>{{ isSubmittingFolder ? 'Creating...' : 'Create' }}</span>
          </button>
          <button
            type="button"
            @click="cancelCreateFolder"
            :disabled="isSubmittingFolder"
            class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Cancel
          </button>
        </form>

        <div v-if="createFolderError" class="flex items-center gap-1.5 text-xs text-rose-400 font-mono">
          <AlertCircle class="w-3.5 h-3.5 flex-shrink-0" />
          <span>{{ createFolderError }}</span>
        </div>
      </div>

      <!-- Creation Success Notification Banner -->
      <div
        v-if="createFolderSuccess"
        class="px-4 py-2 bg-emerald-950/60 border-b border-emerald-500/40 flex items-center justify-between text-xs text-emerald-300"
      >
        <div class="flex items-center gap-2">
          <Check class="w-4 h-4 text-emerald-400" />
          <span>{{ createFolderSuccess }} — <strong>Selected automatically</strong></span>
        </div>
        <button
          type="button"
          @click="createFolderSuccess = ''"
          class="text-slate-400 hover:text-slate-200"
        >
          <X class="w-3.5 h-3.5" />
        </button>
      </div>

      <!-- Filter Bar -->
      <div class="px-4 py-2 bg-slate-900/60 border-b border-slate-800/80 flex items-center justify-between gap-3">
        <div class="relative flex-1">
          <Search class="w-3.5 h-3.5 text-slate-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
          <input
            v-model="filterQuery"
            type="text"
            placeholder="Filter folders or manifests (e.g. go.mod, package.json)..."
            class="w-full pl-8 pr-3 py-1 rounded-md bg-slate-950 border border-slate-800 text-slate-200 text-xs placeholder-slate-400 focus:outline-none focus:border-emerald-500/60"
          />
        </div>
        <span class="text-[11px] font-mono text-slate-400 flex-shrink-0">
          {{ filteredDirectories.length }} folders
        </span>
      </div>

      <!-- Directory List Body -->
      <div class="flex-1 overflow-y-auto p-3 space-y-1 min-h-[260px] max-h-[380px] bg-slate-950/40">
        <!-- Error State -->
        <div v-if="errorMsg" class="p-4 rounded-xl bg-rose-950/30 border border-rose-900/40 flex items-start gap-3 text-xs">
          <AlertCircle class="w-5 h-5 text-rose-400 flex-shrink-0 mt-0.5" />
          <div class="space-y-1 flex-1">
            <h4 class="font-bold text-rose-200">Unable to browse directory</h4>
            <p class="text-rose-300 font-mono text-[11px]">{{ errorMsg }}</p>
            <div class="pt-2 flex items-center gap-2">
              <button
                @click="loadDirectory('/')"
                class="px-2.5 py-1 rounded bg-rose-900/50 hover:bg-rose-900 text-rose-200 font-mono text-[10px]"
              >
                Go to Root (/)
              </button>
              <button
                @click="handleGoUp"
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 font-mono text-[10px]"
              >
                Go Up
              </button>
            </div>
          </div>
        </div>

        <!-- Loading State -->
        <div v-else-if="isLoading" class="py-12 flex flex-col items-center justify-center gap-2 text-slate-400">
          <RefreshCw class="w-6 h-6 animate-spin text-emerald-400" />
          <span class="text-xs font-mono">Reading directory contents...</span>
        </div>

        <!-- Empty State -->
        <div
          v-else-if="filteredDirectories.length === 0"
          class="py-12 flex flex-col items-center justify-center gap-2 text-slate-400 text-center"
        >
          <Folder class="w-8 h-8 text-slate-400 opacity-60" />
          <p class="text-xs text-slate-400">No subdirectories found in this location.</p>
          <p class="text-[11px] text-slate-400">You can select this folder as your target destination, or create a new subfolder.</p>
          <button
            v-if="canCreateFolder && !isCreatingFolder"
            type="button"
            @click="openCreateFolder"
            class="mt-2 px-3 py-1.5 rounded-lg bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-400 border border-emerald-500/30 text-xs font-semibold flex items-center gap-1.5 transition"
          >
            <FolderPlus class="w-3.5 h-3.5" />
            <span>Create New Folder Here</span>
          </button>
        </div>

        <!-- Directory Item Rows -->
        <div
          v-for="item in filteredDirectories"
          :key="item.path"
          @click="handleDirectoryClick(item)"
          @dblclick="handleDirectoryDblClick(item)"
          class="group flex items-center justify-between p-2 rounded-lg border transition cursor-pointer select-none"
          :class="
            selectedPath === item.path
              ? 'bg-emerald-950/40 border-emerald-500/50 text-emerald-200'
              : 'bg-slate-900/40 border-slate-800/80 hover:bg-slate-900 hover:border-slate-700 text-slate-200'
          "
        >
          <div class="flex items-center gap-2.5 min-w-0 flex-1">
            <div
              class="p-1.5 rounded-md flex-shrink-0"
              :class="
                item.is_repo
                  ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                  : 'bg-slate-800 text-slate-400'
              "
            >
              <FolderGit2 v-if="item.is_repo" class="w-4 h-4" />
              <Folder v-else class="w-4 h-4" />
            </div>

            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-mono font-medium text-xs truncate">{{ item.name }}</span>
                <span
                  v-if="item.manifest"
                  class="px-1.5 py-0.5 rounded text-[10px] font-mono uppercase font-bold"
                  :class="getRoleBadgeClass(item.suggested_role)"
                >
                  {{ item.manifest }}
                </span>
                <span
                  v-if="item.suggested_role && item.suggested_role !== 'other'"
                  class="hidden sm:inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-sans border"
                  :class="getRoleBadgeClass(item.suggested_role)"
                >
                  <component :is="getRoleIcon(item.suggested_role)" class="w-3 h-3" />
                  {{ item.suggested_role }}
                </span>
              </div>
            </div>
          </div>

          <div class="flex items-center gap-2 flex-shrink-0">
            <button
              v-if="item.has_children"
              @click.stop="handleNavigateInto(item)"
              class="px-2 py-1 rounded bg-slate-800/80 hover:bg-slate-800 text-slate-300 hover:text-white text-[11px] flex items-center gap-1 font-mono transition"
              title="Open folder"
            >
              <span>Open</span>
              <ChevronRight class="w-3.5 h-3.5 text-slate-400" />
            </button>
          </div>
        </div>
      </div>

      <!-- Selection Details & Modal Footer -->
      <div class="p-4 border-t border-slate-800 bg-slate-900/95 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="text-[11px] font-mono text-slate-400 flex-shrink-0">Selected Path:</span>
            <span
              class="text-emerald-400 font-mono text-xs font-semibold truncate bg-slate-950 px-2 py-0.5 rounded border border-slate-800 max-w-md block"
              :title="selectedPath || fsData?.current_path"
            >
              {{ selectedPath || fsData?.current_path || 'No folder chosen' }}
            </span>
          </div>
          <div v-if="selectedItem?.manifest" class="mt-1 flex items-center gap-2 text-[11px] text-slate-400">
            <span>Detected: <strong class="text-slate-200">{{ selectedItem.manifest }}</strong></span>
            <span>• Suggested Role: <strong class="text-emerald-400 uppercase font-mono">{{ selectedItem.suggested_role }}</strong></span>
          </div>
        </div>

        <div class="flex items-center gap-2 flex-shrink-0">
          <button
            @click="emit('close')"
            class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 font-semibold transition"
          >
            Cancel
          </button>
          <button
            @click="handleConfirmSelection"
            :disabled="!selectedPath && !fsData?.current_path"
            class="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 disabled:cursor-not-allowed text-white font-semibold flex items-center gap-2 shadow-lg shadow-emerald-950/50 transition"
          >
            <Check class="w-4 h-4" />
            Select This Directory
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
