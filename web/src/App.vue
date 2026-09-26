<script setup lang="ts">
import { onMounted } from 'vue'
import NavigationRail from './components/layout/NavigationRail.vue'
import GlobalHeader from './components/layout/GlobalHeader.vue'
import GlobalToast from './components/common/GlobalToast.vue'
import WsReconnectToast from './components/common/WsReconnectToast.vue'
import { useLayoutStore } from './stores/layout'
import { wsService } from './services/websocket'

const layoutStore = useLayoutStore()

onMounted(() => {
  wsService.connect()
})
</script>

<template>
  <div class="h-screen w-screen flex flex-col bg-slate-950 text-slate-100 overflow-hidden">
    <NavigationRail />
    <GlobalHeader />
    <GlobalToast />
    <WsReconnectToast />

    <div
      class="pt-12 h-full w-full overflow-hidden flex flex-col transition-all duration-200"
      :class="layoutStore.isNavExpanded ? 'pl-60' : 'pl-16'"
    >
      <router-view />
    </div>
  </div>
</template>
