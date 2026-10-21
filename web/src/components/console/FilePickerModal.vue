<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { api } from '../../services/api'
import type { BrowseFSResponse } from '../../types'
import { CornerLeftUp, Eye, EyeOff, File, Folder, RefreshCw, Search, X } from 'lucide-vue-next'

// Browses the host filesystem and returns the absolute paths of the chosen files (or the current
// folder) so they can be attached to a console prompt.

const props = defineProps<{ isOpen: boolean }>()
const emit = defineEmits<{
  (e: 'select', paths: string[]): void
  (e: 'close'): void
}>()

const LAST_DIR_KEY = 'meta_orch_attach_dir'

const data = ref<BrowseFSResponse | null>(null)
const loading = ref(false)
const error = ref('')
const filter = ref('')
const showHidden = ref(false)
const picked = ref<string[]>([])
const editingPath = ref(false)
const manualPath = ref('')
const filterInput = ref<HTMLInputElement | null>(null)

function lastDir(): string | undefined {
  try { return localStorage.getItem(LAST_DIR_KEY) || undefined } catch { return undefined }
}

async function load(path?: string) {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.browseDirectory(path, showHidden.value, true)
    manualPath.value = data.value.current_path
    filter.value = ''
    try { localStorage.setItem(LAST_DIR_KEY, data.value.current_path) } catch { /* storage unavailable */ }
  } catch (err: any) {
    error.value = err?.message || 'Could not open this folder'
  } finally {
    loading.value = false
  }
}

watch(() => props.isOpen, (open) => {
  if (!open) return
  picked.value = []
  editingPath.value = false
  load(lastDir())
  nextTick(() => filterInput.value?.focus())
}, { immediate: true })

const q = computed(() => filter.value.trim().toLowerCase())
const dirs = computed(() => (data.value?.directories || []).filter(d => !q.value || d.name.toLowerCase().includes(q.value)))
const files = computed(() => (data.value?.files || []).filter(f => !q.value || f.name.toLowerCase().includes(q.value)))

function togglePick(path: string) {
  picked.value = picked.value.includes(path) ? picked.value.filter(p => p !== path) : [...picked.value, path]
}

function attach(paths: string[]) {
  if (!paths.length) return
  emit('select', paths)
  emit('close')
}

function formatSize(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    emit('close')
  } else if (e.key === 'Enter' && !editingPath.value && picked.value.length) {
    e.preventDefault()
    attach(picked.value)
  }
}
</script>

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-[70] flex items-center justify-center bg-black/70 p-4"
    role="dialog"
    aria-modal="true"
    aria-label="Attach files"
    @click.self="emit('close')"
    @keydown="onKeydown"
  >
    <div class="flex max-h-[80vh] w-full max-w-2xl flex-col overflow-hidden rounded-lg border border-slate-700 bg-[#0b0f17] font-mono text-sm shadow-2xl">
      <header class="flex items-center gap-2 border-b border-slate-800 px-3 py-2">
        <span class="text-xs font-semibold text-slate-200">Attach files</span>
        <span class="font-sans text-[11px] text-slate-500">their full paths are added to your message</span>
        <button type="button" class="ml-auto rounded p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200" title="Close" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </header>

      <div class="flex items-center gap-1.5 border-b border-slate-800 px-3 py-1.5">
        <button type="button" :disabled="!data?.parent_path || loading" title="Up one folder"
          class="rounded p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200 disabled:opacity-40"
          @click="load(data?.parent_path)">
          <CornerLeftUp class="h-3.5 w-3.5" />
        </button>
        <form v-if="editingPath" class="flex flex-1 gap-1.5" @submit.prevent="editingPath = false; load(manualPath.trim())">
          <input v-model="manualPath" autofocus aria-label="Folder path"
            class="h-6 flex-1 rounded border border-slate-600 bg-slate-950 px-2 text-xs text-slate-100 focus:border-sky-500 focus:outline-none" />
          <button type="submit" class="h-6 rounded bg-sky-700 px-2 text-xs text-white hover:bg-sky-600">Go</button>
        </form>
        <button v-else type="button" title="Type a path"
          class="min-w-0 flex-1 truncate rounded px-1.5 py-0.5 text-left text-xs text-slate-300 hover:bg-slate-800"
          @click="editingPath = true">
          {{ data?.current_path || '…' }}
        </button>
        <button type="button" :title="showHidden ? 'Hide hidden files' : 'Show hidden files'"
          class="rounded p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          @click="showHidden = !showHidden; load(data?.current_path)">
          <Eye v-if="showHidden" class="h-3.5 w-3.5" />
          <EyeOff v-else class="h-3.5 w-3.5" />
        </button>
        <button type="button" title="Reload" class="rounded p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200" @click="load(data?.current_path)">
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
        </button>
      </div>

      <div v-if="data?.quick_bookmarks?.length" class="flex gap-1.5 overflow-x-auto border-b border-slate-800 px-3 py-1.5">
        <button v-for="b in data.quick_bookmarks" :key="b.path" type="button"
          class="flex-shrink-0 rounded border border-slate-800 px-2 py-0.5 text-[11px] text-slate-400 hover:border-slate-600 hover:text-slate-200"
          @click="load(b.path)">{{ b.name }}</button>
      </div>

      <div class="relative border-b border-slate-800 px-3 py-1.5">
        <Search class="absolute left-5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
        <input ref="filterInput" v-model="filter" type="text" placeholder="Filter this folder" aria-label="Filter files"
          class="h-7 w-full rounded border border-slate-800 bg-slate-950 pl-7 pr-2 text-xs text-slate-200 placeholder:text-slate-600 focus:border-slate-600 focus:outline-none" />
      </div>

      <div class="min-h-[240px] flex-1 overflow-y-auto py-1">
        <p v-if="error" class="px-3 py-2 text-xs text-rose-300" role="alert">{{ error }}</p>
        <p v-else-if="loading && !data" class="px-3 py-2 text-xs text-slate-500">Reading folder…</p>
        <template v-else>
          <button v-for="d in dirs" :key="d.path" type="button"
            class="flex w-full items-center gap-2 px-3 py-1 text-left text-xs text-slate-300 hover:bg-slate-800/70"
            @click="load(d.path)">
            <Folder class="h-3.5 w-3.5 flex-shrink-0 text-sky-400/80" />
            <span class="truncate" :class="{ 'text-slate-500': d.is_hidden }">{{ d.name }}/</span>
          </button>
          <label v-for="f in files" :key="f.path"
            class="flex cursor-pointer items-center gap-2 px-3 py-1 text-xs hover:bg-slate-800/70"
            :class="picked.includes(f.path) ? 'bg-sky-950/40 text-sky-100' : 'text-slate-200'"
            @dblclick.prevent="attach([f.path])">
            <input type="checkbox" class="h-3 w-3 accent-sky-500" :checked="picked.includes(f.path)" @change="togglePick(f.path)" />
            <File class="h-3.5 w-3.5 flex-shrink-0 text-slate-500" />
            <span class="truncate" :class="{ 'text-slate-500': f.is_hidden }">{{ f.name }}</span>
            <span class="ml-auto flex-shrink-0 text-[10px] text-slate-500">{{ formatSize(f.size) }}</span>
          </label>
          <p v-if="!dirs.length && !files.length" class="px-3 py-2 text-xs text-slate-500">Nothing here{{ filter ? ' matches the filter' : '' }}.</p>
        </template>
      </div>

      <footer class="flex flex-wrap items-center gap-2 border-t border-slate-800 px-3 py-2">
        <span class="font-sans text-[11px] text-slate-500">
          {{ picked.length ? `${picked.length} file(s) selected` : 'Pick files, double-click one, or attach this folder' }}
        </span>
        <button type="button" :disabled="!data"
          class="ml-auto h-7 rounded border border-slate-700 bg-slate-800 px-3 text-xs text-slate-200 hover:bg-slate-700 disabled:opacity-50"
          @click="attach([data!.current_path])">
          Attach folder
        </button>
        <button type="button" :disabled="!picked.length"
          class="h-7 rounded border border-sky-500 bg-sky-600 px-3 text-xs font-semibold text-white hover:bg-sky-500 disabled:opacity-50"
          @click="attach(picked)">
          Attach{{ picked.length > 1 ? ` ${picked.length}` : '' }}
        </button>
      </footer>
    </div>
  </div>
</template>
