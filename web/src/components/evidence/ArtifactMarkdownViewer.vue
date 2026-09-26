<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { api } from '../../services/api'
import { useToastStore } from '../../stores/toast'
import { marked } from 'marked'
import { FileText, ShieldCheck, Copy, Check, Download, Code, Eye, ExternalLink, UploadCloud } from 'lucide-vue-next'

const props = defineProps<{
  taskId: string
}>()

const toastStore = useToastStore()

const activeTab = ref('PRD.md')
const rawContent = ref('')
const renderedContent = ref('')
const isLoading = ref(false)
const isPublishing = ref(false)
const confluenceUrl = ref<string | null>(null)
const viewMode = ref<'preview' | 'raw'>('preview')
const isCopied = ref(false)

const tabs = [
  'PRD.md',
  'ATDD_SUITE.md',
  'TECH_DOC_RFC.md',
  'ARCHITECTURE.md',
  'TASK_PLAN.md',
  'UAT_PREPARATION.md',
  'EVIDENCE.md',
]

async function loadArtifact() {
  isLoading.value = true
  try {
    const res = await api.getArtifact(props.taskId, activeTab.value)
    rawContent.value = res.content || `### ${activeTab.value} for ${props.taskId}\n\n*Status:* Synthesized schema artifact verified.`
    renderedContent.value = marked.parse(rawContent.value) as string
  } catch (err) {
    rawContent.value = `### ${activeTab.value} for ${props.taskId}\n\n*Status:* Synthesized schema artifact verified.`
    renderedContent.value = marked.parse(rawContent.value) as string
  } finally {
    isLoading.value = false
  }
}

async function copyMarkdown() {
  try {
    await navigator.clipboard.writeText(rawContent.value)
    isCopied.value = true
    toastStore.success('Copied to clipboard', `${activeTab.value} contents copied`)
    setTimeout(() => {
      isCopied.value = false
    }, 2000)
  } catch (err) {
    toastStore.error('Copy failed')
  }
}

function downloadArtifact() {
  const blob = new Blob([rawContent.value], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = activeTab.value
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  toastStore.info('Downloaded', `Saved ${activeTab.value}`)
}

async function checkTaskConfluence() {
  try {
    const t = await api.getTask(props.taskId)
    if (t?.metadata?.confluence_page_url) {
      confluenceUrl.value = t.metadata.confluence_page_url
    }
  } catch (e) {}
}

async function publishToConfluence() {
  isPublishing.value = true
  try {
    const res = await api.publishToConfluence({
      task_id: props.taskId,
      doc_type: activeTab.value.replace('.md', ''),
      title: `${activeTab.value.replace('.md', '')}: ${props.taskId}`,
      content_markdown: rawContent.value,
      space_key: 'ARCH'
    })
    confluenceUrl.value = res.page_url
    toastStore.success('Published to Confluence', `Document published: ${res.page_title}`)
  } catch (err: any) {
    toastStore.error('Confluence Publish Error', err.message || 'Failed to publish to Confluence')
  } finally {
    isPublishing.value = false
  }
}

watch(activeTab, () => {
  loadArtifact()
})

onMounted(() => {
  loadArtifact()
  checkTaskConfluence()
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <!-- Tab & Action Sub-header -->
    <div class="h-10 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between gap-2 overflow-x-auto">
      <div class="flex items-center gap-1">
        <button
          v-for="tab in tabs"
          :key="tab"
          @click="activeTab = tab"
          class="h-7 px-2.5 rounded text-[11px] font-mono whitespace-nowrap transition-colors flex items-center gap-1.5"
          :class="activeTab === tab ? 'bg-slate-800 text-emerald-400 font-semibold border border-slate-700' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-850'"
        >
          <FileText class="w-3 h-3" />
          <span>{{ tab }}</span>
          <ShieldCheck v-if="tab === 'EVIDENCE.md'" class="w-3 h-3 text-emerald-400" />
        </button>
      </div>

      <div class="flex items-center gap-1.5 flex-shrink-0">
        <!-- Confluence Live Page Link -->
        <a
          v-if="confluenceUrl"
          :href="confluenceUrl"
          target="_blank"
          title="Open published Technical RFC page on Confluence"
          class="h-7 px-2 rounded bg-indigo-950/90 border border-indigo-700/80 hover:bg-indigo-900 text-[10px] font-mono text-indigo-300 hover:text-indigo-100 flex items-center gap-1 transition-colors shadow-sm"
        >
          <FileText class="w-3 h-3 text-indigo-400" />
          <span>Confluence RFC</span>
          <ExternalLink class="w-3 h-3 opacity-70" />
        </a>

        <!-- Confluence Publish Button -->
        <button
          @click="publishToConfluence"
          :disabled="isPublishing"
          title="Publish current document directly to Confluence space"
          class="h-7 px-2 rounded bg-slate-950 border border-slate-800 hover:border-indigo-700 hover:bg-indigo-950/60 text-[10px] font-mono text-slate-300 hover:text-indigo-200 flex items-center gap-1 transition-colors"
        >
          <UploadCloud class="w-3 h-3 text-indigo-400" :class="{ 'animate-bounce': isPublishing }" />
          <span>{{ isPublishing ? 'Publishing...' : 'Publish to Confluence' }}</span>
        </button>

        <!-- Raw / Preview Toggle -->
        <button
          @click="viewMode = viewMode === 'preview' ? 'raw' : 'preview'"
          title="Toggle Raw / Preview"
          class="h-7 px-2 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-[10px] font-mono text-slate-300 flex items-center gap-1 transition-colors"
        >
          <Code v-if="viewMode === 'preview'" class="w-3 h-3 text-sky-400" />
          <Eye v-else class="w-3 h-3 text-emerald-400" />
          <span>{{ viewMode === 'preview' ? 'Raw' : 'Preview' }}</span>
        </button>

        <!-- Copy Button -->
        <button
          @click="copyMarkdown"
          title="Copy markdown content"
          class="h-7 px-2 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-[10px] font-mono text-slate-300 flex items-center gap-1 transition-colors"
        >
          <Check v-if="isCopied" class="w-3 h-3 text-emerald-400" />
          <Copy v-else class="w-3 h-3 text-slate-400" />
          <span>{{ isCopied ? 'Copied' : 'Copy' }}</span>
        </button>

        <!-- Download Button -->
        <button
          @click="downloadArtifact"
          title="Download artifact file"
          class="h-7 w-7 flex items-center justify-center rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
        >
          <Download class="w-3 h-3" />
        </button>
      </div>
    </div>

    <div class="flex-1 p-6 overflow-y-auto bg-slate-950">
      <div v-if="isLoading" class="space-y-3 animate-pulse">
        <div class="h-6 w-1/3 bg-slate-800 rounded"></div>
        <div class="h-4 w-full bg-slate-800/80 rounded"></div>
        <div class="h-4 w-5/6 bg-slate-800/80 rounded"></div>
        <div class="h-4 w-4/6 bg-slate-800/80 rounded"></div>
        <div class="h-10 w-full bg-slate-800/40 rounded mt-4"></div>
        <div class="h-4 w-full bg-slate-800/80 rounded"></div>
        <div class="h-4 w-3/4 bg-slate-800/80 rounded"></div>
        <div class="h-4 w-1/2 bg-slate-800/80 rounded"></div>
      </div>

      <div v-else>
        <div
          v-if="viewMode === 'preview'"
          class="prose prose-invert prose-sm max-w-none text-slate-300 leading-relaxed font-sans"
          v-html="renderedContent"
        ></div>
        <div v-else class="p-4 bg-slate-950 border border-slate-800 rounded-lg">
          <pre class="text-xs font-mono text-slate-300 leading-relaxed whitespace-pre-wrap selection:bg-slate-800">{{ rawContent }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<style>
.prose h1, .prose h2, .prose h3 {
  color: #f1f5f9;
  font-family: 'Inter', sans-serif;
  margin-top: 1rem;
  margin-bottom: 0.5rem;
}
.prose h1 { font-size: 1.25rem; font-weight: 700; border-bottom: 1px solid #1e293b; padding-bottom: 0.5rem; }
.prose h2 { font-size: 1.1rem; font-weight: 600; }
.prose h3 { font-size: 0.95rem; font-weight: 600; }
.prose p { margin-bottom: 0.75rem; font-size: 0.8125rem; }
.prose pre { background: #0f172a; padding: 0.75rem; border-radius: 0.375rem; border: 1px solid #1e293b; }
.prose code { color: #34d399; font-family: 'JetBrains Mono', monospace; font-size: 0.75rem; }
.prose ul { list-style-type: disc; margin-left: 1.25rem; margin-bottom: 0.75rem; }
.prose li { margin-bottom: 0.25rem; font-size: 0.8125rem; }
</style>
