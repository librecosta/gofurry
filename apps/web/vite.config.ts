import { defineConfig } from 'vite'
import { tanstackStart } from '@tanstack/react-start/plugin/vite'
import react from '@vitejs/plugin-react'
import stylex from '@stylexjs/unplugin'
import { nitro } from 'nitro/vite'

export default defineConfig(({ command }) => ({
  server: {
    host: '127.0.0.1', port: 17320, strictPort: true,
  },
  plugins: [
    tanstackStart(),
    stylex.vite({
      dev: command === 'serve',
      // Start's server/client route transforms shift source lines independently.
      // Exclude only the optional debug data attribute from hydration markup.
      enableDebugDataProp: false,
      runtimeInjection: false,
      devMode: 'css-only',
      useCSSLayers: true,
      unstable_moduleResolution: { type: 'commonJS', rootDir: import.meta.dirname },
    }),
    nitro({ preset: 'node-server', plugins: ['./server/plugins/log.ts'],
      routeRules: command === 'serve' ? { '/api/**': { proxy: 'http://127.0.0.1:17321/api/**' } } : {},
    }),
    react(),
  ],
}))
