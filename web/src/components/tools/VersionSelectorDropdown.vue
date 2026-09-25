<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  modelValue: string
  versions: string[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: string): void
}>()

const selected = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})
</script>

<template>
  <select
    v-model="selected"
    class="h-8 px-2.5 bg-slate-950 border border-slate-800 rounded text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
  >
    <option v-for="v in versions" :key="v" :value="v">
      {{ v }}
    </option>
  </select>
</template>
