<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  source?: string
  rationale?: string
}>()

const showPopover = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const popoverPos = ref({ top: '0px', left: '0px', transform: 'translateY(-100%)' })

function handleMouseEnter() {
  if (triggerRef.value) {
    const rect = triggerRef.value.getBoundingClientRect()
    const popoverWidth = 260
    let left = rect.right - popoverWidth
    if (left < 16) left = 16
    if (left + popoverWidth > window.innerWidth - 16) {
      left = Math.max(16, window.innerWidth - popoverWidth - 16)
    }

    // If too close to viewport top, show below, else show above
    let top = rect.top - 8
    let transform = 'translateY(-100%)'
    if (rect.top < 140) {
      top = rect.bottom + 8
      transform = 'translateY(0)'
    }

    popoverPos.value = {
      top: `${top}px`,
      left: `${left}px`,
      transform,
    }
    showPopover.value = true
  }
}

function handleMouseLeave() {
  showPopover.value = false
}
</script>

<template>
  <div class="relative inline-block" ref="triggerRef" @mouseenter="handleMouseEnter" @mouseleave="handleMouseLeave">
    <button
      type="button"
      class="px-1.5 py-0.5 rounded text-[10px] font-mono tracking-tight font-medium cursor-help transition-colors"
      :class="{
        'bg-sky-950/80 text-sky-300 border border-sky-800/80 hover:bg-sky-900': source === 'BP' || !source,
        'bg-purple-950/80 text-purple-300 border border-purple-800/80 hover:bg-purple-900': source === 'RULE',
      }"
    >
      {{ source === 'RULE' ? '[RULE]' : '[BP]' }}
    </button>

    <Teleport to="body">
      <div
        v-if="showPopover"
        class="fixed z-[9999] w-64 p-3 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl text-[11px] text-slate-300 leading-relaxed pointer-events-none transition-all duration-150"
        :style="{ top: popoverPos.top, left: popoverPos.left, transform: popoverPos.transform }"
      >
        <div class="font-semibold text-slate-100 mb-1 flex items-center gap-1.5">
          <span class="w-2 h-2 rounded-full" :class="source === 'RULE' ? 'bg-purple-400' : 'bg-sky-400'"></span>
          <span>{{ source === 'RULE' ? 'Custom Rule Match' : 'Best Practice Heuristic' }}</span>
        </div>
        <div class="text-slate-300">{{ rationale || 'Evaluated task complexity, AST radius, and SDLC stage requirements.' }}</div>
      </div>
    </Teleport>
  </div>
</template>
