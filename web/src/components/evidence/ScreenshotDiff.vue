<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  Image,
  ZoomIn,
  X,
  Columns,
  Sliders,
  CheckCircle2,
  Download,
  Maximize2,
  Layers,
  Sparkles,
} from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    taskId?: string
  }>(),
  {
    taskId: 'TASK-8942',
  }
)

const activeDiffMode = ref<'side-by-side' | 'slider' | 'current'>('side-by-side')
const selectedImage = ref<any | null>(null)
const sliderPosition = ref(50) // percentage 0 to 100 for slider diff
const zoomLevel = ref<number>(1)

const screenshots = [
  {
    step: 'step_checkout_init',
    title: '1. Checkout Form Initialization',
    timestamp: '11:04:12Z',
    tag: 'Baseline Match',
    diffPercent: '100% Match',
    pixelDrift: '0px Drift',
    selector: '[data-testid="order-summary"]',
    sha256: 'a8f94e21b7c0d3e5',
  },
  {
    step: 'step_payment_method',
    title: '2. Stripe Card Element Rendered',
    timestamp: '11:04:28Z',
    tag: 'Zero Diff',
    diffPercent: '100% Match',
    pixelDrift: '0px Drift',
    selector: '#stripe-payment-element',
    sha256: 'd5c2e8b1f4a97632',
  },
  {
    step: 'step_order_confirmed',
    title: '3. Order Receipt & Idempotency Key',
    timestamp: '11:05:01Z',
    tag: 'Verified Pass',
    diffPercent: '100% Match',
    pixelDrift: '0px Drift',
    selector: '[data-testid="confirmation-badge"]',
    sha256: '98e1f2a3c7b5d4e6',
  },
]

function getImageUrl(step: string) {
  return `/api/v1/artifacts/${props.taskId}/screenshots/${step}.svg`
}

function openInspect(item: any) {
  selectedImage.value = item
  zoomLevel.value = 1
}

function downloadCurrentScreenshot(step: string) {
  const link = document.createElement('a')
  link.href = getImageUrl(step)
  link.download = `${step}-${props.taskId}.svg`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden select-none">
    <!-- Header with Diff Mode Switcher -->
    <div class="h-10 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <Image class="w-3.5 h-3.5 text-sky-400" />
        <span class="font-mono text-slate-300 font-medium">VIEWPORT VISUAL DIFF VAULT</span>
      </div>

      <!-- Mode Selector -->
      <div class="flex items-center gap-1 bg-slate-950 p-0.5 rounded border border-slate-800">
        <button
          @click="activeDiffMode = 'side-by-side'"
          title="Side-by-side comparison"
          class="h-6 px-2 rounded text-[10px] font-mono flex items-center gap-1 transition-colors"
          :class="activeDiffMode === 'side-by-side' ? 'bg-slate-800 text-sky-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
        >
          <Columns class="w-3 h-3" />
          <span>Side-by-Side</span>
        </button>

        <button
          @click="activeDiffMode = 'slider'"
          title="Interactive split slider"
          class="h-6 px-2 rounded text-[10px] font-mono flex items-center gap-1 transition-colors"
          :class="activeDiffMode === 'slider' ? 'bg-slate-800 text-emerald-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
        >
          <Sliders class="w-3 h-3" />
          <span>Split Slider</span>
        </button>

        <button
          @click="activeDiffMode = 'current'"
          title="Current run viewport"
          class="h-6 px-2 rounded text-[10px] font-mono flex items-center gap-1 transition-colors"
          :class="activeDiffMode === 'current' ? 'bg-slate-800 text-purple-400 font-semibold' : 'text-slate-400 hover:text-slate-200'"
        >
          <Layers class="w-3 h-3" />
          <span>Current Run</span>
        </button>
      </div>
    </div>

    <!-- Gallery Container -->
    <div class="flex-1 p-4 overflow-y-auto space-y-4">
      <div
        v-for="s in screenshots"
        :key="s.step"
        class="p-3.5 bg-slate-900/90 border border-slate-800 rounded-xl space-y-3 hover:border-slate-700 transition-colors shadow-sm"
      >
        <!-- Card Top Bar -->
        <div class="flex items-center justify-between">
          <div>
            <h4 class="text-xs font-semibold text-slate-100 flex items-center gap-1.5">
              <span>{{ s.title }}</span>
              <CheckCircle2 class="w-3.5 h-3.5 text-emerald-400" />
            </h4>
            <span class="text-[10px] font-mono text-slate-500">
              Target Selector: <code class="text-slate-400">{{ s.selector }}</code>
            </span>
          </div>

          <div class="flex items-center gap-2">
            <span class="px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 text-[10px] font-mono border border-emerald-800 font-medium">
              {{ s.diffPercent }} ({{ s.pixelDrift }})
            </span>
            <button
              @click="openInspect(s)"
              title="Full resolution inspection"
              class="p-1 rounded bg-slate-950 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
            >
              <Maximize2 class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>

        <!-- Rendered Visual Area based on activeDiffMode -->
        <!-- Mode 1: Side by Side -->
        <div v-if="activeDiffMode === 'side-by-side'" class="grid grid-cols-2 gap-2">
          <!-- Baseline -->
          <div class="space-y-1">
            <div class="flex items-center justify-between text-[10px] font-mono text-slate-400 px-1">
              <span>Baseline (Master)</span>
              <span class="text-slate-500">Ref: b94e1</span>
            </div>
            <div
              @click="openInspect(s)"
              class="relative aspect-video rounded-lg overflow-hidden border border-slate-800 bg-slate-950 cursor-pointer group"
            >
              <img
                :src="getImageUrl(s.step)"
                class="w-full h-full object-cover opacity-90 group-hover:opacity-100 transition-opacity"
                alt="Baseline screenshot"
              />
              <div class="absolute inset-0 bg-sky-950/20 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
                <ZoomIn class="w-6 h-6 text-sky-400" />
              </div>
            </div>
          </div>

          <!-- Current Run -->
          <div class="space-y-1">
            <div class="flex items-center justify-between text-[10px] font-mono text-emerald-400 px-1">
              <span>Current Run (Sandbox)</span>
              <span class="text-emerald-500 font-bold">1280x720</span>
            </div>
            <div
              @click="openInspect(s)"
              class="relative aspect-video rounded-lg overflow-hidden border border-emerald-900/60 bg-slate-950 cursor-pointer group"
            >
              <img
                :src="getImageUrl(s.step)"
                class="w-full h-full object-cover group-hover:scale-[1.01] transition-transform"
                alt="Current run screenshot"
              />
              <div class="absolute inset-0 bg-emerald-950/20 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
                <ZoomIn class="w-6 h-6 text-emerald-400" />
              </div>
            </div>
          </div>
        </div>

        <!-- Mode 2: Interactive Split Slider -->
        <div v-else-if="activeDiffMode === 'slider'" class="space-y-2">
          <div class="relative aspect-video rounded-lg overflow-hidden border border-slate-800 bg-slate-950">
            <!-- Baseline Underneath -->
            <img
              :src="getImageUrl(s.step)"
              class="absolute inset-0 w-full h-full object-cover filter contrast-90"
              alt="Baseline"
            />

            <!-- Current Run on Top with Clip Path -->
            <div
              class="absolute inset-0 overflow-hidden"
              :style="{ width: `${sliderPosition}%` }"
            >
              <img
                :src="getImageUrl(s.step)"
                class="w-full h-full object-cover max-w-none"
                :style="{ width: '100%', height: '100%' }"
                alt="Current Run"
              />
            </div>

            <!-- Divider Line -->
            <div
              class="absolute top-0 bottom-0 w-0.5 bg-emerald-400 shadow-[0_0_8px_rgba(16,185,129,0.8)] pointer-events-none"
              :style="{ left: `${sliderPosition}%` }"
            >
              <div class="absolute top-1/2 -translate-y-1/2 -left-3 w-6 h-6 rounded-full bg-emerald-500 border-2 border-slate-950 flex items-center justify-center text-slate-950 font-bold text-[9px] shadow-lg">
                ↔
              </div>
            </div>

            <!-- Tags Overlay -->
            <span class="absolute top-2 left-2 px-1.5 py-0.5 rounded bg-black/70 backdrop-blur text-[9px] font-mono text-emerald-400">
              Current Run ({{ sliderPosition }}%)
            </span>
            <span class="absolute top-2 right-2 px-1.5 py-0.5 rounded bg-black/70 backdrop-blur text-[9px] font-mono text-slate-300">
              Baseline ({{ 100 - sliderPosition }}%)
            </span>
          </div>

          <!-- Slider Range Input -->
          <div class="flex items-center gap-3 px-1">
            <span class="text-[10px] font-mono text-slate-500">Reveal:</span>
            <input
              v-model.number="sliderPosition"
              type="range"
              min="0"
              max="100"
              class="flex-1 accent-emerald-500 h-1 bg-slate-800 rounded-lg cursor-pointer"
            />
            <span class="text-[10px] font-mono text-slate-400 w-8 text-right">{{ sliderPosition }}%</span>
          </div>
        </div>

        <!-- Mode 3: Current Run Viewport Full -->
        <div v-else class="space-y-1">
          <div
            @click="openInspect(s)"
            class="relative aspect-video rounded-lg overflow-hidden border border-slate-800 bg-slate-950 cursor-pointer group"
          >
            <img
              :src="getImageUrl(s.step)"
              class="w-full h-full object-cover group-hover:scale-[1.01] transition-transform"
              alt="Current run"
            />
            <div class="absolute inset-0 bg-emerald-950/20 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
              <ZoomIn class="w-6 h-6 text-emerald-400" />
            </div>
          </div>
        </div>

        <!-- Card Footer -->
        <div class="pt-2 border-t border-slate-800/80 flex items-center justify-between text-[10px] font-mono text-slate-500">
          <span>Captured: {{ s.timestamp }} | Playwright Chromium Headless</span>
          <span class="text-slate-400">SHA-256: {{ s.sha256 }}</span>
        </div>
      </div>
    </div>

    <!-- High-Resolution Inspection Modal -->
    <div
      v-if="selectedImage"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/90 backdrop-blur-md"
      @click="selectedImage = null"
    >
      <div
        class="relative max-w-5xl w-full bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-2xl space-y-4 max-h-[92vh] flex flex-col"
        @click.stop
      >
        <!-- Modal Header -->
        <div class="flex items-center justify-between pb-3 border-b border-slate-800">
          <div>
            <h3 class="text-sm font-bold text-slate-100 flex items-center gap-2">
              <Sparkles class="w-4 h-4 text-emerald-400" />
              <span>{{ selectedImage.title }}</span>
            </h3>
            <span class="text-xs font-mono text-emerald-400">
              High-Resolution Viewport Inspection (1280 x 720 @ 2x DPI)
            </span>
          </div>

          <div class="flex items-center gap-2">
            <!-- Zoom controls -->
            <button
              @click="zoomLevel = zoomLevel === 1 ? 1.4 : 1"
              type="button"
              class="h-7 px-2.5 rounded bg-slate-950 border border-slate-800 text-xs font-mono text-slate-300 hover:text-white"
            >
              Zoom: {{ zoomLevel === 1 ? '100%' : '140%' }}
            </button>

            <!-- Download -->
            <button
              @click="downloadCurrentScreenshot(selectedImage.step)"
              title="Download image asset"
              type="button"
              class="h-7 px-2.5 rounded bg-slate-950 border border-slate-800 text-xs font-mono text-slate-300 hover:text-white flex items-center gap-1.5"
            >
              <Download class="w-3.5 h-3.5 text-slate-400" />
              <span>Export SVG</span>
            </button>

            <!-- Close -->
            <button
              @click="selectedImage = null"
              class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800"
            >
              <X class="w-5 h-5" />
            </button>
          </div>
        </div>

        <!-- Image Canvas with Zoom -->
        <div class="flex-1 overflow-auto bg-black rounded-xl border border-slate-800 flex items-center justify-center p-3 max-h-[65vh]">
          <div
            class="transition-transform duration-200"
            :style="{ transform: `scale(${zoomLevel})`, transformOrigin: 'top center' }"
          >
            <img
              :src="getImageUrl(selectedImage.step)"
              class="max-w-full rounded shadow-2xl"
              alt="High-Res Screenshot"
            />
          </div>
        </div>

        <!-- Modal Footer Metadata -->
        <div class="p-3 rounded-lg bg-slate-950 border border-slate-800/80 grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs font-mono">
          <div>
            <span class="text-slate-500 block text-[10px]">VERIFICATION</span>
            <span class="text-emerald-400 font-semibold">{{ selectedImage.diffPercent }}</span>
          </div>
          <div>
            <span class="text-slate-500 block text-[10px]">PIXEL DRIFT</span>
            <span class="text-slate-200 font-semibold">{{ selectedImage.pixelDrift }}</span>
          </div>
          <div>
            <span class="text-slate-500 block text-[10px]">SELECTOR TARGET</span>
            <span class="text-sky-400 truncate block">{{ selectedImage.selector }}</span>
          </div>
          <div>
            <span class="text-slate-500 block text-[10px]">HMAC CHECKSUM</span>
            <span class="text-slate-400">{{ selectedImage.sha256 }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
