<script setup lang="ts">
import { ref } from 'vue'
import { useSkillsStore } from '../../stores/skills'
import { useToastStore } from '../../stores/toast'
import type { CheckPathCompatibilityResponse } from '../../types'
import {
  X,
  FolderPlus,
  Search,
  Sparkles,
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Folder,
} from 'lucide-vue-next'

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'registered'): void
}>()

const skillsStore = useSkillsStore()
const toastStore = useToastStore()

const sourcePath = ref('')
const sourceName = ref('')
const selectedFormat = ref<'auto' | 'claude' | 'mcp'>('auto')
const isTesting = ref(false)
const isSubmitting = ref(false)
const testResult = ref<CheckPathCompatibilityResponse | null>(null)

const presets = [
  { label: 'Billing Claude Skills (.claude/skills)', path: '/Users/rezamekari/Projects/go/src/bitbucket.org/mid-kelola-indonesia/billing/.claude/skills' },
  { label: 'Project Claude (.claude/skills)', path: '.claude/skills' },
  { label: 'Agent Custom (.agents/skills)', path: '.agents/skills' },
  { label: 'Home Claude (~/.claude/skills)', path: '~/.claude/skills' },
  { label: 'Project Local (.sdlc/skills)', path: '.sdlc/skills' },
]


function applyPreset(presetPath: string) {
  sourcePath.value = presetPath
  testResult.value = null
}

async function handleTestCompatibility() {
  if (!sourcePath.value.trim()) {
    toastStore.warning('Validation', 'Please enter a directory path')
    return
  }

  isTesting.value = true
  testResult.value = null
  try {
    const res = await skillsStore.checkPath(sourcePath.value.trim())
    testResult.value = res
    if (res.compatible && res.discovered_count > 0) {
      toastStore.success('Audit Passed', `Discovered ${res.discovered_count} compatible skill(s)`)
      if (!sourceName.value) {
        sourceName.value = sourcePath.value.split('/').filter(Boolean).pop() || 'Custom Skills'
      }
    } else if (res.discovered_count === 0) {
      toastStore.warning('No Skills Found', res.error || 'No recognized SKILL.md or mcp.json found')
    } else {
      toastStore.warning('Compatibility Warnings', `Discovered ${res.discovered_count} skill(s) with warnings`)
    }
  } catch (err: any) {
    toastStore.error('Audit Failed', err.message || 'Could not access directory')
  } finally {
    isTesting.value = false
  }
}

async function handleRegister() {
  if (!sourcePath.value.trim()) {
    toastStore.warning('Validation', 'Please enter a directory path')
    return
  }

  isSubmitting.value = true
  try {
    const name = sourceName.value.trim() || sourcePath.value.split('/').filter(Boolean).pop() || 'Custom Skills'
    const format = selectedFormat.value === 'auto' ? undefined : selectedFormat.value
    await skillsStore.registerSource(name, sourcePath.value.trim(), format)
    toastStore.success('Source Registered', `Skills from ${name} are now active in Meta-Orchestrator`)
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
          <div class="w-10 h-10 rounded-xl bg-emerald-950/80 border border-emerald-800 flex items-center justify-center text-emerald-400">
            <FolderPlus class="w-5 h-5" />
          </div>
          <div>
            <h3 class="text-base font-bold text-slate-100">
              Register Skill Directory
            </h3>
            <p class="text-xs text-slate-400">
              Adapt modular skills from Claude (<span class="font-mono text-emerald-400">SKILL.md</span>) or external tools
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
            Common Skill Locations
          </label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="p in presets"
              :key="p.path"
              type="button"
              @click="applyPreset(p.path)"
              class="px-2.5 py-1 rounded-lg text-xs bg-slate-950 border border-slate-800 hover:border-emerald-600 text-slate-300 hover:text-emerald-300 transition-colors flex items-center gap-1.5"
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
            <span class="text-[10px] text-slate-500">Absolute or relative to workspace</span>
          </label>
          <div class="relative">
            <input
              v-model="sourcePath"
              type="text"
              placeholder="/Users/dev/.claude/skills or .claude/skills"
              class="w-full h-10 px-3.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500 font-mono"
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
            placeholder="e.g. Anthropic Official Skills, QA Automation Pack"
            class="w-full h-10 px-3.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
          />
        </div>

        <!-- Format Selector -->
        <div class="space-y-1.5">
          <label class="text-xs font-semibold uppercase tracking-wider text-slate-300">
            Adaptation Format
          </label>
          <div class="grid grid-cols-3 gap-2">
            <button
              type="button"
              @click="selectedFormat = 'auto'"
              class="h-9 rounded-xl border text-xs font-medium transition-colors flex items-center justify-center gap-1.5"
              :class="selectedFormat === 'auto'
                ? 'bg-emerald-950/60 border-emerald-700 text-emerald-300'
                : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
            >
              <Sparkles class="w-3.5 h-3.5" />
              <span>Auto-Detect</span>
            </button>
            <button
              type="button"
              @click="selectedFormat = 'claude'"
              class="h-9 rounded-xl border text-xs font-medium transition-colors flex items-center justify-center gap-1.5"
              :class="selectedFormat === 'claude'
                ? 'bg-emerald-950/60 border-emerald-700 text-emerald-300'
                : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
            >
              <span>Claude (SKILL.md)</span>
            </button>
            <button
              type="button"
              @click="selectedFormat = 'mcp'"
              class="h-9 rounded-xl border text-xs font-medium transition-colors flex items-center justify-center gap-1.5"
              :class="selectedFormat === 'mcp'
                ? 'bg-emerald-950/60 border-emerald-700 text-emerald-300'
                : 'bg-slate-950 border-slate-800 text-slate-400 hover:text-slate-200'"
            >
              <span>MCP Manifest</span>
            </button>
          </div>
        </div>

        <!-- Pre-flight Compatibility Audit Card -->
        <div class="pt-2">
          <button
            type="button"
            @click="handleTestCompatibility"
            :disabled="isTesting || !sourcePath.trim()"
            class="w-full h-10 rounded-xl bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-slate-200 text-xs font-semibold flex items-center justify-center gap-2 transition-colors border border-slate-700 shadow-sm"
          >
            <Search class="w-4 h-4 text-emerald-400" :class="{ 'animate-spin': isTesting }" />
            <span>{{ isTesting ? 'Running Compatibility Audit...' : 'Test & Check Compatibility' }}</span>
          </button>
        </div>

        <!-- Audit Feedback -->
        <div
          v-if="testResult"
          class="p-4 rounded-xl border animate-in fade-in space-y-2 text-xs"
          :class="testResult.compatible && testResult.discovered_count > 0
            ? 'bg-emerald-950/30 border-emerald-800/80 text-emerald-300'
            : testResult.discovered_count === 0
            ? 'bg-rose-950/30 border-rose-800/80 text-rose-300'
            : 'bg-amber-950/30 border-amber-800/80 text-amber-300'"
        >
          <div class="flex items-center gap-2 font-semibold">
            <CheckCircle2 v-if="testResult.compatible && testResult.discovered_count > 0" class="w-4 h-4 text-emerald-400 flex-shrink-0" />
            <XCircle v-else-if="testResult.discovered_count === 0" class="w-4 h-4 text-rose-400 flex-shrink-0" />
            <AlertTriangle v-else class="w-4 h-4 text-amber-400 flex-shrink-0" />

            <span>
              {{ testResult.discovered_count }} Skill(s) Discovered in Path
            </span>
          </div>

          <div v-if="testResult.skills.length > 0" class="space-y-1 pl-6">
            <div
              v-for="s in testResult.skills"
              :key="s.name"
              class="flex items-center justify-between text-[11px] text-slate-300 py-0.5"
            >
              <span class="font-mono">{{ s.name }}</span>
              <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase" :class="s.compatibility?.status === 'compatible' ? 'bg-emerald-900/60 text-emerald-300' : 'bg-amber-900/60 text-amber-300'">
                {{ s.compatibility?.score ?? 100 }}% Score
              </span>
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
          class="px-5 py-2 rounded-xl bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-semibold shadow-lg shadow-emerald-900/30 transition-colors flex items-center gap-2"
        >
          <FolderPlus class="w-4 h-4" />
          <span>{{ isSubmitting ? 'Registering...' : 'Register Source' }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
