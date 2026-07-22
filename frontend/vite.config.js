import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
const target = process.env.VITE_API_URL || 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/healthz': {
        target,
        changeOrigin: true,
      },
      '/users': {
        target,
        changeOrigin: true,
      },
      '/events': {
        target,
        changeOrigin: true,
      },
    }
  }
})
