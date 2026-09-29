import { Marked } from 'marked'

/**
 * Markdown renderer for untrusted text (JIRA descriptions, agent output). Raw HTML is escaped and
 * only http(s), mailto, anchor and relative links survive, so rendered content cannot run script.
 */
const escapeHtml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')

const SAFE_URL = /^(https?:|mailto:|#|\/(?!\/))/i

const safe = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html({ text }) {
      return escapeHtml(text)
    },
    link({ href, tokens }) {
      if (!SAFE_URL.test((href || '').trim())) return this.parser.parseInline(tokens)
      return false // default rendering
    },
    image({ href, text }) {
      if (!/^https?:/i.test((href || '').trim())) return escapeHtml(text || '')
      return false
    },
  },
})

export function renderMarkdown(src: string | undefined | null): string {
  if (!src) return ''
  try {
    const html = safe.parse(src, { async: false }) as string
    // Links open in a new tab without handing the opener to the target page.
    return html.replace(/<a href=/g, '<a target="_blank" rel="noopener noreferrer" href=')
  } catch {
    return escapeHtml(src)
  }
}
