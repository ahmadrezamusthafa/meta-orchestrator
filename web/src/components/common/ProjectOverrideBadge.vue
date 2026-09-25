<script setup lang="ts">
defineProps<{
  isOverride: boolean
}>()
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[10px] font-mono font-medium"
    :class="{
      'bg-emerald-950/80 text-emerald-300 border border-emerald-800/80': isOverride,
      'bg-slate-800 text-slate-400 border border-slate-700': !isOverride,
    }"
  >
    <span
      class="w-1.5 h-1.5 rounded-full"
      :class="isOverride ? 'bg-emerald-400' : 'bg-slate-500'"
    ></span>
    {{ isOverride ? 'Project Override (.sdlc/config.yaml)' : 'System Inherited (Global)' }}
  </span>
</template>
