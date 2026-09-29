/**
 * Unified-diff parsing and layout for the Changes tab: per-file hunks, side-by-side pairing of
 * removed/added lines, and word-level highlights inside changed lines (like GitHub).
 */

export type LineKind = 'add' | 'del' | 'ctx'

/** A run of text; `hl` marks the part that changed within a modified line. */
export interface Segment {
  text: string
  hl?: boolean
}

export interface DiffLine {
  kind: LineKind
  oldNo?: number
  newNo?: number
  segments: Segment[]
}

export interface Hunk {
  header: string
  lines: DiffLine[]
}

export interface FilePatch {
  path: string
  hunks: Hunk[]
  binary: boolean
  lineCount: number
}

export type UnifiedRow = { type: 'hunk'; header: string } | { type: 'line'; line: DiffLine } | { type: 'note'; text: string }

export type SplitRow =
  | { type: 'hunk'; header: string }
  | { type: 'note'; text: string }
  | { type: 'pair'; left?: DiffLine; right?: DiffLine }

export function parsePatch(patch: string): Record<string, FilePatch> {
  const files: Record<string, FilePatch> = {}
  let file: FilePatch | null = null
  let hunk: Hunk | null = null
  let oldNo = 0
  let newNo = 0
  for (const raw of patch.split('\n')) {
    if (raw.startsWith('diff --git ')) {
      const m = raw.match(/ b\/(.+)$/)
      file = { path: m ? m[1] : raw.slice(11), hunks: [], binary: false, lineCount: 0 }
      files[file.path] = file
      hunk = null
      continue
    }
    if (!file) continue
    if (raw.startsWith('@@')) {
      const m = raw.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$/)
      oldNo = m ? Number(m[1]) : 0
      newNo = m ? Number(m[2]) : 0
      hunk = { header: raw, lines: [] }
      file.hunks.push(hunk)
      continue
    }
    if (!hunk) {
      if (raw.startsWith('Binary files')) file.binary = true // headers before the first hunk
      continue
    }
    if (raw.startsWith('+')) {
      hunk.lines.push({ kind: 'add', newNo: newNo++, segments: [{ text: raw.slice(1) }] })
    } else if (raw.startsWith('-')) {
      hunk.lines.push({ kind: 'del', oldNo: oldNo++, segments: [{ text: raw.slice(1) }] })
    } else if (raw.startsWith(' ')) {
      hunk.lines.push({ kind: 'ctx', oldNo: oldNo++, newNo: newNo++, segments: [{ text: raw.slice(1) }] })
    } else {
      continue // "\ No newline at end of file" and trailing blank lines are not rows
    }
    file.lineCount++
  }
  for (const f of Object.values(files)) for (const h of f.hunks) highlightWords(h.lines)
  return files
}

/** Pair each block of removed lines with the added lines that follow it and diff them by word. */
function highlightWords(lines: DiffLine[]) {
  for (let i = 0; i < lines.length; ) {
    if (lines[i].kind !== 'del') {
      i++
      continue
    }
    let j = i
    while (j < lines.length && lines[j].kind === 'del') j++
    let k = j
    while (k < lines.length && lines[k].kind === 'add') k++
    const dels = lines.slice(i, j)
    const adds = lines.slice(j, k)
    for (let n = 0; n < Math.min(dels.length, adds.length); n++) {
      const [a, b] = wordDiff(dels[n].segments[0].text, adds[n].segments[0].text)
      if (a && b) {
        dels[n].segments = a
        adds[n].segments = b
      }
    }
    i = k
  }
}

const TOKEN = /\s+|[A-Za-z0-9_]+|[^\sA-Za-z0-9_]/g
const MAX_TOKENS = 400

/** Token-level LCS; returns highlighted segments for both sides, or nulls when too dissimilar. */
export function wordDiff(oldText: string, newText: string): [Segment[] | null, Segment[] | null] {
  const a = oldText.match(TOKEN) || []
  const b = newText.match(TOKEN) || []
  if (!a.length || !b.length || a.length > MAX_TOKENS || b.length > MAX_TOKENS) return [null, null]
  const dp: number[][] = Array.from({ length: a.length + 1 }, () => new Array(b.length + 1).fill(0))
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }
  const keepA = new Array(a.length).fill(false)
  const keepB = new Array(b.length).fill(false)
  for (let i = 0, j = 0; i < a.length && j < b.length; ) {
    if (a[i] === b[j]) {
      keepA[i++] = true
      keepB[j++] = true
    } else if (dp[i + 1][j] >= dp[i][j + 1]) i++
    else j++
  }
  // Lines that share almost nothing read better as plain replacements.
  const common = keepA.filter(Boolean).length
  if (common === 0 || common / Math.max(a.length, b.length) < 0.3) return [null, null]
  return [toSegments(a, keepA), toSegments(b, keepB)]
}

function toSegments(tokens: string[], keep: boolean[]): Segment[] {
  // Whitespace sitting between two changed tokens reads as part of one change.
  for (let i = 1; i < tokens.length - 1; i++) {
    if (keep[i] && /^\s+$/.test(tokens[i]) && !keep[i - 1] && !keep[i + 1]) keep[i] = false
  }
  const out: Segment[] = []
  tokens.forEach((t, i) => {
    const hl = !keep[i]
    const last = out[out.length - 1]
    if (last && !!last.hl === hl) last.text += t
    else out.push(hl ? { text: t, hl: true } : { text: t })
  })
  return out
}

export function unifiedRows(f: FilePatch): UnifiedRow[] {
  const rows: UnifiedRow[] = []
  for (const h of f.hunks) {
    rows.push({ type: 'hunk', header: h.header })
    for (const line of h.lines) rows.push({ type: 'line', line })
  }
  return rows
}

/** Side-by-side rows: context on both sides; removed lines face the added lines that replace them. */
export function splitRows(f: FilePatch): SplitRow[] {
  const rows: SplitRow[] = []
  for (const h of f.hunks) {
    rows.push({ type: 'hunk', header: h.header })
    const lines = h.lines
    for (let i = 0; i < lines.length; ) {
      if (lines[i].kind === 'ctx') {
        rows.push({ type: 'pair', left: lines[i], right: lines[i] })
        i++
        continue
      }
      let j = i
      while (j < lines.length && lines[j].kind === 'del') j++
      let k = j
      while (k < lines.length && lines[k].kind === 'add') k++
      const dels = lines.slice(i, j)
      const adds = lines.slice(j, k)
      for (let n = 0; n < Math.max(dels.length, adds.length); n++) rows.push({ type: 'pair', left: dels[n], right: adds[n] })
      i = k > i ? k : i + 1
    }
  }
  return rows
}
