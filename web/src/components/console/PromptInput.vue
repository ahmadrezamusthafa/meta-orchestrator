<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import SlashMenu from './SlashMenu.vue'
import { filterCommands } from './consoleCommands'
import type { SlashCommand } from './consoleCommands'

const props = withDefaults(defineProps<{
  busy?: boolean
  placeholder?: string
  history?: string[]
  /** Accent for special input modes (e.g. gate feedback). */
  mode?: 'default' | 'feedback'
  hint?: string
}>(), {
  busy: false,
  placeholder: 'Try "summarize the failing test" or /help',
  history: () => [],
  mode: 'default',
  hint: '',
})

const emit = defineEmits<{
  (e: 'submit', text: string): void
  (e: 'cancel'): void
  (e: 'escape'): void
  (e: 'shortcuts'): void
}>()

const text = ref('')
const textarea = ref<HTMLTextAreaElement | null>(null)
const menuIndex = ref(0)
const menuDismissed = ref(false)
const historyIndex = ref(-1)
const draft = ref('')
const listId = `slash-menu-${Math.random().toString(36).slice(2, 8)}`

const menuItems = computed(() => (menuDismissed.value ? [] : filterCommands(text.value)))
const menuOpen = computed(() => menuItems.value.length > 0)
const isSlash = computed(() => text.value.trimStart().startsWith('/'))
const sendBlocked = computed(() => props.busy && !isSlash.value && props.mode === 'default')

watch(text, () => {
  menuIndex.value = 0
  if (!text.value.startsWith('/')) menuDismissed.value = false
  nextTick(autoGrow)
})

function autoGrow() {
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 240)}px`
}

function focus() {
  textarea.value?.focus()
}

function setValue(v: string) {
  text.value = v
  nextTick(() => {
    const el = textarea.value
    if (el) {
      el.focus()
      el.setSelectionRange(v.length, v.length)
    }
  })
}

defineExpose({ focus, setValue })

function pick(cmd: SlashCommand, submitIfComplete: boolean) {
  if (submitIfComplete && !cmd.args?.startsWith('<')) {
    text.value = `/${cmd.name}`
    submit()
    return
  }
  setValue(`/${cmd.name} `)
  menuDismissed.value = true
}

function submit() {
  const value = text.value.trim()
  if (!value) return
  if (sendBlocked.value) return
  const hist = props.history
  if (props.mode === 'default' && hist[hist.length - 1] !== value) hist.push(value)
  historyIndex.value = -1
  draft.value = ''
  text.value = ''
  menuDismissed.value = false
  emit('submit', value)
}

function cursorOnFirstLine(el: HTMLTextAreaElement) {
  return !el.value.slice(0, el.selectionStart).includes('\n')
}

function cursorOnLastLine(el: HTMLTextAreaElement) {
  return !el.value.slice(el.selectionEnd).includes('\n')
}

function recall(dir: -1 | 1) {
  const hist = props.history
  if (!hist.length) return false
  if (dir === -1) {
    if (historyIndex.value === -1) {
      draft.value = text.value
      historyIndex.value = hist.length - 1
    } else if (historyIndex.value > 0) {
      historyIndex.value--
    } else {
      return true
    }
  } else {
    if (historyIndex.value === -1) return false
    if (historyIndex.value < hist.length - 1) {
      historyIndex.value++
    } else {
      historyIndex.value = -1
      setValue(draft.value)
      return true
    }
  }
  setValue(hist[historyIndex.value])
  return true
}

function onKeydown(e: KeyboardEvent) {
  const el = textarea.value
  if (!el || e.isComposing) return

  if (menuOpen.value) {
    if (e.key === 'ArrowDown' || (e.key === 'n' && e.ctrlKey)) {
      e.preventDefault()
      menuIndex.value = (menuIndex.value + 1) % menuItems.value.length
      return
    }
    if (e.key === 'ArrowUp' || (e.key === 'p' && e.ctrlKey)) {
      e.preventDefault()
      menuIndex.value = (menuIndex.value - 1 + menuItems.value.length) % menuItems.value.length
      return
    }
    if (e.key === 'Tab') {
      e.preventDefault()
      pick(menuItems.value[menuIndex.value], false)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      pick(menuItems.value[menuIndex.value], true)
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      menuDismissed.value = true
      return
    }
  }

  if (e.key === 'Enter' && !e.shiftKey && !e.altKey) {
    e.preventDefault()
    submit()
    return
  }

  if (e.key === 'Escape') {
    if (props.busy) {
      e.preventDefault()
      e.stopPropagation()
      emit('cancel')
    } else if (text.value || props.mode !== 'default') {
      e.preventDefault()
      e.stopPropagation()
      text.value = ''
      historyIndex.value = -1
      emit('escape')
    }
    // empty & idle: let it bubble (e.g. drawer close)
    return
  }

  if (e.key === '?' && !text.value) {
    e.preventDefault()
    emit('shortcuts')
    return
  }

  if (e.key === 'ArrowUp' && !e.shiftKey && cursorOnFirstLine(el)) {
    if (recall(-1)) e.preventDefault()
    return
  }
  if (e.key === 'ArrowDown' && !e.shiftKey && cursorOnLastLine(el)) {
    if (recall(1)) e.preventDefault()
  }
}
</script>

<template>
  <div class="relative">
    <SlashMenu
      v-if="menuOpen"
      class="absolute bottom-full left-0 right-0"
      :items="menuItems"
      :selected="menuIndex"
      :list-id="listId"
      @pick="c => pick(c, true)"
      @hover="i => (menuIndex = i)"
    />
    <div
      class="flex items-start gap-2 rounded-lg border px-3 py-2 transition-colors focus-within:border-slate-500"
      :class="mode === 'feedback' ? 'border-amber-600/70 bg-amber-950/10' : 'border-slate-700 bg-[#0b0f17]'"
      @click="focus"
    >
      <span
        class="select-none pt-px font-mono text-sm leading-6"
        :class="mode === 'feedback' ? 'text-amber-400' : 'text-slate-400'"
        aria-hidden="true"
      >&gt;</span>
      <textarea
        ref="textarea"
        v-model="text"
        rows="1"
        spellcheck="false"
        autocomplete="off"
        :placeholder="placeholder"
        :aria-label="mode === 'feedback' ? 'Gate rejection feedback' : 'Message the agent or type a slash command'"
        role="combobox"
        :aria-expanded="menuOpen"
        :aria-controls="menuOpen ? listId : undefined"
        :aria-activedescendant="menuOpen ? `${listId}-${menuIndex}` : undefined"
        aria-autocomplete="list"
        class="block max-h-60 min-h-[24px] w-full resize-none overflow-y-auto bg-transparent font-mono text-sm leading-6 text-slate-100 placeholder:text-slate-600 focus:outline-none"
        @keydown="onKeydown"
      ></textarea>
    </div>
    <div class="flex h-5 items-center justify-between px-1 font-mono text-[11px] text-slate-500">
      <span v-if="sendBlocked" class="text-amber-400/80">turn in progress — esc to interrupt</span>
      <span v-else-if="hint">{{ hint }}</span>
      <span v-else-if="isSlash">tab to complete · enter to run</span>
      <span v-else>enter to send · shift+enter for newline</span>
      <slot name="right" />
    </div>
  </div>
</template>
