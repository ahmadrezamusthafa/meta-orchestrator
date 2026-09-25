<script setup lang="ts">
import { wsService } from '../../services/websocket'

const { isConnected, isReconnecting, reconnectCount } = wsService
</script>

<template>
  <transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="transform -translate-y-4 opacity-0"
    enter-to-class="transform translate-y-0 opacity-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="transform translate-y-0 opacity-100"
    leave-to-class="transform -translate-y-4 opacity-0"
  >
    <div
      v-if="!isConnected && isReconnecting"
      class="fixed top-14 left-1/2 -translate-x-1/2 z-50 px-4 py-2 bg-amber-950/90 border border-amber-800/90 text-amber-200 text-xs rounded-lg shadow-xl flex items-center gap-3 backdrop-blur"
    >
      <span class="relative flex h-2.5 w-2.5">
        <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
        <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-amber-500"></span>
      </span>
      <span>
        Daemon WebSocket disconnected. Reconnecting in 3s (attempt #{{ reconnectCount }})... 
        <span class="text-amber-400/80 font-mono text-[11px]">(State persisted in Redis)</span>
      </span>
    </div>
  </transition>
</template>
