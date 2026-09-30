import { Marked, type Tokens } from 'marked'

/**
 * Markdown renderer for untrusted text (JIRA descriptions, generated documents, agent output).
 * Raw HTML is escaped and only http(s), mailto, anchor and relative links survive, so rendered
 * content cannot run script. Headings get stable ids for the table of contents; code blocks and
 * tables get wrappers that MarkdownView styles and enhances (copy button, horizontal scroll).
 */

export interface TocEntry {
  id: string
  text: string
  depth: number
}

export const escapeHtml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')

const SAFE_URL = /^(https?:|mailto:|#|\/(?!\/))/i
// Remote images, plus task artifacts served by the orchestrator itself (UAT screenshots).
const SAFE_IMAGE = /^(https?:|\/api\/v1\/artifacts\/)/i

// Per-render state (renders are synchronous, so module scope is safe).
let toc: TocEntry[] = []
let slugs = new Map<string, number>()

function slugify(text: string): string {
  const base = text.toLowerCase().replace(/<[^>]+>/g, '').replace(/&[a-z#0-9]+;/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'section'
  const n = slugs.get(base) || 0
  slugs.set(base, n + 1)
  return n ? `${base}-${n}` : base
}

const marked = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html({ text }) {
      return escapeHtml(text)
    },
    heading({ tokens, depth, text }) {
      const inner = this.parser.parseInline(tokens)
      const id = slugify(text)
      toc.push({ id, text: inner.replace(/<[^>]+>/g, ''), depth })
      return `<h${depth} id="md-${id}"><a class="md-anchor" href="#md-${id}" aria-hidden="true" tabindex="-1">#</a>${inner}</h${depth}>\n`
    },
    link({ href, title, tokens }) {
      const inner = this.parser.parseInline(tokens)
      if (!SAFE_URL.test((href || '').trim())) return inner
      const external = /^https?:/i.test(href)
      const t = title ? ` title="${escapeHtml(title)}"` : ''
      return `<a href="${escapeHtml(href)}"${t}${external ? ' target="_blank" rel="noopener noreferrer"' : ''}>${inner}</a>`
    },
    image({ href, text, title }) {
      if (!SAFE_IMAGE.test((href || '').trim())) return escapeHtml(text || '')
      const t = title ? ` title="${escapeHtml(title)}"` : ''
      return `<img src="${escapeHtml(href)}" alt="${escapeHtml(text || '')}"${t} loading="lazy">`
    },
    code({ text, lang }: Tokens.Code) {
      const language = (lang || '').split(/\s/)[0]
      const label = language ? `<span class="md-code-lang">${escapeHtml(language)}</span>` : ''
      return `<div class="md-code">${label}<button type="button" class="md-copy" aria-label="Copy code">Copy</button><pre><code>${escapeHtml(text)}</code></pre></div>\n`
    },
    table(token: Tokens.Table) {
      const cell = (c: Tokens.TableCell, tag: 'th' | 'td') =>
        `<${tag}${c.align ? ` style="text-align:${c.align}"` : ''}>${this.parser.parseInline(c.tokens)}</${tag}>`
      const head = `<tr>${token.header.map((c) => cell(c, 'th')).join('')}</tr>`
      const body = token.rows.map((r) => `<tr>${r.map((c) => cell(c, 'td')).join('')}</tr>`).join('')
      return `<div class="md-table"><table><thead>${head}</thead><tbody>${body}</tbody></table></div>\n`
    },
  },
})

/** Generated documents start with an HTML comment (provenance); it is metadata, not content. */
function stripLeadingComments(src: string): string {
  return src.replace(/^\s*(<!--[\s\S]*?-->\s*)+/, '')
}

export function renderMarkdownDoc(src: string | undefined | null): { html: string; toc: TocEntry[] } {
  toc = []
  slugs = new Map()
  if (!src) return { html: '', toc: [] }
  try {
    const html = marked.parse(stripLeadingComments(src), { async: false }) as string
    return { html, toc: [...toc] }
  } catch {
    return { html: `<pre>${escapeHtml(src)}</pre>`, toc: [] }
  }
}

export function renderMarkdown(src: string | undefined | null): string {
  return renderMarkdownDoc(src).html
}
