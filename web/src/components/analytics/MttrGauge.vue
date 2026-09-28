<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { Gauge, Timer } from 'lucide-vue-next'
import {
  fpvrHealth, mttrHealth, fmtPct, fmtDuration,
  HEALTH_TEXT, HEALTH_BG, HEALTH_HEX, type Health,
} from './analyticsFormat'

const props = defineProps<{
  fpvrPercent: number
  mttrSeconds: number
  runs?: number
}>()

const FPVR_TARGET = 80
const MTTR_TARGET_MIN = 15
const DURATION_MS = 900

// Animated display values (0 → target on mount, and tween on subsequent changes).
const animFpvr = ref(0)
const animMttr = ref(0)
let rafId: number | null = null

function clampPct(v: number): number {
  return Math.min(100, Math.max(0, Number.isFinite(v) ? v : 0))
}

function animate() {
  if (rafId !== null) cancelAnimationFrame(rafId)
  const fromF = animFpvr.value
  const fromM = animMttr.value
  const toF = clampPct(props.fpvrPercent)
  const toM = Math.max(0, props.mttrSeconds || 0)
  const start = performance.now()
  const step = (now: number) => {
    const t = Math.min(1, (now - start) / DURATION_MS)
    const e = 1 - Math.pow(1 - t, 3) // easeOutCubic
    animFpvr.value = fromF + (toF - fromF) * e
    animMttr.value = fromM + (toM - fromM) * e
    rafId = t < 1 ? requestAnimationFrame(step) : null
  }
  rafId = requestAnimationFrame(step)
}

onMounted(animate)
watch(() => [props.fpvrPercent, props.mttrSeconds], animate)
onBeforeUnmount(() => {
  if (rafId !== null) cancelAnimationFrame(rafId)
})

const noData = computed(() => props.runs !== undefined && props.runs === 0)

const fpvrState = computed<Health>(() => fpvrHealth(props.fpvrPercent || 0))
const mttrState = computed<Health>(() => mttrHealth(props.mttrSeconds || 0))

// Semicircle: centre (60,60), radius 50, from 180° to 0°. pathLength normalised to 100.
const ARC = 'M 10 60 A 50 50 0 0 1 110 60'
const dashOffset = computed(() => 100 - animFpvr.value)

// Target marker at 80% along the arc.
const targetMarker = computed(() => {
  const angle = Math.PI * (1 - FPVR_TARGET / 100)
  const x1 = 60 + 44 * Math.cos(angle)
  const y1 = 60 - 44 * Math.sin(angle)
  const x2 = 60 + 57 * Math.cos(angle)
  const y2 = 60 - 57 * Math.sin(angle)
  return { x1, y1, x2, y2 }
})

// MTTR bar: scale 0..30 min, target line at 15 min.
const MTTR_SCALE_MIN = 30
const mttrBarPct = computed(() => Math.min(100, ((animMttr.value / 60) / MTTR_SCALE_MIN) * 100))
const MTTR_BAR_FILL: Record<Health, string> = {
  pass: 'bg-emerald-500',
  warn: 'bg-amber-500',
  breach: 'bg-rose-500',
}

const STATE_LABEL: Record<Health, string> = {
  pass: 'On target',
  warn: 'Warning',
  breach: 'Target breached',
}
</script>

<template>
  <section class="grid grid-cols-1 sm:grid-cols-2 gap-4">
    <!-- FPVR gauge -->
    <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-2">
      <div class="flex items-center justify-between pb-2 border-b border-slate-800">
        <div>
          <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">First-Pass Verification</h3>
          <span class="text-[11px] text-slate-400">Target &gt; {{ FPVR_TARGET }}%</span>
        </div>
        <Gauge class="w-4 h-4" :class="HEALTH_TEXT[fpvrState]" />
      </div>

      <div class="relative flex flex-col items-center">
        <svg viewBox="0 0 120 70" class="w-full max-w-[240px]" role="img" :aria-label="`FPVR ${fmtPct(fpvrPercent)}`">
          <path :d="ARC" fill="none" stroke="#1e293b" stroke-width="10" stroke-linecap="round" pathLength="100" />
          <path
            :d="ARC"
            fill="none"
            :stroke="noData ? '#334155' : HEALTH_HEX[fpvrState]"
            stroke-width="10"
            stroke-linecap="round"
            pathLength="100"
            stroke-dasharray="100 100"
            :stroke-dashoffset="dashOffset"
          />
          <line v-bind="targetMarker" stroke="#e2e8f0" stroke-width="1.2" stroke-opacity="0.7" />
        </svg>
        <div class="absolute bottom-1 text-center">
          <div class="text-2xl font-bold font-mono" :class="noData ? 'text-slate-500' : HEALTH_TEXT[fpvrState]">
            {{ noData ? '—' : fmtPct(animFpvr) }}
          </div>
        </div>
      </div>
      <div class="flex justify-center">
        <span
          v-if="!noData"
          class="px-2 py-0.5 rounded border text-[10px] font-mono uppercase"
          :class="[HEALTH_BG[fpvrState], HEALTH_TEXT[fpvrState]]"
        >{{ STATE_LABEL[fpvrState] }}</span>
        <span v-else class="text-[10px] font-mono text-slate-500 uppercase">No runs yet</span>
      </div>
    </div>

    <!-- MTTR card -->
    <div class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
      <div class="flex items-center justify-between pb-2 border-b border-slate-800">
        <div>
          <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">Mean Time to Resolution</h3>
          <span class="text-[11px] text-slate-400">Target &lt; {{ MTTR_TARGET_MIN }} minutes</span>
        </div>
        <Timer class="w-4 h-4" :class="HEALTH_TEXT[mttrState]" />
      </div>

      <div class="pt-2">
        <div class="text-3xl font-bold font-mono" :class="noData ? 'text-slate-500' : HEALTH_TEXT[mttrState]">
          {{ noData ? '—' : fmtDuration(animMttr) }}
        </div>
        <div class="text-[11px] text-slate-400 mt-1">average across resolved tasks</div>
      </div>

      <div class="space-y-1">
        <div class="relative h-2 rounded-full bg-slate-800 overflow-hidden">
          <div
            class="absolute inset-y-0 left-0 rounded-full"
            :class="noData ? 'bg-slate-700' : MTTR_BAR_FILL[mttrState]"
            :style="{ width: `${mttrBarPct}%` }"
          ></div>
          <!-- 15 min target line at 50% of a 30-min scale -->
          <div class="absolute inset-y-0 left-1/2 w-px bg-slate-200/70"></div>
        </div>
        <div class="flex justify-between text-[9px] font-mono text-slate-500">
          <span>0m</span><span>15m</span><span>30m+</span>
        </div>
      </div>

      <span
        v-if="!noData"
        class="inline-block px-2 py-0.5 rounded border text-[10px] font-mono uppercase"
        :class="[HEALTH_BG[mttrState], HEALTH_TEXT[mttrState]]"
      >{{ STATE_LABEL[mttrState] }}</span>
    </div>
  </section>
</template>
