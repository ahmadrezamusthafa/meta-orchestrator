import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface ToastMessage {
  id: string
  title: string
  description?: string
  type: 'success' | 'info' | 'warning' | 'error'
  duration?: number
}

export const useToastStore = defineStore('toast', () => {
  const toasts = ref<ToastMessage[]>([])

  function show(toast: Omit<ToastMessage, 'id'>) {
    const id = `toast-${Date.now()}-${Math.random().toString(36).substr(2, 5)}`
    const duration = toast.duration ?? 4000
    const newToast: ToastMessage = { ...toast, id, duration }
    toasts.value.push(newToast)

    if (duration > 0) {
      setTimeout(() => {
        dismiss(id)
      }, duration)
    }
    return id
  }

  function success(title: string, description?: string) {
    return show({ title, description, type: 'success' })
  }

  function info(title: string, description?: string) {
    return show({ title, description, type: 'info' })
  }

  function warning(title: string, description?: string) {
    return show({ title, description, type: 'warning' })
  }

  function error(title: string, description?: string) {
    return show({ title, description, type: 'error', duration: 6000 })
  }

  function dismiss(id: string) {
    const idx = toasts.value.findIndex((t) => t.id === id)
    if (idx !== -1) {
      toasts.value.splice(idx, 1)
    }
  }

  return {
    toasts,
    show,
    success,
    info,
    warning,
    error,
    dismiss,
  }
})
