import { defineConfig } from 'vite'
import { tanstackStart } from '@tanstack/react-start/plugin/vite'
import react from '@vitejs/plugin-react'
import { nitro } from 'nitro/vite'

export default defineConfig(({ command }) => ({
  server: {
    host: '127.0.0.1', port: 17320, strictPort: true,
  },
  plugins: [
    tanstackStart(),
    nitro({ preset: 'node-server', plugins: ['./server/plugins/log.ts'],
      routeRules: command === 'serve' ? { '/api/**': { proxy: 'http://127.0.0.1:17321/api/**' } } : {},
    }),
    react(),
  ],
}))
