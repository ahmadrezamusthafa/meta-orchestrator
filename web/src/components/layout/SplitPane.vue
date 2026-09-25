<script setup lang="ts">
import { ref, onMounted } from 'vue'

const splitPercent = ref(60)
const isDragging = ref(false)

onMounted(() => {
  const saved = localStorage.getItem('meta_orch_split_ratio')
  if (saved) {
    const val = parseFloat(saved)
    if (!isNaN(val) && val >= 35 && val <= 75) {
      splitPercent.value = val
    }
  }
})

function startDrag(e: MouseEvent) {
  isDragging.value = true
  e.preventDefault()
  window.addEventListener('mousemove', onDrag)
  window.addEventListener('mouseup', stopDrag)
}

function onDrag(e: MouseEvent) {
  if (!isDragging.value) return
  const totalWidth = window.innerWidth - 64
  const offset = e.clientX - 64
  const newPercent = (offset / totalWidth) * 100

  if (newPercent >= 40 && newPercent <= 75) {
    splitPercent.value = newPercent
  }
}

function stopDrag() {
  if (!isDragging.value) return
  isDragging.value = false
  localStorage.setItem('meta_orch_split_ratio', splitPercent.value.toString())
  window.removeEventListener('mousemove', onDrag)
  window.removeEventListener('mouseup', stopDrag)
}
</script>

<template>
  <div class="h-full flex overflow-hidden select-none">
    <div
      class="h-full flex flex-col overflow-hidden transition-all duration-75"
      :style="{ width: `${splitPercent}%` }"
    >
      <slot name="left" />
    </div>

    <div
      class="w-1.5 bg-slate-800 hover:bg-emerald-500 cursor-col-resize transition-colors flex-shrink-0 relative group"
      @mousedown="startDrag"
    >
      <div class="absolute inset-y-0 -left-1 -right-1 z-10 cursor-col-resize"></div>
    </div>

    <div class="h-full flex-1 flex flex-col overflow-hidden bg-slate-900/40">
      <slot name="right" />
    </div>
  </div>
</template>
