<script setup lang="ts">
import { ref } from 'vue'
import { Film } from 'lucide-vue-next'

const props = defineProps<{
  videoUrl?: string
}>()

const videoRef = ref<HTMLVideoElement | null>(null)
const playbackRate = ref(1.0)

function setSpeed(rate: number) {
  if (!videoRef.value) return
  videoRef.value.playbackRate = rate
  playbackRate.value = rate
}
</script>

<template>
  <div class="h-full flex flex-col bg-slate-950 overflow-hidden">
    <div class="h-9 px-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between text-xs">
      <div class="flex items-center gap-2">
        <Film class="w-3.5 h-3.5 text-purple-400" />
        <span class="font-mono text-slate-300 font-medium">PLAYWRIGHT E2E RECORDING (.MP4)</span>
      </div>
      <div v-if="videoUrl" class="flex items-center gap-2">
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

    <div class="flex-1 relative flex items-center justify-center bg-black p-4">
      <video
        v-if="videoUrl"
        ref="videoRef"
        :src="videoUrl"
        class="max-h-full max-w-full rounded shadow-xl aspect-video bg-slate-950"
        controls
      ></video>

      <div
        v-else
        class="flex flex-col items-center justify-center p-8 text-center text-slate-500 space-y-2"
      >
        <Film class="w-8 h-8 text-slate-700" />
        <span class="text-xs font-medium text-slate-400">No MP4 recorded for this test execution run</span>
        <span class="text-[11px] text-slate-600 max-w-xs">
          Enable "Record Playwright .mp4 video" in task settings or run Stage 6 (Automation & E2E Validation)
        </span>
      </div>
    </div>
  </div>
</template>
