<script setup lang="ts">
import { onMounted } from 'vue'
import NavigationRail from './components/layout/NavigationRail.vue'
import GlobalHeader from './components/layout/GlobalHeader.vue'
import WsReconnectToast from './components/common/WsReconnectToast.vue'
import { wsService } from './services/websocket'

onMounted(() => {
  wsService.connect()
})
</script>

<template>
  <div class="h-screen w-screen flex flex-col bg-slate-950 text-slate-100 overflow-hidden">
    <NavigationRail />
    <GlobalHeader />
    <WsReconnectToast />

    <div class="pt-12 pl-16 h-full w-full overflow-hidden flex flex-col">
      <router-view />
    </div>
  </div>
</template>
