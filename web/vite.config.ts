import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    // IPv4 loopback on a fixed port: another dev server (e.g. a Laravel Sail container) on the
    // same port would otherwise answer some requests, and strictPort fails loudly instead.
    host: '127.0.0.1',
    port: 5180,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:5181',
        changeOrigin: true
      },
      '/ws': {
        target: 'ws://127.0.0.1:5181',
        ws: true
      }
    }
  }
})
