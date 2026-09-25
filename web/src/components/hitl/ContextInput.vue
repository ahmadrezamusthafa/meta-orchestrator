<script setup lang="ts">
import { ref } from 'vue'
import { Send, RotateCcw } from 'lucide-vue-next'
import BtnPrimary from '../common/BtnPrimary.vue'

defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'send', instruction: string): void
  (e: 'reset'): void
}>()

const text = ref('')
const isSending = ref(false)

function submit() {
  if (!text.value.trim() || isSending.value) return
  isSending.value = true
  emit('send', text.value.trim())
  text.value = ''
  setTimeout(() => {
    isSending.value = false
  }, 400)
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    submit()
  }
}
</script>

<template>
  <div class="h-20 p-2.5 bg-slate-900 border-t border-slate-800 flex items-center gap-3">
    <div class="flex-1 relative">
      <textarea
        v-model="text"
        :disabled="disabled"
        @keydown="handleKeydown"
        placeholder="Type corrective guidance to steer active agent (Cmd+Enter to send)..."
        class="w-full h-14 p-2 bg-slate-950 border border-slate-700 rounded text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500 resize-none font-sans leading-relaxed disabled:opacity-50"
      ></textarea>
    </div>

    <div class="flex items-center gap-2">
      <button
        @click="$emit('reset')"
        title="Reset workspace volumes"
        type="button"
        class="h-10 w-10 flex items-center justify-center rounded bg-slate-950 border border-slate-700 text-slate-400 hover:text-rose-400 hover:border-rose-800 transition-colors"
      >
        <RotateCcw class="w-4 h-4" />
      </button>

      <BtnPrimary
        :disabled="disabled || !text.trim()"
        :loading="isSending"
        @click="submit"
        class="!h-10 px-4"
      >
        <Send class="w-3.5 h-3.5" />
        <span>Steer Agent</span>
      </BtnPrimary>
    </div>
  </div>
</template>
