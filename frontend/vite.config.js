import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/healthz': 'http://localhost:8080',
      '/users': 'http://localhost:8080',
      '/events': 'http://localhost:8080',
    }
  }
})
