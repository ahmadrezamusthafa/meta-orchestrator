<script setup lang="ts">
import { ref } from 'vue'
import {
  CheckCircle2, AlertTriangle, CircleDashed, Download, Copy, Check, ExternalLink, Loader2, ChevronDown, ChevronRight, Terminal,
} from 'lucide-vue-next'
import type { ToolchainItem, ToolchainMethod } from '../../types'
import { useToolchainStore } from '../../stores/toolchain'

defineProps<{
  items: ToolchainItem[]
  /** Hide the "why" lines (e.g. in the catalog view, where nothing is needed). */
  hideReasons?: boolean
}>()

const store = useToolchainStore()
const copied = ref('')
const expandedMethods = ref<Record<string, boolean>>({})
const expandedLog = ref<Record<string, boolean>>({})

const MANAGER_LABEL: Record<string, string> = { brew: 'Homebrew', npm: 'npm', go: 'go install', apt: 'apt', snap: 'snap' }

function methods(item: ToolchainItem): ToolchainMethod[] {
  return item.methods || []
}

function recommended(item: ToolchainItem): ToolchainMethod | undefined {
  return methods(item).find((m) => m.index === item.recommended)
}

function otherMethods(item: ToolchainItem): ToolchainMethod[] {
  return methods(item).filter((m) => m.index !== item.recommended)
}

function installLabel(m: ToolchainMethod): string {
  return `Install with ${MANAGER_LABEL[m.manager] || m.manager}`
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    // Older browsers / insecure origins: fall back to a hidden textarea.
    const el = document.createElement('textarea')
    el.value = text
    el.style.position = 'fixed'
    el.style.opacity = '0'
    document.body.appendChild(el)
    el.select()
    document.execCommand('copy')
    document.body.removeChild(el)
  }
  copied.value = text
  setTimeout(() => {
    if (copied.value === text) copied.value = ''
  }, 1800)
}

function logTail(lines: string[], all: boolean): string[] {
  return all ? lines : lines.slice(-8)
}
</script>

<template>
  <ul class="space-y-2">
    <li
      v-for="item in items"
      :key="item.id"
      class="rounded-xl border p-3.5 text-xs transition-colors"
      :class="item.installed
        ? 'bg-slate-900/60 border-slate-800'
        : item.required
          ? 'bg-amber-950/20 border-amber-800/50'
          : 'bg-slate-900 border-slate-800'"
    >
      <div class="flex items-start gap-3">
        <!-- Status icon -->
        <div class="pt-0.5 flex-shrink-0">
          <Loader2 v-if="store.isInstalling(item.id)" class="w-4 h-4 text-sky-400 animate-spin" />
          <CheckCircle2 v-else-if="item.installed" class="w-4 h-4 text-emerald-400" />
          <AlertTriangle v-else-if="item.required" class="w-4 h-4 text-amber-400" />
          <CircleDashed v-else class="w-4 h-4 text-slate-500" />
        </div>

        <!-- Name, status, why -->
        <div class="min-w-0 flex-1 space-y-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-semibold text-slate-100 text-[13px]">{{ item.name }}</span>
            <span v-if="item.binary" class="font-mono text-[10px] text-slate-500">{{ item.binary }}</span>
            <span
              v-if="item.needed && item.required"
              class="px-1.5 py-0.5 rounded border text-[10px] font-semibold"
              :class="item.installed ? 'border-slate-700 text-slate-400' : 'border-amber-700/60 bg-amber-900/30 text-amber-300'"
            >Required</span>
            <span v-else-if="item.needed" class="px-1.5 py-0.5 rounded border border-slate-700 text-slate-400 text-[10px]">Recommended</span>
          </div>

          <p v-if="item.installed" class="text-emerald-300/90 font-mono text-[11px] truncate" :title="item.path">
            {{ item.version || 'Installed' }}
          </p>
          <p v-else-if="store.isInstalling(item.id)" class="text-sky-300">Installing…</p>
          <p v-else-if="item.problem" class="text-amber-300/90">{{ item.problem }}</p>
          <p v-else class="text-slate-400">Not installed</p>

          <p class="text-slate-400">{{ item.purpose }}</p>
          <ul v-if="!hideReasons && item.reasons?.length" class="text-[11px] text-slate-500 space-y-0.5">
            <li v-for="r in item.reasons" :key="r">• {{ r }}</li>
          </ul>
        </div>

        <!-- Primary action -->
        <div v-if="!item.installed" class="flex flex-col items-end gap-1.5 flex-shrink-0">
          <template v-if="recommended(item)?.auto && !store.isInstalling(item.id)">
            <button
              type="button"
              class="h-8 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-semibold flex items-center gap-1.5 transition"
              :title="`Runs: ${recommended(item)!.command}`"
              @click="store.install(item, item.recommended)"
            >
              <Download class="w-3.5 h-3.5" />
              {{ installLabel(recommended(item)!) }}
            </button>
          </template>
          <button
            v-else-if="recommended(item) && !store.isInstalling(item.id)"
            type="button"
            class="h-8 px-3 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-100 font-semibold flex items-center gap-1.5 transition"
            @click="copy(recommended(item)!.command)"
          >
            <Check v-if="copied === recommended(item)!.command" class="w-3.5 h-3.5 text-emerald-400" />
            <Copy v-else class="w-3.5 h-3.5" />
            {{ copied === recommended(item)!.command ? 'Copied' : 'Copy command' }}
          </button>
          <a
            :href="item.docs"
            target="_blank"
            rel="noopener noreferrer"
            class="text-[11px] text-sky-400 hover:text-sky-300 flex items-center gap-1"
          >
            Install guide <ExternalLink class="w-3 h-3" />
          </a>
        </div>
      </div>

      <!-- Install details for a missing tool -->
      <div v-if="!item.installed" class="mt-2.5 ml-7 space-y-2">
        <!-- What will run / what to run -->
        <div v-if="recommended(item)" class="space-y-1">
          <div class="flex items-center gap-2 rounded-lg bg-slate-950 border border-slate-800 px-2.5 py-1.5 font-mono text-[11px] text-slate-200">
            <Terminal class="w-3.5 h-3.5 text-slate-500 flex-shrink-0" />
            <span class="truncate flex-1" :title="recommended(item)!.command">{{ recommended(item)!.command }}</span>
            <button type="button" class="text-slate-500 hover:text-slate-200" title="Copy" @click="copy(recommended(item)!.command)">
              <Check v-if="copied === recommended(item)!.command" class="w-3.5 h-3.5 text-emerald-400" />
              <Copy v-else class="w-3.5 h-3.5" />
            </button>
          </div>
          <p class="text-[11px] text-slate-500">
            <template v-if="recommended(item)!.auto">The orchestrator runs this for you and checks the result.</template>
            <template v-else>Run this in your terminal, then press Re-check.</template>
            <template v-if="recommended(item)!.note"> {{ recommended(item)!.note }}</template>
          </p>
        </div>
        <p v-else class="text-[11px] text-amber-300/90">
          No installer is available on this machine — follow the install guide, then press Re-check.
          <template v-for="m in methods(item)" :key="m.index">
            <span v-if="m.unavailable" class="block text-slate-500">{{ MANAGER_LABEL[m.manager] || m.manager }}: {{ m.unavailable }}</span>
          </template>
        </p>

        <!-- Other ways -->
        <div v-if="otherMethods(item).length">
          <button
            type="button"
            class="text-[11px] text-slate-400 hover:text-slate-200 flex items-center gap-1"
            @click="expandedMethods[item.id] = !expandedMethods[item.id]"
          >
            <ChevronDown v-if="expandedMethods[item.id]" class="w-3 h-3" />
            <ChevronRight v-else class="w-3 h-3" />
            Other ways to install ({{ otherMethods(item).length }})
          </button>
          <ul v-if="expandedMethods[item.id]" class="mt-1.5 space-y-1.5">
            <li v-for="m in otherMethods(item)" :key="m.index" class="flex items-center gap-2 text-[11px]">
              <span class="w-20 text-slate-500 flex-shrink-0">{{ MANAGER_LABEL[m.manager] || m.manager }}</span>
              <code class="flex-1 truncate font-mono text-slate-300 bg-slate-950 border border-slate-800 rounded px-2 py-1" :title="m.command">{{ m.command }}</code>
              <button
                v-if="m.auto && m.available"
                type="button"
                class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-100 disabled:opacity-50"
                :disabled="store.isInstalling(item.id)"
                @click="store.install(item, m.index)"
              >Install</button>
              <button v-else type="button" class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-100" @click="copy(m.command)">
                {{ copied === m.command ? 'Copied' : 'Copy' }}
              </button>
              <span v-if="!m.available" class="text-slate-500 truncate max-w-[12rem]" :title="m.unavailable">{{ m.unavailable }}</span>
            </li>
          </ul>
        </div>
      </div>

      <!-- Install progress and outcome -->
      <div v-if="store.jobFor(item)" class="mt-2.5 ml-7 space-y-1.5">
        <p
          v-if="store.jobFor(item)!.message"
          class="text-[11px]"
          :class="store.jobFor(item)!.state === 'succeeded' ? 'text-emerald-300' : 'text-rose-300'"
        >{{ store.jobFor(item)!.message }}</p>
        <button
          type="button"
          class="text-[11px] text-slate-400 hover:text-slate-200 flex items-center gap-1"
          @click="expandedLog[item.id] = !expandedLog[item.id]"
        >
          <ChevronDown v-if="expandedLog[item.id] || store.jobFor(item)!.state === 'running'" class="w-3 h-3" />
          <ChevronRight v-else class="w-3 h-3" />
          Install log
        </button>
        <pre
          v-if="expandedLog[item.id] || store.jobFor(item)!.state === 'running'"
          class="max-h-48 overflow-auto rounded-lg bg-black/60 border border-slate-800 p-2 font-mono text-[10.5px] leading-relaxed text-slate-300 whitespace-pre-wrap"
        >{{ logTail(store.jobFor(item)!.log || [], !!expandedLog[item.id]).join('\n') || '…' }}</pre>
      </div>
    </li>
  </ul>
</template>
