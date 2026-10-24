import { defineConfig } from 'vite'
import { tanstackStart } from '@tanstack/react-start/plugin/vite'
import react from '@vitejs/plugin-react'
import stylex from '@stylexjs/unplugin'
import { nitro } from 'nitro/vite'

export default defineConfig(({ command }) => ({
  plugins: [
    tanstackStart(),
    stylex.vite({
      dev: command === 'serve',
      runtimeInjection: false,
      devMode: 'css-only',
      useCSSLayers: true,
      unstable_moduleResolution: { type: 'commonJS', rootDir: import.meta.dirname },
    }),
    nitro({ preset: 'node-server', plugins: ['./server/plugins/log.ts'] }),
    react(),
  ],
}))
