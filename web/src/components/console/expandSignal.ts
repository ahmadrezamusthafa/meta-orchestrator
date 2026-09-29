import { ref, watch } from 'vue'

/** Global expand/collapse broadcast (/expand, /collapse). token 0 = never set. */
export interface ExpandSignal {
  token: number
  value: boolean
}

export function useExpandable(signal: () => ExpandSignal | undefined, initial = false) {
  const s = signal()
  const expanded = ref(s && s.token > 0 ? s.value : initial)
  watch(
    () => signal()?.token,
    () => {
      const cur = signal()
      if (cur && cur.token > 0) expanded.value = cur.value
    },
  )
  return expanded
}
