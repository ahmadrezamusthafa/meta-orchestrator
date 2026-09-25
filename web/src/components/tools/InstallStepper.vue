<script setup lang="ts">
import { Check } from 'lucide-vue-next'

defineProps<{
  currentStep: number
}>()

const steps = [
  '1. Pre-flight Checks',
  '2. Fetch & Build',
  '3. Diagnostics',
  '4. Ready',
]
</script>

<template>
  <div class="flex items-center justify-between">
    <div
      v-for="(name, idx) in steps"
      :key="name"
      class="flex items-center gap-2"
    >
      <div
        class="w-6 h-6 rounded-full flex items-center justify-center text-[11px] font-mono font-semibold"
        :class="{
          'bg-emerald-600 text-white': currentStep > idx + 1,
          'bg-emerald-500 text-slate-950 font-bold ring-4 ring-emerald-500/20': currentStep === idx + 1,
          'bg-slate-800 text-slate-400': currentStep < idx + 1,
        }"
      >
        <Check v-if="currentStep > idx + 1" class="w-3.5 h-3.5" />
        <span v-else>{{ idx + 1 }}</span>
      </div>

      <span
        class="text-xs font-medium"
        :class="currentStep >= idx + 1 ? 'text-slate-200' : 'text-slate-500'"
      >
        {{ name }}
      </span>

      <div
        v-if="idx < steps.length - 1"
        class="w-8 h-px bg-slate-800 mx-2"
      ></div>
    </div>
  </div>
</template>
