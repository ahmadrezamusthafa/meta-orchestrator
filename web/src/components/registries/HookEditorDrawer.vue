<script setup lang="ts">
import { ref } from 'vue'
import { X, Anchor, Check } from 'lucide-vue-next'
import BtnPrimary from '../common/BtnPrimary.vue'

const props = defineProps<{
  isOpen: boolean
  hook?: any
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save', hook: any): void
}>()

const eventName = ref(props.hook?.event || 'pre-stage')
const type = ref(props.hook?.type || 'Shell Script')
const command = ref(props.hook?.command || '')
const policy = ref(props.hook?.policy || 'BLOCK')

function save() {
  emit('save', {
    id: props.hook?.id || `hook_${Date.now()}`,
    event: eventName.value,
    type: type.value,
    command: command.value,
    policy: policy.value,
  })
}
</script>

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-50 overflow-hidden bg-slate-950/70 backdrop-blur-sm"
    @click="$emit('close')"
  >
    <div
      class="absolute inset-y-0 right-0 max-w-md w-full bg-slate-900 border-l border-slate-800 shadow-2xl flex flex-col p-6 space-y-4"
      @click.stop
    >
      <div class="flex items-center justify-between pb-3 border-b border-slate-800">
        <div class="flex items-center gap-2">
          <Anchor class="w-5 h-5 text-purple-400" />
          <div>
            <h3 class="text-sm font-bold text-slate-100">Lifecycle Hook Editor</h3>
            <span class="text-xs text-slate-400">Configure interceptor policy & command</span>
          </div>
        </div>

        <button @click="$emit('close')" class="text-slate-500 hover:text-slate-300">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-3 text-xs">
        <div>
          <label class="block text-slate-400 mb-1 font-medium">Trigger Event</label>
          <select
            v-model="eventName"
            class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none"
          >
            <option value="pre-stage">pre-stage (Before stage execution)</option>
            <option value="post-stage">post-stage (After stage execution)</option>
            <option value="on-gate">on-gate (When gate triggers)</option>
            <option value="on-failure">on-failure (On exception / circuit breaker)</option>
            <option value="pre-commit">pre-commit (Before multi-repo git commit)</option>
            <option value="post-commit">post-commit (After PR creation)</option>
          </select>
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Execution Engine</label>
          <select
            v-model="type"
            class="w-full h-8 px-2 bg-slate-950 border border-slate-700 rounded text-slate-200 focus:outline-none"
          >
            <option value="Shell Script">Local Shell Script (.sh / bash)</option>
            <option value="Docker Container">Ephemeral Docker Container Sandbox</option>
            <option value="Webhook POST">External Webhook POST Request</option>
          </select>
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Command / Script Path / URL</label>
          <input
            v-model="command"
            type="text"
            placeholder=".sdlc/hooks/verify-conventions.sh"
            class="w-full h-8 px-2.5 bg-slate-950 border border-slate-700 rounded text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
          />
        </div>

        <div>
          <label class="block text-slate-400 mb-1 font-medium">Failure Policy</label>
          <div class="grid grid-cols-2 gap-2">
            <label
              class="p-2.5 rounded border cursor-pointer select-none text-xs flex items-center gap-2"
              :class="policy === 'BLOCK' ? 'bg-rose-950/60 border-rose-600 text-rose-200' : 'bg-slate-950 border-slate-800 text-slate-400'"
            >
              <input type="radio" value="BLOCK" v-model="policy" />
              <span>BLOCK Pipeline</span>
            </label>

            <label
              class="p-2.5 rounded border cursor-pointer select-none text-xs flex items-center gap-2"
              :class="policy === 'WARN' ? 'bg-amber-950/60 border-amber-600 text-amber-200' : 'bg-slate-950 border-slate-800 text-slate-400'"
            >
              <input type="radio" value="WARN" v-model="policy" />
              <span>WARN & Continue</span>
            </label>
          </div>
        </div>
      </div>

      <div class="pt-3 border-t border-slate-800 flex items-center justify-end gap-3">
        <button
          @click="$emit('close')"
          type="button"
          class="h-9 px-4 rounded text-xs text-slate-400 hover:text-slate-200"
        >
          Cancel
        </button>

        <BtnPrimary :disabled="!command.trim()" @click="save">
          <Check class="w-3.5 h-3.5" />
          <span>Save Lifecycle Hook</span>
        </BtnPrimary>
      </div>
    </div>
  </div>
</template>
