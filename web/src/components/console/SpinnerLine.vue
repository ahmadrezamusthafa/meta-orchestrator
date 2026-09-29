<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { formatTokens } from './consoleFormat'

const props = defineProps<{
  startedAt: number | null
  tokens: number
}>()

const FRAMES = ['·', '✢', '✳', '✶', '✻', '✽', '✻', '✶', '✳', '✢']
const VERBS = [
  'Thinking', 'Routing', 'Reasoning', 'Orchestrating', 'Pondering', 'Synthesizing',
  'Computing', 'Crafting', 'Deliberating', 'Assembling', 'Weaving', 'Brewing',
]

const tick = ref(0)
const now = ref(Date.now())
const verbOffset = Math.floor(Math.random() * VERBS.length)
let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  timer = setInterval(() => {
    tick.value++
    now.value = Date.now()
  }, 120)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})

const glyph = computed(() => FRAMES[tick.value % FRAMES.length])
const verb = computed(() => VERBS[(verbOffset + Math.floor(tick.value / 25)) % VERBS.length])
const elapsed = computed(() => (props.startedAt ? Math.max(0, Math.floor((now.value - props.startedAt) / 1000)) : 0))
</script>

<template>
  <div class="flex items-baseline gap-2 font-mono text-sm" role="status" aria-live="off">
    <span class="w-3 select-none text-orange-400" aria-hidden="true">{{ glyph }}</span>
    <span class="text-orange-300">{{ verb }}…</span>
    <span class="text-slate-500">
      ({{ elapsed }}s<template v-if="tokens"> · ↓ {{ formatTokens(tokens) }} tokens</template> · <span class="text-slate-400">esc</span> to interrupt)
    </span>
  </div>
</template>
