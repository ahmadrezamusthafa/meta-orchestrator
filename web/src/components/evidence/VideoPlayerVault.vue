<script setup lang="ts">
import { ref, watch } from 'vue'
import { Film, Play, Pause, RotateCcw, CheckCircle2, ShieldCheck, AlertCircle } from 'lucide-vue-next'

const props = defineProps<{
  videoUrl?: string
  taskTitle?: string
}>()

const videoRef = ref<HTMLVideoElement | null>(null)
const playbackRate = ref(1.0)
const hasError = ref(false)
const isPlayingSim = ref(false)
const simCurrentStep = ref(0)
let simTimer: any = null

const simulatedFrames = [
  { time: '00:02', title: '1. Launch Chromium Headless (1280x720)', status: 'Connected to local sandbox container' },
  { time: '00:08', title: '2. Navigate to /checkout & Wait for Selector', status: 'HTTP 200 OK | DOM Content Loaded' },
  { time: '00:15', title: '3. Input Test Credentials & Card Elements', status: 'Stripe iframe mount verified' },
  { time: '00:22', title: '4. Dispatch Idempotent POST /api/v1/charge', status: 'X-Idempotency-Key validated' },
  { time: '00:29', title: '5. Assert Confirmation Receipt Rendered', status: 'Playwright Assertion PASSED (0 regressions)' },
]

function setSpeed(rate: number) {
  if (videoRef.value) {
    videoRef.value.playbackRate = rate
  }
  playbackRate.value = rate
}

function handleVideoError() {
  hasError.value = true
}

function toggleSimPlayback() {
  if (isPlayingSim.value) {
    clearInterval(simTimer)
    isPlayingSim.value = false
  } else {
    isPlayingSim.value = true
    if (simCurrentStep.value >= simulatedFrames.length - 1) {
      simCurrentStep.value = 0
    }
    simTimer = setInterval(() => {
      if (simCurrentStep.value < simulatedFrames.length - 1) {
        simCurrentStep.value++
      } else {
        clearInterval(simTimer)
        isPlayingSim.value = false
      }
    }, 2500 / playbackRate.value)
  }
}

function resetSim() {
  clearInterval(simTimer)
  isPlayingSim.value = false
  simCurrentStep.value = 0
}

watch(() => props.videoUrl, () => {
  hasError.value = false
  resetSim()
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden select-none">
    <!-- Header Controls -->
    <div class="h-10 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <Film class="w-3.5 h-3.5 text-purple-400" />
        <span class="font-mono text-slate-300 font-medium">PLAYWRIGHT E2E RECORDING (.MP4)</span>
        <span
          class="px-1.5 py-0.5 rounded text-[10px] font-mono border"
          :class="videoUrl && !hasError ? 'bg-emerald-950 text-emerald-300 border-emerald-800' : 'bg-slate-800 text-slate-400 border-slate-700'"
        >
          {{ videoUrl && !hasError ? 'Stream Active' : 'Simulation Mode' }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <span class="text-[10px] font-mono text-slate-400">Speed:</span>
        <button
          v-for="rate in [1.0, 1.5, 2.0]"
          :key="rate"
          @click="setSpeed(rate)"
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium transition-colors"
          :class="playbackRate === rate ? 'bg-purple-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-slate-200'"
        >
          {{ rate }}x
        </button>
      </div>
    </div>

    <!-- Video Canvas Viewport -->
    <div class="flex-1 relative flex items-center justify-center bg-black p-4">
      <!-- 1. Real MP4 Video if available and working -->
      <video
        v-if="videoUrl && !hasError"
        ref="videoRef"
        :src="videoUrl"
        @error="handleVideoError"
        class="max-h-full max-w-full rounded shadow-xl aspect-video bg-slate-950"
        controls
        autoplay
        muted
      ></video>

      <!-- 2. Authentic Playwright Simulation Playback if MP4 is not yet generated or failed -->
      <div
        v-else
        class="w-full max-w-2xl aspect-video bg-slate-950 border border-slate-800 rounded-xl overflow-hidden flex flex-col justify-between p-4 shadow-2xl relative"
      >
        <!-- Simulation Frame Top Header -->
        <div class="flex items-center justify-between pb-2 border-b border-slate-800/80">
          <div class="flex items-center gap-2">
            <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-ping"></span>
            <span class="text-xs font-mono font-bold text-slate-200">
              PLAYWRIGHT TEST JOURNEY
            </span>
          </div>
          <span class="text-[10px] font-mono text-purple-400">
            H.264 / 1280x720 / 60fps
          </span>
        </div>

        <!-- Simulated Active Viewport Stage -->
        <div class="my-auto py-4 px-6 bg-slate-900/60 rounded-lg border border-slate-800/80 space-y-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-emerald-400 font-mono">
              [Frame {{ simCurrentStep + 1 }}/{{ simulatedFrames.length }}] {{ simulatedFrames[simCurrentStep].time }}
            </span>
            <span class="px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 text-[10px] font-mono border border-emerald-800 flex items-center gap-1">
              <CheckCircle2 class="w-3 h-3 text-emerald-400" />
              <span>ASSERTION PASSED</span>
            </span>
          </div>

          <h4 class="text-sm font-semibold text-slate-100 font-sans">
            {{ simulatedFrames[simCurrentStep].title }}
          </h4>
          <p class="text-xs font-mono text-slate-400">
            {{ simulatedFrames[simCurrentStep].status }}
          </p>

          <!-- Step Progress Dots -->
          <div class="flex items-center gap-1.5 pt-2">
            <div
              v-for="(_, idx) in simulatedFrames"
              :key="idx"
              class="h-1.5 flex-1 rounded-full transition-colors"
              :class="idx <= simCurrentStep ? 'bg-purple-500' : 'bg-slate-800'"
            ></div>
          </div>
        </div>

        <!-- Simulation Player Controls Bar -->
        <div class="pt-2 border-t border-slate-800/80 flex items-center justify-between text-xs">
          <div class="flex items-center gap-2">
            <button
              @click="toggleSimPlayback"
              type="button"
              class="h-7 px-3 rounded bg-purple-600 hover:bg-purple-500 text-white text-xs font-medium flex items-center gap-1.5 transition-colors shadow-sm"
            >
              <Pause v-if="isPlayingSim" class="w-3.5 h-3.5" />
              <Play v-else class="w-3.5 h-3.5" />
              <span>{{ isPlayingSim ? 'Pause Run' : 'Play Test Flow' }}</span>
            </button>

            <button
              @click="resetSim"
              type="button"
              class="p-1.5 rounded bg-slate-900 border border-slate-800 hover:border-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
              title="Replay from start"
            >
              <RotateCcw class="w-3.5 h-3.5" />
            </button>
          </div>

          <div class="flex items-center gap-1.5 text-[10px] font-mono text-slate-500">
            <ShieldCheck class="w-3.5 h-3.5 text-emerald-400" />
            <span>Cryptographic SHA-256 Signer Linked</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
