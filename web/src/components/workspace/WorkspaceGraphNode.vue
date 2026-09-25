<script setup lang="ts">
defineProps<{
  x: number
  y: number
  name: string
  branch: string
  isLinked: boolean
  isSelected: boolean
}>()

defineEmits<{
  (e: 'select'): void
}>()
</script>

<template>
  <g
    :transform="`translate(${x}, ${y})`"
    @click="$emit('select')"
    class="cursor-pointer group"
  >
    <!-- Node Box -->
    <rect
      width="180"
      height="64"
      rx="8"
      class="transition-all duration-150"
      :class="{
        'fill-slate-900 stroke-emerald-500 stroke-2': isSelected,
        'fill-slate-900/90 stroke-slate-700 group-hover:stroke-slate-500': !isSelected,
      }"
    />

    <!-- Status Indicator Dot -->
    <circle
      cx="16"
      cy="20"
      r="4"
      :class="isLinked ? 'fill-emerald-400' : 'fill-rose-400'"
    />

    <!-- Repo Name -->
    <text
      x="28"
      y="24"
      fill="#f1f5f9"
      font-size="12"
      font-family="sans-serif"
      font-weight="600"
    >
      {{ name }}
    </text>

    <!-- Branch Name -->
    <text
      x="16"
      y="46"
      fill="#94a3b8"
      font-size="10"
      font-family="monospace"
    >
      {{ branch }}
    </text>

    <!-- Symlink Badge -->
    <rect
      x="138"
      y="12"
      width="32"
      height="16"
      rx="4"
      class="fill-slate-950 stroke-slate-800"
    />
    <text
      x="144"
      y="24"
      fill="#10b981"
      font-size="9"
      font-family="monospace"
      font-weight="bold"
    >
      LNK
    </text>
  </g>
</template>
