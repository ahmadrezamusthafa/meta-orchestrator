import { Marked } from 'marked'
import type { Token } from 'marked'
import type { ConsoleEntry } from '../../types'

// ---------------------------------------------------------------------------
// Markdown: untrusted model output. Raw HTML is escaped (never rendered) and
// link/image targets are restricted to safe schemes.
// ---------------------------------------------------------------------------

export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

const SAFE_LINK = /^(https?:|mailto:|#|\/(?!\/))/i
const SAFE_IMAGE = /^https:/i

const md = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html({ text }) {
      return escapeHtml(text)
    },
  },
  walkTokens(token: Token) {
    if (token.type === 'link') {
      if (!SAFE_LINK.test(token.href || '')) token.href = '#'
    } else if (token.type === 'image') {
      if (!SAFE_IMAGE.test(token.href || '')) token.href = ''
    }
  },
})

export function renderMarkdown(src: string | undefined): string {
  if (!src) return ''
  try {
    return md.parse(src, { async: false }) as string
  } catch {
    return `<pre>${escapeHtml(src)}</pre>`
  }
}

// ---------------------------------------------------------------------------
// Number / unit formatting
// ---------------------------------------------------------------------------

export function formatTokens(n: number | undefined): string {
  const v = n || 0
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(v >= 10_000_000 ? 0 : 1)}M`
  if (v >= 1_000) return `${(v / 1_000).toFixed(v >= 10_000 ? 0 : 1)}k`
  return String(v)
}

export function formatCost(usd: number | undefined): string {
  const v = usd || 0
  if (v === 0) return '$0.00'
  if (v < 0.01) return `$${v.toFixed(4)}`
  if (v < 1) return `$${v.toFixed(3)}`
  return `$${v.toFixed(2)}`
}

export function formatDuration(ms: number | undefined): string {
  const v = ms || 0
  if (v < 1000) return `${v}ms`
  if (v < 60_000) return `${(v / 1000).toFixed(1)}s`
  const m = Math.floor(v / 60_000)
  const s = Math.round((v % 60_000) / 1000)
  return `${m}m ${s}s`
}

export function shortId(id: string | undefined, len = 8): string {
  if (!id) return '—'
  return id.length > len ? id.slice(0, len) : id
}

export function formatTime(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

// ---------------------------------------------------------------------------
// Tool call summaries: ToolName(short arg summary)
// ---------------------------------------------------------------------------

const TOOL_ARG_KEYS: Record<string, string[]> = {
  read: ['file_path', 'path'],
  write: ['file_path', 'path'],
  edit: ['file_path', 'path'],
  multiedit: ['file_path', 'path'],
  notebookedit: ['notebook_path', 'file_path'],
  bash: ['command'],
  grep: ['pattern'],
  glob: ['pattern'],
  webfetch: ['url'],
  websearch: ['query'],
  task: ['description', 'prompt'],
  agent: ['description', 'prompt'],
  todowrite: [],
}

const GENERIC_KEYS = ['file_path', 'path', 'command', 'pattern', 'url', 'query', 'description', 'name', 'id']

function oneLine(s: string, max = 80): string {
  const flat = s.replace(/\s+/g, ' ').trim()
  return flat.length > max ? `${flat.slice(0, max - 1)}…` : flat
}

export function toolArgSummary(name: string | undefined, input: Record<string, any> | undefined): string {
  if (!input || typeof input !== 'object') return ''
  const key = (name || '').toLowerCase()
  if (key === 'todowrite' && Array.isArray(input.todos)) return `${input.todos.length} todos`
  const keys = [...(TOOL_ARG_KEYS[key] || []), ...GENERIC_KEYS]
  for (const k of keys) {
    const v = input[k]
    if (typeof v === 'string' && v) {
      let out = v
      if ((key === 'grep' || key === 'glob') && typeof input.path === 'string' && input.path) {
        out = `${v}, ${input.path}`
      }
      return oneLine(out)
    }
  }
  for (const v of Object.values(input)) {
    if (typeof v === 'string' && v) return oneLine(v)
  }
  const n = Object.keys(input).length
  return n ? `${n} arg${n === 1 ? '' : 's'}` : ''
}

export function splitLines(text: string | undefined): string[] {
  if (!text) return []
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  while (lines.length && lines[lines.length - 1] === '') lines.pop()
  return lines
}

export function prettyJson(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

// ---------------------------------------------------------------------------
// Transcript export (markdown)
// ---------------------------------------------------------------------------

function fence(body: string, lang = ''): string {
  const ticks = body.includes('```') ? '````' : '```'
  return `${ticks}${lang}\n${body}\n${ticks}`
}

export function responseSummary(e: ConsoleEntry): string {
  const u = e.usage
  if (!u) return '←'
  const parts = [
    `${formatTokens(u.prompt_tokens)} in`,
    `${formatTokens(u.completion_tokens)} out`,
  ]
  if (u.cached_tokens) parts.push(`${formatTokens(u.cached_tokens)} cached`)
  parts.push(formatCost(u.cost_usd), formatDuration(u.duration_ms))
  if (u.finish_reason) parts.push(u.finish_reason)
  return `← ${parts.join(' · ')}`
}

export function requestSummary(e: ConsoleEntry): string {
  const r = e.request
  if (!r) return '→'
  const n = r.messages?.length || 0
  const parts = [r.model || 'auto', r.method || '—', `${n} message${n === 1 ? '' : 's'}`]
  if (r.strategy) parts.push(r.strategy)
  return `→ ${parts.join(' · ')}`
}

export function stateSummary(e: ConsoleEntry): string {
  const s = e.state
  if (!s) return e.content || ''
  return `● ${s.from || '—'} → ${s.to || '—'}${s.stage ? ` (${s.stage})` : ''}`
}

export function transcriptToMarkdown(entries: ConsoleEntry[], title: string): string {
  const out: string[] = [`# ${title}`, '', `_Exported ${new Date().toISOString()}_`, '']
  for (const e of entries) {
    switch (e.kind) {
      case 'user':
        out.push(splitLines(e.content).map(l => `> ${l}`).join('\n') || '>')
        break
      case 'assistant':
        out.push(`⏺ ${e.content || ''}${e.status === 'cancelled' ? '\n\n_(interrupted)_' : ''}`)
        break
      case 'thinking':
        out.push(`<details><summary>✻ Thinking</summary>\n\n${e.content || ''}\n\n</details>`)
        break
      case 'tool_use':
        out.push(`⏺ **${e.tool?.name || 'Tool'}**(${toolArgSummary(e.tool?.name, e.tool?.input)})`)
        if (e.tool?.input) out.push(fence(prettyJson(e.tool.input), 'json'))
        break
      case 'tool_result':
        out.push(`⎿ ${e.tool?.is_error ? 'error' : 'result'}${e.tool?.id ? ` (${e.tool.id})` : ''}`)
        out.push(fence(e.content || ''))
        break
      case 'request':
        out.push(`\`${requestSummary(e)}\``)
        if (e.request?.fallback_chain?.length) out.push(`fallback: ${e.request.fallback_chain.join(' → ')}`)
        break
      case 'response':
        out.push(`\`${responseSummary(e)}\``)
        break
      case 'state':
        out.push(`\`${stateSummary(e)}\``)
        break
      case 'error':
        out.push(`**Error:** ${e.content || ''}`)
        break
      default:
        out.push(`_${e.content || ''}_`)
    }
    out.push('')
  }
  return out.join('\n')
}

export function downloadText(filename: string, text: string, mime = 'text/markdown;charset=utf-8') {
  const blob = new Blob([text], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
