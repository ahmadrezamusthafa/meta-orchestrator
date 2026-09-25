<script setup lang="ts">
import { ref } from 'vue'
import { Image, ZoomIn, X } from 'lucide-vue-next'

const selectedImage = ref<string | null>(null)

const screenshots = [
  { step: 'step_checkout_init', title: '1. Checkout Form Initialization', timestamp: '11:04:12Z', tag: 'Baseline Match' },
  { step: 'step_payment_method', title: '2. Stripe Card Element Rendered', timestamp: '11:04:28Z', tag: 'Zero Diff' },
  { step: 'step_order_confirmed', title: '3. Order Receipt & Idempotency Key', timestamp: '11:05:01Z', tag: 'Verified' },
]
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <div class="h-9 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <Image class="w-3.5 h-3.5 text-sky-400" />
        <span class="font-mono text-slate-300 font-medium">VIEWPORT SCREENSHOT AUDIT TRAIL</span>
      </div>
      <span class="text-[11px] font-mono text-slate-500">{{ screenshots.length }} milestones</span>
    </div>

    <div class="flex-1 p-4 overflow-y-auto grid grid-cols-1 gap-4">
      <div
        v-for="s in screenshots"
        :key="s.step"
        class="p-3 bg-slate-900/80 border border-slate-800 rounded-lg space-y-2 hover:border-slate-700 transition-colors"
      >
        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold text-slate-200">{{ s.title }}</span>
          <span class="px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 text-[10px] font-mono border border-emerald-800">
            {{ s.tag }}
          </span>
        </div>

        <div
          @click="selectedImage = s.step"
          class="h-36 bg-slate-950 rounded border border-slate-800 flex items-center justify-center cursor-pointer group relative overflow-hidden"
        >
          <div class="flex flex-col items-center gap-1 text-slate-600 group-hover:text-slate-400 transition-colors">
            <Image class="w-8 h-8" />
            <span class="text-[10px] font-mono">{{ s.step }}.png (1280x720)</span>
          </div>
          <div class="absolute inset-0 bg-emerald-950/20 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
            <ZoomIn class="w-6 h-6 text-emerald-400" />
          </div>
        </div>

        <div class="text-[10px] font-mono text-slate-500">
          Timestamp: {{ s.timestamp }} | SHA-256 Verified
        </div>
      </div>
    </div>

    <div
      v-if="selectedImage"
      class="fixed inset-0 z-50 flex items-center justify-center p-6 bg-slate-950/90 backdrop-blur-sm"
      @click="selectedImage = null"
    >
      <div class="relative max-w-4xl w-full bg-slate-900 border border-slate-700 rounded-xl p-4 shadow-2xl" @click.stop>
        <button
          @click="selectedImage = null"
          class="absolute top-3 right-3 text-slate-400 hover:text-white"
        >
          <X class="w-5 h-5" />
        </button>
        <div class="text-xs font-semibold text-slate-200 mb-2">
          Milestone High-Resolution Inspection: {{ selectedImage }}.png
        </div>
        <div class="h-96 bg-black rounded border border-slate-800 flex items-center justify-center text-slate-500 font-mono text-xs">
          High-Res Viewport Mockup (1280 x 720 @ 2x DPI)
        </div>
      </div>
    </div>
  </div>
</template>
