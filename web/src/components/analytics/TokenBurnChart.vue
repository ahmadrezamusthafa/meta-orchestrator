<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import type { TokenBurnPointDTO } from '../../types'
import { Flame } from 'lucide-vue-next'
import { fmtInt, fmtCompact, fmtUsd } from './analyticsFormat'

const props = defineProps<{
  points: TokenBurnPointDTO[]
  bucket?: string
}>()

const PROMPT_COLOR = '#38bdf8' // sky-400
const COMPLETION_COLOR = '#34d399' // emerald-400

const HEIGHT = 220
const PAD = { top: 12, right: 12, bottom: 28, left: 48 }

const container = ref<HTMLDivElement | null>(null)
const width = ref(600)
const hoverIndex = ref<number | null>(null)
let observer: ResizeObserver | null = null

onMounted(() => {
  if (!container.value) return
  width.value = container.value.clientWidth || 600
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w && w > 0) width.value = w
    })
    observer.observe(container.value)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})

const data = computed(() => props.points ?? [])
const plotW = computed(() => Math.max(10, width.value - PAD.left - PAD.right))
const plotH = HEIGHT - PAD.top - PAD.bottom

const maxTotal = computed(() => {
  const m = Math.max(0, ...data.value.map((p) => (p.prompt_tokens || 0) + (p.completion_tokens || 0)))
  return m > 0 ? niceCeil(m) : 1
})

function niceCeil(v: number): number {
  const exp = Math.pow(10, Math.floor(Math.log10(v)))
  const f = v / exp
  const nf = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10
  return nf * exp
}

const yTicks = computed(() => [0, 0.25, 0.5, 0.75, 1].map((t) => ({
  value: maxTotal.value * t,
  y: PAD.top + plotH - plotH * t,
})))

const slot = computed(() => (data.value.length ? plotW.value / data.value.length : 0))
const barW = computed(() => Math.max(2, Math.min(48, slot.value * 0.7)))

const bars = computed(() => data.value.map((p, i) => {
  const x = PAD.left + slot.value * i + (slot.value - barW.value) / 2
  const promptH = (p.prompt_tokens / maxTotal.value) * plotH
  const complH = (p.completion_tokens / maxTotal.value) * plotH
  const base = PAD.top + plotH
  return {
    x,
    cx: x + barW.value / 2,
    promptY: base - promptH,
    promptH,
    complY: base - promptH - complH,
    complH,
    point: p,
  }
}))

// Show at most ~8 x-axis labels to avoid overlap.
const labelEvery = computed(() => Math.max(1, Math.ceil(data.value.length / Math.max(1, Math.floor(plotW.value / 70)))))

function shortDate(d: string): string {
  if (props.bucket === 'hour' && d.length > 10) return d.slice(11, 16)
  return d.length >= 10 ? d.slice(5, 10) : d
}

const hovered = computed(() => (hoverIndex.value !== null ? bars.value[hoverIndex.value] : null))

const tooltipStyle = computed(() => {
  if (!hovered.value) return {}
  const left = Math.min(Math.max(hovered.value.cx, 90), width.value - 90)
  return { left: `${left}px`, top: `${PAD.top}px` }
})

const totals = computed(() => data.value.reduce(
  (acc, p) => {
    acc.prompt += p.prompt_tokens || 0
    acc.completion += p.completion_tokens || 0
    acc.cost += p.cost_usd || 0
    return acc
  },
  { prompt: 0, completion: 0, cost: 0 },
))
</script>

<template>
  <section class="p-4 bg-slate-900/80 border border-slate-800 rounded-xl space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2 pb-2 border-b border-slate-800">
      <div>
        <h3 class="text-xs font-bold text-slate-100 uppercase tracking-wide">Token Burn Rate</h3>
        <span class="text-[11px] text-slate-400">
          Prompt vs completion tokens per {{ bucket || 'bucket' }} &middot;
          <span class="text-sky-400 font-mono text-xs">{{ fmtCompact(totals.prompt + totals.completion) }}</span> tok,
          <span class="text-sky-400 font-mono text-xs">{{ fmtUsd(totals.cost) }}</span>
        </span>
      </div>
      <div class="flex items-center gap-3 text-[10px] font-mono">
        <span class="flex items-center gap-1"><span class="w-2 h-2 rounded-sm bg-sky-400"></span><span class="text-slate-400">Prompt</span></span>
        <span class="flex items-center gap-1"><span class="w-2 h-2 rounded-sm bg-emerald-400"></span><span class="text-slate-400">Completion</span></span>
        <Flame class="w-4 h-4 text-amber-400" />
      </div>
    </div>

    <div ref="container" class="relative w-full" :style="{ height: `${HEIGHT}px` }" @mouseleave="hoverIndex = null">
      <div v-if="!data.length" class="absolute inset-0 flex items-center justify-center text-xs text-slate-500">
        No token consumption recorded for this window.
      </div>

      <svg v-else :width="width" :height="HEIGHT" class="block" role="img" aria-label="Token burn stacked bar chart">
        <!-- grid + y axis -->
        <g v-for="t in yTicks" :key="t.y">
          <line :x1="PAD.left" :x2="width - PAD.right" :y1="t.y" :y2="t.y" stroke="#1e293b" stroke-width="1" />
          <text :x="PAD.left - 6" :y="t.y + 3" text-anchor="end" class="fill-slate-500 font-mono" font-size="9">
            {{ fmtCompact(t.value) }}
          </text>
        </g>

        <!-- bars -->
        <g v-for="(b, i) in bars" :key="b.point.date">
          <rect
            :x="PAD.left + slot * i"
            :y="PAD.top"
            :width="slot"
            :height="plotH"
            :fill="hoverIndex === i ? '#1e293b' : 'transparent'"
            fill-opacity="0.5"
            @mouseenter="hoverIndex = i"
          />
          <rect :x="b.x" :y="b.promptY" :width="barW" :height="b.promptH" :fill="PROMPT_COLOR" rx="1" pointer-events="none" />
          <rect :x="b.x" :y="b.complY" :width="barW" :height="b.complH" :fill="COMPLETION_COLOR" rx="1" pointer-events="none" />
          <text
            v-if="i % labelEvery === 0"
            :x="b.cx"
            :y="HEIGHT - PAD.bottom + 14"
            text-anchor="middle"
            class="fill-slate-500 font-mono"
            font-size="9"
          >
            {{ shortDate(b.point.date) }}
          </text>
        </g>
      </svg>

      <div
        v-if="hovered"
        class="absolute -translate-x-1/2 pointer-events-none z-10 px-2.5 py-2 rounded-lg bg-slate-950 border border-slate-700 shadow-xl text-[11px] font-mono space-y-0.5 whitespace-nowrap"
        :style="tooltipStyle"
      >
        <div class="text-slate-200 font-semibold">{{ hovered.point.date }}</div>
        <div class="text-slate-400">Prompt: <span class="text-sky-400">{{ fmtInt(hovered.point.prompt_tokens) }}</span></div>
        <div class="text-slate-400">Completion: <span class="text-emerald-400">{{ fmtInt(hovered.point.completion_tokens) }}</span></div>
        <div v-if="hovered.point.cached_tokens" class="text-slate-400">Cached: <span class="text-slate-300">{{ fmtInt(hovered.point.cached_tokens) }}</span></div>
        <div class="text-slate-400">Cost: <span class="text-amber-400">{{ fmtUsd(hovered.point.cost_usd) }}</span></div>
      </div>
    </div>
  </section>
</template>
