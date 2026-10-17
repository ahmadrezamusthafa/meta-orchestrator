// safeHref returns url when it is an http(s) link, else undefined. Links built from Jira,
// Confluence or pull-request data must never run script (javascript:, data:, vbscript: …).
export function safeHref(url: string | null | undefined): string | undefined {
  if (!url) return undefined
  try {
    const u = new URL(url, window.location.origin)
    return u.protocol === 'http:' || u.protocol === 'https:' ? url : undefined
  } catch {
    return undefined
  }
}
