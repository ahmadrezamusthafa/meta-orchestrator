// Shared formatting + color helpers for the analytics dashboard.
// All Tailwind classes are written as full literal strings so the JIT picks them up.

export type Health = 'pass' | 'warn' | 'breach'

export const METHODS = ['BMAD', 'Supervisor', 'ReAct', 'Superpower'] as const

export interface MethodStyle {
  text: string
  bg: string
  border: string
  fill: string // hex for SVG
}

const METHOD_STYLES: Record<string, MethodStyle> = {
  BMAD: { text: 'text-emerald-400', bg: 'bg-emerald-950/60', border: 'border-emerald-800', fill: '#34d399' },
  Supervisor: { text: 'text-sky-400', bg: 'bg-sky-950/60', border: 'border-sky-800', fill: '#38bdf8' },
  ReAct: { text: 'text-purple-400', bg: 'bg-purple-950/60', border: 'border-purple-800', fill: '#c084fc' },
  Superpower: { text: 'text-amber-400', bg: 'bg-amber-950/60', border: 'border-amber-800', fill: '#fbbf24' },
}

const FALLBACK_STYLE: MethodStyle = {
  text: 'text-slate-300',
  bg: 'bg-slate-900',
  border: 'border-slate-700',
  fill: '#94a3b8',
}

export function methodStyle(method: string | undefined): MethodStyle {
  if (!method) return FALLBACK_STYLE
  return METHOD_STYLES[method] ?? FALLBACK_STYLE
}

export const HEALTH_TEXT: Record<Health, string> = {
  pass: 'text-emerald-400',
  warn: 'text-amber-400',
  breach: 'text-rose-400',
}

export const HEALTH_BG: Record<Health, string> = {
  pass: 'bg-emerald-950/40 border-emerald-800',
  warn: 'bg-amber-950/40 border-amber-800',
  breach: 'bg-rose-950/40 border-rose-800',
}

export const HEALTH_BAR: Record<Health, string> = {
  pass: 'bg-emerald-500',
  warn: 'bg-amber-500',
  breach: 'bg-rose-500',
}

export const HEALTH_HEX: Record<Health, string> = {
  pass: '#34d399',
  warn: '#fbbf24',
  breach: '#fb7185',
}

/** FPVR target > 80%; warning within 10 points below target. */
export function fpvrHealth(pct: number): Health {
  if (pct > 80) return 'pass'
  if (pct >= 70) return 'warn'
  return 'breach'
}

/** MTTR target < 15 min; warning 15–20 min; breach beyond 20 min. */
export function mttrHealth(seconds: number): Health {
  const min = seconds / 60
  if (min < 15) return 'pass'
  if (min <= 20) return 'warn'
  return 'breach'
}

/** Flakiness index 0..1: <0.1 pass, <0.25 warn, else breach. */
export function flakinessHealth(idx: number): Health {
  if (idx < 0.1) return 'pass'
  if (idx < 0.25) return 'warn'
  return 'breach'
}

export function fmtInt(n: number | undefined | null): string {
  return Math.round(n ?? 0).toLocaleString()
}

export function fmtCompact(n: number | undefined | null): string {
  const v = n ?? 0
  if (Math.abs(v) >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`
  if (Math.abs(v) >= 1_000) return `${(v / 1_000).toFixed(1)}k`
  return `${Math.round(v)}`
}

export function fmtUsd(n: number | undefined | null): string {
  const v = n ?? 0
  if (v !== 0 && Math.abs(v) < 0.01) return `$${v.toFixed(4)}`
  return `$${v.toFixed(2)}`
}

export function fmtPct(n: number | undefined | null): string {
  return `${(n ?? 0).toFixed(1)}%`
}

export function fmtDuration(seconds: number | undefined | null): string {
  const s = Math.max(0, seconds ?? 0)
  if (s < 60) return `${s.toFixed(1)}s`
  if (s < 3600) return `${(s / 60).toFixed(1)}m`
  return `${(s / 3600).toFixed(1)}h`
}
