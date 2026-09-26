<script setup lang="ts">
import { useToastStore } from '../../stores/toast'
import { CheckCircle2, AlertCircle, Info, AlertTriangle, X } from 'lucide-vue-next'

const toastStore = useToastStore()
</script>

<template>
  <div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none">
    <transition-group
      enter-active-class="transform ease-out duration-300 transition"
      enter-from-class="translate-y-2 opacity-0 sm:translate-y-0 sm:translate-x-2"
      enter-to-class="translate-y-0 opacity-100 sm:translate-x-0"
      leave-active-class="transition ease-in duration-200"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-for="toast in toastStore.toasts"
        :key="toast.id"
        class="pointer-events-auto p-3 rounded-lg border shadow-xl backdrop-blur-md flex items-start gap-3 transition-all"
        :class="{
          'bg-emerald-950/90 border-emerald-800 text-emerald-200': toast.type === 'success',
          'bg-rose-950/90 border-rose-800 text-rose-200': toast.type === 'error',
          'bg-amber-950/90 border-amber-800 text-amber-200': toast.type === 'warning',
          'bg-slate-900/90 border-slate-700 text-slate-200': toast.type === 'info',
        }"
      >
        <div class="flex-shrink-0 mt-0.5">
          <CheckCircle2 v-if="toast.type === 'success'" class="w-4 h-4 text-emerald-400" />
          <AlertCircle v-else-if="toast.type === 'error'" class="w-4 h-4 text-rose-400" />
          <AlertTriangle v-else-if="toast.type === 'warning'" class="w-4 h-4 text-amber-400" />
          <Info v-else class="w-4 h-4 text-sky-400" />
        </div>

        <div class="flex-1 min-w-0">
          <h5 class="text-xs font-semibold leading-snug">{{ toast.title }}</h5>
          <p v-if="toast.description" class="text-[11px] opacity-90 mt-0.5 leading-normal">
            {{ toast.description }}
          </p>
        </div>

        <button
          @click="toastStore.dismiss(toast.id)"
          type="button"
          class="flex-shrink-0 text-slate-400 hover:text-white transition-colors"
        >
          <X class="w-3.5 h-3.5" />
        </button>
      </div>
    </transition-group>
  </div>
</template>
