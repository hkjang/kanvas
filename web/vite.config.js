import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: fileURLToPath(new URL('../webembed/dist', import.meta.url)),
    emptyOutDir: true,
    sourcemap: false,
    chunkSizeWarningLimit: 800,
  },
  server: {
    port: 5173,
    proxy: { '/api': 'http://localhost:8080', '/mcp': 'http://localhost:8080' },
  },
})
