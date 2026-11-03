import { randomUUID } from 'node:crypto'
import { definePlugin } from 'nitro'

function log(message: string, fields: Record<string, unknown> = {}) {
  const entry = { time: new Date().toISOString(), service: 'gofurry-web', message, ...fields }
  console.log(process.env.NODE_ENV === 'production' ? JSON.stringify(entry) : `${entry.time} ${message} ${JSON.stringify(fields)}`)
}

export default definePlugin((app) => {
  const requests = new WeakMap<Request, { id: string; start: number }>()
  log('starting')
  app.hooks.hook('request', (event) => {
    requests.set(event.req, { id: randomUUID(), start: performance.now() })
  })
  app.hooks.hook('response', (res, event) => {
    const request = requests.get(event.req)
    if (request) res.headers.set('X-Request-ID', request.id)
    const path = new URL(event.req.url).pathname
    const duration = Math.round(performance.now() - (request?.start ?? performance.now()))
    if (process.env.NODE_ENV === 'production' && res.status < 400 && duration < 1000) return
    // Log only the currently known routes, never query strings or raw user paths.
    log('request', {
      request_id: request?.id,
      method: event.req.method,
      route: ['/', '/start', '/status', '/livez', '/readyz'].includes(path) ? path : '(other)',
      status: res.status,
      duration_ms: duration,
    })
  })
  app.hooks.hook('error', (_error, context) => {
    log('request error', { request_id: context.event ? requests.get(context.event.req)?.id : undefined })
  })
  app.hooks.hook('close', () => log('shutdown complete'))
})
