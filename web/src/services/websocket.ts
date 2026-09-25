import { ref } from 'vue'
import type { OrchestratorEvent } from '../types'

export type EventCallback = (event: OrchestratorEvent) => void

class WebSocketService {
  private ws: WebSocket | null = null
  private listeners: Set<EventCallback> = new Set()
  private reconnectInterval = 3000
  private reconnectTimer: any = null
  
  public isConnected = ref(false)
  public isReconnecting = ref(false)
  public reconnectCount = ref(0)

  connect(taskId?: string) {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    const url = taskId 
      ? `${protocol}//${host}/ws?task_id=${taskId}`
      : `${protocol}//${host}/ws`

    try {
      this.ws = new WebSocket(url)

      this.ws.onopen = () => {
        this.isConnected.value = true
        this.isReconnecting.value = false
        this.reconnectCount.value = 0
        if (this.reconnectTimer) {
          clearTimeout(this.reconnectTimer)
          this.reconnectTimer = null
        }
      }

      this.ws.onmessage = (event) => {
        try {
          const lines = event.data.split('\n')
          for (const line of lines) {
            if (!line.trim()) continue
            const parsed = JSON.parse(line) as OrchestratorEvent
            this.emit(parsed)
          }
        } catch (e) {
          // ignore parsing error
        }
      }

      this.ws.onclose = () => {
        this.isConnected.value = false
        this.scheduleReconnect(taskId)
      }

      this.ws.onerror = () => {
        this.isConnected.value = false
        this.ws?.close()
      }
    } catch (err) {
      this.scheduleReconnect(taskId)
    }
  }

  private scheduleReconnect(taskId?: string) {
    if (this.reconnectTimer) return
    this.isReconnecting.value = true
    this.reconnectCount.value++
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      this.connect(taskId)
    }, this.reconnectInterval)
  }

  subscribe(callback: EventCallback) {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  private emit(event: OrchestratorEvent) {
    this.listeners.forEach((callback) => {
      try {
        callback(event)
      } catch (err) {
        console.error('Error in WS subscriber:', err)
      }
    })
  }

  send(data: any) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data))
    }
  }

  disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.isConnected.value = false
    this.isReconnecting.value = false
  }
}

export const wsService = new WebSocketService()
