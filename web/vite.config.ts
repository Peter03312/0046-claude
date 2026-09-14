import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// During local development / browser tests, /api is proxied to the Go API.
// In production the same path is served by nginx which proxies to the api
// container, so the application code only ever uses relative URLs.
export default defineConfig({
  plugins: [vue()],
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.VITE_API_TARGET || 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
