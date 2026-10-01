<script setup lang="ts">
import { wsService } from '../../services/websocket'

const { isConnected, isReconnecting, reconnectCount } = wsService
</script>

<template>
  <div
    role="status"
    class="flex items-center gap-2 px-2.5 py-1 bg-slate-950/80 border rounded-full text-xs font-mono"
    :class="isConnected ? 'border-slate-800' : 'border-amber-800/80'"
    :title="isConnected ? 'Live updates connected' : `Lost connection to the daemon. Reconnecting (attempt ${reconnectCount}); live updates resume once it is back.`"
  >
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
      {{ isConnected ? 'Daemon Online' : isReconnecting ? `Reconnecting… (${reconnectCount})` : 'Daemon Offline' }}
    </span>
  </div>
</template>
