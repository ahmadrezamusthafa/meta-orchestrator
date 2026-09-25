<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const props = defineProps<{
  to: string
  label: string
  isExpanded: boolean
}>()

const route = useRoute()
const isActive = computed(() => route.path === props.to)
</script>

<template>
  <router-link
    :to="to"
    class="flex items-center gap-3 px-3.5 py-3 rounded-lg transition-colors group relative"
    :class="{
      'bg-slate-800 text-emerald-400 font-medium': isActive,
      'text-slate-400 hover:text-slate-200 hover:bg-slate-850': !isActive,
    }"
  >
    <div
      v-if="isActive"
      class="absolute left-0 top-1.5 bottom-1.5 w-1 bg-emerald-500 rounded-r"
    ></div>

    <div class="flex-shrink-0 flex items-center justify-center w-6 h-6">
      <slot />
    </div>

    <span
      v-if="isExpanded"
      class="text-xs whitespace-nowrap overflow-hidden transition-all duration-200"
    >
      {{ label }}
    </span>

    <div
      v-if="!isExpanded"
      class="absolute left-full ml-2 px-2 py-1 bg-slate-900 border border-slate-700 text-slate-100 text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity z-50 whitespace-nowrap shadow-lg"
    >
      {{ label }}
    </div>
  </router-link>
</template>
