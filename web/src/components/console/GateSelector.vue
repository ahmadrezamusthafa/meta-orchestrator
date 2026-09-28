<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  stage: string
  pending?: boolean
}>()

const emit = defineEmits<{
  (e: 'approve'): void
  (e: 'reject'): void
}>()

const selected = ref(0)
const root = ref<HTMLDivElement | null>(null)

const options = [
  { label: 'Yes', hint: 'advance to the next stage' },
  { label: 'No, and tell the agent what to do differently', hint: '' },
]

function choose(i: number) {
  if (props.pending) return
  selected.value = i
  if (i === 0) emit('approve')
  else emit('reject')
}

function onKeydown(e: KeyboardEvent) {
  switch (e.key) {
    case 'ArrowUp':
    case 'k':
      e.preventDefault()
      selected.value = (selected.value + options.length - 1) % options.length
      break
    case 'ArrowDown':
    case 'j':
    case 'Tab':
      e.preventDefault()
      selected.value = (selected.value + 1) % options.length
      break
    case '1':
      e.preventDefault()
      choose(0)
      break
    case '2':
      e.preventDefault()
      choose(1)
      break
    case 'Enter':
    case ' ':
      e.preventDefault()
      choose(selected.value)
      break
  }
}

defineExpose({ focus: () => root.value?.focus() })
</script>

<template>
  <div
    ref="root"
    tabindex="0"
    role="radiogroup"
    :aria-label="`Gate approval for ${stage}`"
    class="rounded-lg border border-amber-700/60 bg-amber-950/10 px-3 py-2 font-mono text-sm outline-none focus-visible:ring-1 focus-visible:ring-amber-500/70"
    @keydown="onKeydown"
  >
    <div class="mb-1 text-amber-300">
      Do you want to advance past <span class="font-semibold text-amber-200">{{ stage || 'this stage' }}</span>?
    </div>
    <div
      v-for="(opt, i) in options"
      :key="i"
      role="radio"
      :aria-checked="selected === i"
      class="flex cursor-pointer items-baseline gap-2 py-px"
      :class="selected === i ? 'text-sky-300' : 'text-slate-400 hover:text-slate-200'"
      @click="choose(i)"
      @mouseenter="selected = i"
    >
      <span class="w-3 select-none">{{ selected === i ? '❯' : ' ' }}</span>
      <span>{{ i + 1 }}. {{ opt.label }}</span>
      <span v-if="opt.hint" class="text-xs text-slate-600">{{ opt.hint }}</span>
    </div>
    <div class="mt-1 text-[11px] text-slate-600">
      <span v-if="pending">submitting…</span>
      <span v-else>↑/↓ to select · 1/2 or enter to confirm</span>
    </div>
  </div>
</template>
