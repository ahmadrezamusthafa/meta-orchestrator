<script setup lang="ts">
import { wsService } from '../../services/websocket'

const { isConnected, isReconnecting } = wsService
</script>

<template>
  <div class="flex items-center gap-2 px-2.5 py-1 bg-slate-950/80 border border-slate-800 rounded-full text-xs font-mono">
    <span class="relative flex h-2 w-2">
      <span
        v-if="isConnected"
        class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
      ></span>
      <span
        class="relative inline-flex rounded-full h-2 w-2"
        :class="{
          'bg-emerald-500': isConnected,
          'bg-amber-500': isReconnecting && !isConnected,
          'bg-rose-500': !isConnected && !isReconnecting,
        }"
      ></span>
    </span>
    <span class="text-slate-300">
      {{ isConnected ? 'Daemon Online' : isReconnecting ? 'Reconnecting...' : 'Daemon Offline' }}
    </span>
  </div>
</template>
