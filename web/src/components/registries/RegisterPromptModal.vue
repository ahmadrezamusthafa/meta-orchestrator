<script setup lang="ts">
import { ref } from 'vue'
import { useToastStore } from '../../stores/toast'
import { api } from '../../services/api'
import type { PromptItemDTO } from '../../types'
import {
  X,
  FolderPlus,
  Search,
  Sparkles,
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Folder,
  FileText,
} from 'lucide-vue-next'

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'registered'): void
}>()

const toastStore = useToastStore()

const sourcePath = ref('')
const sourceName = ref('')
const isTesting = ref(false)
const isSubmitting = ref(false)
const testResult = ref<{
  compatible: boolean
  path: string
  discovered_count: number
  templates: PromptItemDTO[]
  error?: string
} | null>(null)

const presets = [
  { label: 'Billing Prompts (.github/prompts)', path: '/Users/rezamekari/Projects/go/src/bitbucket.org/mid-kelola-indonesia/billing/.github/prompts' },
  { label: 'Project GitHub (.github/prompts)', path: '.github/prompts' },
  { label: 'Project Local (.sdlc/prompts)', path: '.sdlc/prompts' },
  { label: 'System Prompts (~/.config/...)', path: '~/.config/meta-orchestrator/prompts' },
]

function applyPreset(presetPath: string) {
  sourcePath.value = presetPath
  testResult.value = null
}

async function handleTestCompatibility() {
  if (!sourcePath.value.trim()) {
    toastStore.warning('Validation', 'Please enter a prompt directory path')
    return
  }

  isTesting.value = true
  testResult.value = null
  try {
    const res = await api.checkPromptCompatibility(sourcePath.value.trim())
    testResult.value = res
    if (res.compatible && res.discovered_count > 0) {
      toastStore.success('Audit Passed', `Discovered ${res.discovered_count} compatible prompt template(s)`)
      if (!sourceName.value) {
        sourceName.value = sourcePath.value.split('/').filter(Boolean).pop() || 'Prompt Templates'
      }
    } else if (res.discovered_count === 0) {
      toastStore.warning('No Prompts Found', res.error || 'No *.prompt.md or *.md templates found')
    } else {
      toastStore.warning('Compatibility Warnings', `Discovered ${res.discovered_count} template(s) with warnings`)
    }
  } catch (err: any) {
    toastStore.error('Audit Failed', err.message || 'Could not access prompt directory')
  } finally {
    isTesting.value = false
  }
}

async function handleRegister() {
  if (!sourcePath.value.trim()) {
    toastStore.warning('Validation', 'Please enter a prompt directory path')
    return
  }

  isSubmitting.value = true
  try {
    const name = sourceName.value.trim() || sourcePath.value.split('/').filter(Boolean).pop() || 'Prompt Templates'
    const res = await api.registerPromptSource({ name, path: sourcePath.value.trim() })
    toastStore.success('Source Registered', `${res.discovered_count} prompt templates from ${name} are now available`)
    emit('registered')
    emit('close')
  } catch (err: any) {
    toastStore.error('Registration Failed', err.message || 'Could not register directory')
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm animate-in fade-in duration-200">
    <div
      class="w-full max-w-xl bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]"
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b border-slate-800 bg-slate-900/60 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-sky-950/80 border border-sky-800 flex items-center justify-center text-sky-400">
            <FolderPlus class="w-5 h-5" />
          </div>
          <div>
            <h3 class="text-base font-bold text-slate-100">
              Register Prompt Directory
            </h3>
            <p class="text-xs text-slate-400">
              Import parameterized prompt templates (<span class="font-mono text-sky-400">*.prompt.md</span>) from GitHub or local repos
            </p>
          </div>
        </div>

        <button
          type="button"
          @click="emit('close')"
          class="w-8 h-8 rounded-lg flex items-center justify-center text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Form Body -->
      <div class="p-6 overflow-y-auto space-y-5">
        <!-- Quick Presets -->
        <div class="space-y-2">
          <label class="text-xs font-semibold uppercase tracking-wider text-slate-400">
            Common Prompt Locations
          </label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="p in presets"
              :key="p.path"
              type="button"
              @click="applyPreset(p.path)"
              class="px-2.5 py-1 rounded-lg text-xs bg-slate-950 border border-slate-800 hover:border-sky-600 text-slate-300 hover:text-sky-300 transition-colors flex items-center gap-1.5"
            >
              <Folder class="w-3.5 h-3.5 text-slate-500" />
              <span>{{ p.label }}</span>
            </button>
          </div>
        </div>

        <!-- Directory Path Input -->
        <div class="space-y-1.5">
          <label class="text-xs font-semibold uppercase tracking-wider text-slate-300 flex items-center justify-between">
            <span>Directory Path <span class="text-rose-400">*</span></span>
            <span class="text-[10px] text-slate-500">Path to folder containing *.prompt.md or *.md</span>
          </label>
          <div class="relative">
            <input
              v-model="sourcePath"
              type="text"
              placeholder="/Users/.../billing/.github/prompts or .github/prompts"
              class="w-full h-10 px-3.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 font-mono"
            />
          </div>
        </div>

        <!-- Source Label / Name -->
        <div class="space-y-1.5">
          <label class="text-xs font-semibold uppercase tracking-wider text-slate-300">
            Display Label (Optional)
          </label>
          <input
            v-model="sourceName"
            type="text"
            placeholder="e.g. Billing Core Prompts, SDLC Engineering Prompts"
            class="w-full h-10 px-3.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500"
          />
        </div>

        <!-- Pre-flight Compatibility Audit Card -->
        <div class="pt-2">
          <button
            type="button"
            @click="handleTestCompatibility"
            :disabled="isTesting || !sourcePath.trim()"
            class="w-full h-10 rounded-xl bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-slate-200 text-xs font-semibold flex items-center justify-center gap-2 transition-colors border border-slate-700 shadow-sm"
          >
            <Search class="w-4 h-4 text-sky-400" :class="{ 'animate-spin': isTesting }" />
            <span>{{ isTesting ? 'Running Compatibility Audit...' : 'Audit & Test Compatibility' }}</span>
          </button>
        </div>

        <!-- Audit Feedback -->
        <div
          v-if="testResult"
          class="p-4 rounded-xl border animate-in fade-in space-y-2 text-xs"
          :class="testResult.compatible && testResult.discovered_count > 0
            ? 'bg-sky-950/30 border-sky-800/80 text-sky-300'
            : testResult.discovered_count === 0
            ? 'bg-rose-950/30 border-rose-800/80 text-rose-300'
            : 'bg-amber-950/30 border-amber-800/80 text-amber-300'"
        >
          <div class="flex items-center gap-2 font-semibold">
            <CheckCircle2 v-if="testResult.compatible && testResult.discovered_count > 0" class="w-4 h-4 text-emerald-400 flex-shrink-0" />
            <XCircle v-else-if="testResult.discovered_count === 0" class="w-4 h-4 text-rose-400 flex-shrink-0" />
            <AlertTriangle v-else class="w-4 h-4 text-amber-400 flex-shrink-0" />

            <span>
              {{ testResult.discovered_count }} Prompt Template(s) Discovered & Validated
            </span>
          </div>

          <div v-if="testResult.templates.length > 0" class="space-y-1.5 pl-6 max-h-48 overflow-y-auto">
            <div
              v-for="t in testResult.templates"
              :key="t.id"
              class="flex items-center justify-between text-[11px] text-slate-300 py-0.5 border-b border-slate-800/50 last:border-0"
            >
              <div class="min-w-0 pr-2">
                <span class="font-medium text-slate-200">{{ t.name }}</span>
                <span class="text-slate-500 font-mono text-[10px] block truncate">{{ t.id }}</span>
              </div>
              <div class="flex items-center gap-1.5 flex-shrink-0">
                <span v-if="t.variables.length > 0" class="px-1.5 py-0.2 rounded text-[9px] font-mono bg-sky-950 text-sky-300 border border-sky-800">
                  {{ t.variables.length }} vars
                </span>
                <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase bg-emerald-900/60 text-emerald-300">
                  100% Valid
                </span>
              </div>
            </div>
          </div>

          <p v-if="testResult.error" class="text-rose-400 pl-6 text-[11px]">
            {{ testResult.error }}
          </p>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t border-slate-800 bg-slate-900/60 flex items-center justify-end gap-2.5">
        <button
          type="button"
          @click="emit('close')"
          class="px-4 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition-colors"
        >
          Cancel
        </button>
        <button
          type="button"
          @click="handleRegister"
          :disabled="isSubmitting || !sourcePath.trim()"
          class="px-5 py-2 rounded-xl bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white text-xs font-semibold shadow-lg shadow-sky-900/30 transition-colors flex items-center gap-2"
        >
          <FolderPlus class="w-4 h-4" />
          <span>{{ isSubmitting ? 'Registering...' : 'Register Directory' }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
