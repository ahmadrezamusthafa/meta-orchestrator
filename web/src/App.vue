<script setup lang="ts">
import { onMounted } from 'vue'
import NavigationRail from './components/layout/NavigationRail.vue'
import GlobalHeader from './components/layout/GlobalHeader.vue'
import GlobalToast from './components/common/GlobalToast.vue'
import ToolchainPromptModal from './components/toolchain/ToolchainPromptModal.vue'
import { useToolchainStore } from './stores/toolchain'
import { useLayoutStore } from './stores/layout'
import { wsService } from './services/websocket'

const layoutStore = useLayoutStore()
const toolchainStore = useToolchainStore()

onMounted(() => {
  wsService.connect()
  // A stage the operator started (or approved into) waits for tools: ask to install them.
  wsService.subscribe((event) => {
    if (event.type === 'toolchain.needed' && event.task_id) {
      void toolchainStore.openPrompt(event.task_id, { stageId: event.stage_id })
    }
  })
})
</script>

<template>
  <div class="h-screen w-screen flex flex-col bg-slate-950 text-slate-100 overflow-hidden">
    <NavigationRail />
    <GlobalHeader />
    <GlobalToast />
    <ToolchainPromptModal />

    <div
      class="pt-12 h-full w-full overflow-hidden flex flex-col transition-all duration-200"
      :class="layoutStore.isNavExpanded ? 'pl-60' : 'pl-16'"
    >
      <router-view />
    </div>
  </div>
</template>
