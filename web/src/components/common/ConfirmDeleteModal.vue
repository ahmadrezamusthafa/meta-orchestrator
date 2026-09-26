<script setup lang="ts">
import { AlertTriangle, Trash2, X } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    isOpen: boolean
    title?: string
    message?: string
    itemName?: string
    confirmText?: string
    cancelText?: string
    loading?: boolean
    note?: string
  }>(),
  {
    isOpen: false,
    title: 'Confirm Deletion',
    message: 'Are you sure you want to proceed? This action cannot be undone.',
    itemName: '',
    confirmText: 'Delete',
    cancelText: 'Cancel',
    loading: false,
    note: 'Your physical host source code remains completely safe on disk.'
  }
)

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'close'): void
}>()
</script>

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 overflow-y-auto"
    @click.self="emit('close')"
  >
    <div
      class="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-md shadow-2xl overflow-hidden ring-1 ring-rose-500/20 animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Header -->
      <div class="p-5 border-b border-slate-800/80 flex items-start justify-between bg-slate-950/40">
        <div class="flex items-center gap-3">
          <div class="p-2.5 rounded-xl bg-rose-500/10 text-rose-400 border border-rose-500/20">
            <AlertTriangle class="w-5 h-5 text-rose-400" />
          </div>
          <div>
            <h3 class="font-bold text-base text-white">{{ title }}</h3>
            <p class="text-xs text-slate-400 mt-0.5">Destructive action confirmation</p>
          </div>
        </div>
        <button
          @click="emit('close')"
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Body -->
      <div class="p-6 space-y-4 text-xs">
        <p class="text-slate-300 leading-relaxed">{{ message }}</p>

        <div v-if="itemName" class="p-3 rounded-xl bg-slate-950 border border-slate-800/80 space-y-1">
          <span class="text-[10px] uppercase font-mono text-slate-500 font-semibold">Target Item:</span>
          <div class="font-mono text-sm font-bold text-rose-300 truncate">{{ itemName }}</div>
        </div>

        <div v-if="note" class="p-3 rounded-xl bg-emerald-500/5 border border-emerald-500/20 flex items-start gap-2 text-[11px] text-emerald-300/90 leading-relaxed">
          <span class="text-emerald-400 font-bold flex-shrink-0">✓</span>
          <span>{{ note }}</span>
        </div>
      </div>

      <!-- Footer -->
      <div class="p-4 border-t border-slate-800 bg-slate-950/60 flex items-center justify-end gap-2.5">
        <button
          type="button"
          @click="emit('close')"
          :disabled="loading"
          class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-semibold transition disabled:opacity-50"
        >
          {{ cancelText }}
        </button>

        <button
          type="button"
          @click="emit('confirm')"
          :disabled="loading"
          class="px-4 py-2 rounded-lg bg-rose-600 hover:bg-rose-500 active:bg-rose-700 text-white text-xs font-semibold flex items-center gap-1.5 shadow-lg shadow-rose-950/50 transition disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <svg v-if="loading" class="animate-spin h-3.5 w-3.5 text-white" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <Trash2 v-else class="w-3.5 h-3.5" />
          <span>{{ confirmText }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
