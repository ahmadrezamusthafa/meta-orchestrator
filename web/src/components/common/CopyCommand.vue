<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { Copy, Check } from 'lucide-vue-next'

const props = defineProps<{ command: string }>()
const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

async function copy() {
  try {
    await navigator.clipboard?.writeText(props.command)
    copied.value = true
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => (copied.value = false), 1500)
  } catch {
    // clipboard blocked (insecure context): the command is still selectable
  }
}

onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>

<template>
  <div class="flex items-center gap-2">
    <code class="flex-1 min-w-0 px-2.5 py-1.5 rounded bg-slate-950 border border-slate-800 font-mono text-[11px] text-slate-200 break-all select-all">{{ command }}</code>
    <button type="button" class="h-7 px-2 rounded bg-slate-800 hover:bg-slate-700 text-[11px] text-slate-300 flex items-center gap-1 shrink-0" :title="copied ? 'Copied' : 'Copy command'" @click="copy">
      <Check v-if="copied" class="w-3.5 h-3.5 text-emerald-400" />
      <Copy v-else class="w-3.5 h-3.5" />
      {{ copied ? 'Copied' : 'Copy' }}
    </button>
  </div>
</template>
