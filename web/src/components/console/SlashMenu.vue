<script setup lang="ts">
import type { SlashCommand } from './consoleCommands'

defineProps<{
  items: SlashCommand[]
  selected: number
  listId: string
}>()

const emit = defineEmits<{
  (e: 'pick', cmd: SlashCommand): void
  (e: 'hover', index: number): void
}>()
</script>

<template>
  <ul
    :id="listId"
    role="listbox"
    aria-label="Slash commands"
    class="mb-1 max-h-64 overflow-y-auto rounded-md border border-slate-800 bg-[#0b0f17] py-1 font-mono text-xs shadow-xl"
  >
    <li
      v-for="(cmd, i) in items"
      :id="`${listId}-${i}`"
      :key="cmd.name"
      role="option"
      :aria-selected="i === selected"
      class="flex cursor-pointer items-baseline gap-3 px-3 py-0.5"
      :class="i === selected ? 'bg-slate-800/70 text-violet-300' : 'text-slate-400 hover:bg-slate-900'"
      @mousedown.prevent="emit('pick', cmd)"
      @mouseenter="emit('hover', i)"
    >
      <span class="w-44 flex-shrink-0 truncate">
        /{{ cmd.name }}<span v-if="cmd.args" class="text-slate-600"> {{ cmd.args }}</span>
      </span>
      <span class="truncate" :class="i === selected ? 'text-slate-300' : 'text-slate-500'">{{ cmd.description }}</span>
    </li>
  </ul>
</template>
