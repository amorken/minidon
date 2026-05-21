import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  // Output to web/dist so the Go binary can embed it via go:embed
  build: {
    outDir: 'dist',
  },
  server: {
    port: 5173,
    // Proxy API and SSE requests to the Go backend during development
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
