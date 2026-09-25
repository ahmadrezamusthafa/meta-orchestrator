<script setup lang="ts">
import { Inbox } from 'lucide-vue-next'
</script>

<template>
  <div class="h-44 border border-dashed border-slate-800 rounded-lg flex flex-col items-center justify-center p-4 text-center">
    <Inbox class="w-6 h-6 text-slate-600 mb-2" />
    <span class="text-xs font-medium text-slate-400 mb-0.5">No active tasks</span>
    <span class="text-[11px] text-slate-600 max-w-[200px] leading-tight">
      Items routed to this phase will appear automatically
    </span>
  </div>
</template>
