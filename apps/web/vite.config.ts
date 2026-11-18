import { fileURLToPath } from 'node:url'
import { webConfig } from './src/web-config.server'
import { defineConfig } from 'vite'
import { tanstackStart } from '@tanstack/react-start/plugin/vite'
import react from '@vitejs/plugin-react'
import { nitro } from 'nitro/vite'

export default defineConfig(({ command }) => {
  if (command === 'serve') process.env.GOFURRY_WEB_CONFIG_FILE ??= fileURLToPath(new URL('./config.example.yaml', import.meta.url))
  return {
    server: {
      host: '127.0.0.1', port: 17320, strictPort: true,
    },
    plugins: [
      tanstackStart(),
      nitro({ preset: 'node-server', plugins: ['./server/plugins/log.ts'],
        routeRules: command === 'serve' ? { '/api/**': { proxy: webConfig().api_origin + '/api/**' } } : {},
      }),
      react(),
    ],
  }
})
