import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
const target = process.env.VITE_API_URL || 'http://localhost:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/healthz': target,
      '/users': target,
      '/events': target,
    }
  }
})
