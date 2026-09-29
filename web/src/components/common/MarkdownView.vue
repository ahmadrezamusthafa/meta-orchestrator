<script setup lang="ts">
import { computed, ref } from 'vue'
import { renderMarkdownDoc } from '../../utils/markdown'
import { List } from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  source?: string | null
  /** document: PRDs, requirements, stage outputs. compact: console and reasoning transcripts. */
  variant?: 'document' | 'compact'
  /** Show an "On this page" outline when the document has enough headings. */
  toc?: boolean
}>(), { source: '', variant: 'document', toc: false })

const root = ref<HTMLElement | null>(null)
const tocOpen = ref(false)

const doc = computed(() => renderMarkdownDoc(props.source))
const outline = computed(() => (props.toc ? doc.value.toc.filter((h) => h.depth >= 1 && h.depth <= 3) : []))
const showToc = computed(() => outline.value.length >= 3)
const minDepth = computed(() => Math.min(...outline.value.map((h) => h.depth)))

function jump(id: string) {
  root.value?.querySelector(`#md-${CSS.escape(id)}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  tocOpen.value = false
}

// Delegated handlers: copy buttons on code blocks, and in-document anchors (which must scroll
// the container instead of changing the route).
async function onClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  const copy = target.closest('.md-copy') as HTMLButtonElement | null
  if (copy) {
    const code = copy.parentElement?.querySelector('pre')?.innerText || ''
    try {
      await navigator.clipboard.writeText(code)
      copy.textContent = 'Copied'
      setTimeout(() => (copy.textContent = 'Copy'), 1400)
    } catch {
      copy.textContent = 'Copy failed'
    }
    return
  }
  const link = target.closest('a') as HTMLAnchorElement | null
  const href = link?.getAttribute('href') || ''
  if (href.startsWith('#md-')) {
    e.preventDefault()
    jump(href.slice(4))
  }
}
</script>

<template>
  <div ref="root" class="md-root" :class="{ 'md-with-toc': showToc }">
    <!-- Outline -->
    <nav v-if="showToc" class="md-toc" aria-label="On this page">
      <button type="button" class="md-toc-toggle" :aria-expanded="tocOpen" @click="tocOpen = !tocOpen">
        <List class="w-3.5 h-3.5" /> On this page
      </button>
      <ol :class="{ open: tocOpen }">
        <li v-for="h in outline" :key="h.id" :style="{ paddingLeft: `${(h.depth - minDepth) * 0.75}rem` }">
          <a :href="`#md-${h.id}`" @click.prevent="jump(h.id)">{{ h.text }}</a>
        </li>
      </ol>
    </nav>

    <!-- eslint-disable-next-line vue/no-v-html — sanitized by renderMarkdownDoc -->
    <article class="md" :class="`md-${variant}`" v-html="doc.html" @click="onClick"></article>
  </div>
</template>

<style scoped>
.md-root { min-width: 0; }
.md-with-toc { display: grid; grid-template-columns: minmax(0, 1fr); gap: 1.5rem; }
@media (min-width: 1100px) {
  .md-with-toc { grid-template-columns: minmax(0, 1fr) 13rem; }
  .md-with-toc .md-toc { order: 2; position: sticky; top: 0; align-self: start; max-height: 70vh; overflow-y: auto; }
  .md-toc-toggle { display: none !important; }
  .md-toc ol { display: block !important; }
}
.md-toc { font-size: 0.75rem; }
.md-toc-toggle {
  display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.3rem 0.6rem;
  border: 1px solid #1e293b; border-radius: 0.5rem; color: #cbd5e1; background: #0b1220;
}
.md-toc ol { display: none; margin-top: 0.5rem; border-left: 1px solid #1e293b; padding-left: 0.75rem; }
.md-toc ol.open { display: block; }
.md-toc li { margin: 0.3rem 0; line-height: 1.35; }
.md-toc a { color: #94a3b8; }
.md-toc a:hover { color: #f1f5f9; }

/* ---------- shared ---------- */
.md { color: #cbd5e1; overflow-wrap: anywhere; min-width: 0; }
.md :deep(> :first-child) { margin-top: 0 !important; }
.md :deep(> :last-child) { margin-bottom: 0 !important; }
.md :deep(strong) { color: #f1f5f9; font-weight: 600; }
.md :deep(em) { font-style: italic; }
.md :deep(del) { color: #64748b; }
.md :deep(a) { color: #7dd3fc; text-decoration: underline; text-underline-offset: 2px; text-decoration-color: rgba(125, 211, 252, 0.4); }
.md :deep(a:hover) { text-decoration-color: currentColor; }
.md :deep(h1), .md :deep(h2), .md :deep(h3), .md :deep(h4), .md :deep(h5), .md :deep(h6) {
  position: relative; color: #f8fafc; font-weight: 600; line-height: 1.3; scroll-margin-top: 0.75rem;
}
.md :deep(.md-anchor) {
  position: absolute; left: -1.1em; padding-right: 0.3em; color: #475569; text-decoration: none; opacity: 0; font-weight: 400;
}
.md :deep(h1:hover .md-anchor), .md :deep(h2:hover .md-anchor), .md :deep(h3:hover .md-anchor) { opacity: 1; }
.md :deep(ul), .md :deep(ol) { padding-left: 1.4rem; }
.md :deep(ul) { list-style: disc; }
.md :deep(ul ul) { list-style: circle; }
.md :deep(ul ul ul) { list-style: square; }
.md :deep(ol) { list-style: decimal; }
.md :deep(ol ol) { list-style: lower-alpha; }
.md :deep(li::marker) { color: #64748b; }
.md :deep(li > p) { margin: 0; }
/* GFM task lists */
.md :deep(li:has(> input[type='checkbox'])) { list-style: none; margin-left: -1.3rem; }
.md :deep(li > input[type='checkbox']) { margin-right: 0.45rem; accent-color: #10b981; vertical-align: -0.1em; }
.md :deep(code) {
  font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 0.85em; color: #c4b5fd;
  background: rgba(30, 41, 59, 0.7); padding: 0.1rem 0.35rem; border-radius: 0.3rem;
}
.md :deep(.md-code) { position: relative; border: 1px solid #1e293b; border-radius: 0.5rem; background: #0b1220; }
.md :deep(.md-code pre) { overflow-x: auto; padding: 0.85rem 1rem; }
.md :deep(.md-code-lang) {
  position: absolute; top: 0.35rem; left: 0.75rem; font-size: 0.65rem; text-transform: uppercase; letter-spacing: 0.06em; color: #64748b;
}
.md :deep(.md-code:has(.md-code-lang) pre) { padding-top: 1.6rem; }
.md :deep(.md-copy) {
  position: absolute; top: 0.3rem; right: 0.4rem; font-size: 0.65rem; padding: 0.15rem 0.45rem; border-radius: 0.3rem;
  color: #94a3b8; background: #111827; border: 1px solid #1e293b; opacity: 0; transition: opacity 0.15s;
}
.md :deep(.md-code:hover .md-copy), .md :deep(.md-copy:focus-visible) { opacity: 1; }
.md :deep(pre code) { background: transparent; padding: 0; color: #e2e8f0; font-size: 0.8rem; line-height: 1.6; }
.md :deep(blockquote) {
  border-left: 3px solid #334155; background: rgba(15, 23, 42, 0.6); color: #94a3b8;
  padding: 0.5rem 0.9rem; border-radius: 0 0.4rem 0.4rem 0;
}
.md :deep(blockquote p) { margin: 0.25rem 0; }
.md :deep(hr) { border: 0; border-top: 1px solid #1e293b; }
.md :deep(img) { max-width: 100%; border-radius: 0.4rem; border: 1px solid #1e293b; }
.md :deep(.md-table) { overflow-x: auto; border: 1px solid #1e293b; border-radius: 0.5rem; }
.md :deep(table) { width: 100%; border-collapse: collapse; }
.md :deep(th) { background: #0f172a; color: #e2e8f0; font-weight: 600; text-align: left; }
.md :deep(th), .md :deep(td) { padding: 0.45rem 0.75rem; border-bottom: 1px solid #1e293b; vertical-align: top; }
.md :deep(th + th), .md :deep(td + td) { border-left: 1px solid #1e293b; }
.md :deep(tbody tr:nth-child(even)) { background: rgba(15, 23, 42, 0.45); }
.md :deep(tbody tr:last-child td) { border-bottom: 0; }

/* ---------- document ---------- */
.md-document { font-size: 0.875rem; line-height: 1.7; }
.md-document :deep(p), .md-document :deep(ul), .md-document :deep(ol), .md-document :deep(blockquote),
.md-document :deep(.md-code), .md-document :deep(.md-table) { margin: 0 0 0.9rem; }
.md-document :deep(li) { margin: 0.25rem 0; }
.md-document :deep(h1) { font-size: 1.45rem; margin: 0 0 1rem; padding-bottom: 0.5rem; border-bottom: 1px solid #1e293b; }
.md-document :deep(h2) { font-size: 1.15rem; margin: 1.75rem 0 0.75rem; padding-bottom: 0.35rem; border-bottom: 1px solid #1e293b; }
.md-document :deep(h3) { font-size: 1rem; margin: 1.4rem 0 0.5rem; }
.md-document :deep(h4), .md-document :deep(h5), .md-document :deep(h6) { font-size: 0.9rem; margin: 1.1rem 0 0.4rem; color: #e2e8f0; }
.md-document :deep(hr) { margin: 1.5rem 0; }
.md-document :deep(table) { font-size: 0.8rem; }

/* ---------- compact ---------- */
.md-compact { font-size: 0.8rem; line-height: 1.6; }
.md-compact :deep(p), .md-compact :deep(ul), .md-compact :deep(ol), .md-compact :deep(blockquote),
.md-compact :deep(.md-code), .md-compact :deep(.md-table) { margin: 0 0 0.5rem; }
.md-compact :deep(li) { margin: 0.1rem 0; }
.md-compact :deep(h1) { font-size: 1.05rem; margin: 0.75rem 0 0.35rem; }
.md-compact :deep(h2) { font-size: 0.95rem; margin: 0.75rem 0 0.35rem; }
.md-compact :deep(h3), .md-compact :deep(h4), .md-compact :deep(h5), .md-compact :deep(h6) { font-size: 0.85rem; margin: 0.6rem 0 0.3rem; }
.md-compact :deep(.md-anchor) { display: none; }
.md-compact :deep(hr) { margin: 0.6rem 0; }
.md-compact :deep(table) { font-size: 0.75rem; }
.md-compact :deep(th), .md-compact :deep(td) { padding: 0.25rem 0.5rem; }
</style>
