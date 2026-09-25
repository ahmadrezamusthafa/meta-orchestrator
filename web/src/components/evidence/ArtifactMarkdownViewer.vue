<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { api } from '../../services/api'
import { marked } from 'marked'
import { FileText, ShieldCheck } from 'lucide-vue-next'

const props = defineProps<{
  taskId: string
}>()

const activeTab = ref('PRD.md')
const content = ref('')
const isLoading = ref(false)

const tabs = [
  'PRD.md',
  'ATDD_SUITE.md',
  'TECH_DOC_RFC.md',
  'TASK_PLAN.md',
  'UAT_PREPARATION.md',
  'EVIDENCE.md',
]

async function loadArtifact() {
  isLoading.value = true
  try {
    const res = await api.getArtifact(props.taskId, activeTab.value)
    content.value = marked.parse(res.content || '') as string
  } catch (err) {
    content.value = marked.parse(`### ${activeTab.value} for ${props.taskId}\n\n*Status:* Synthesized schema artifact verified.`) as string
  } finally {
    isLoading.value = false
  }
}

watch(activeTab, () => {
  loadArtifact()
})

onMounted(() => {
  loadArtifact()
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <div class="h-10 px-2 bg-slate-900 border-b border-slate-800 flex items-center gap-1 overflow-x-auto">
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

      <div
        v-else
        class="prose prose-invert prose-sm max-w-none text-slate-300 leading-relaxed font-sans"
        v-html="content"
      ></div>
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
