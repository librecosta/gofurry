import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/api/v1/start/cities/nearest')({
  server: { handlers: { POST: async ({ request }) => {
    const error = (code: number) => Response.json({ error: { code, message: code === 413 ? 'Payload Too Large' : code === 400 ? 'Bad Request' : code === 415 ? 'Unsupported Media Type' : 'Service Unavailable' }, request_id: '' }, { status: code, headers: { 'Cache-Control': 'private, no-store' } })
    if (!/^application\/json(?:;|$)/i.test(request.headers.get('content-type') ?? '')) return error(415)
    try {
      const reader = request.body?.getReader()
      if (!reader) return error(400)
      const chunks: Uint8Array[] = []
      let length = 0
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        length += value.byteLength
        if (length > 256) { await reader.cancel(); return error(413) }
        chunks.push(value)
      }
      const body = new Uint8Array(length)
      let offset = 0
      for (const chunk of chunks) { body.set(chunk, offset); offset += chunk.length }
      const url = new URL('/api/v1/start/cities/nearest', process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321')
      const response = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body,
        credentials: 'omit', redirect: 'error', signal: AbortSignal.any([request.signal, AbortSignal.timeout(3000)]) })
      if (response.headers.get('x-gofurry-region') !== (process.env.WEB_REGION ?? 'CN') || response.headers.get('x-gofurry-content-mode') !== 'SFW') return error(503)
      const headers = new Headers({ 'Cache-Control': 'private, no-store', 'X-GoFurry-Region': response.headers.get('x-gofurry-region')!, 'X-GoFurry-Content-Mode': 'SFW' })
      if (response.headers.has('retry-after')) headers.set('Retry-After', response.headers.get('retry-after')!)
      return Response.json(await response.json(), { status: response.status, headers })
    } catch { return error(503) }
  } } },
})
