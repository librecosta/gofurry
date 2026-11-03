import { createFileRoute } from '@tanstack/react-router'

// Same regional API boundary as backgrounds. Only public suggestions cross it;
// no browser credentials, proxy settings or arbitrary upstream URLs are relayed.
export const Route = createFileRoute('/api/v1/start/search/suggestions')({
  server: { handlers: { GET: async ({ request }) => {
    const unavailable = () => Response.json({ error: { code: 503, message: 'Service Unavailable' }, request_id: '' }, { status: 503, headers: { 'Cache-Control': 'no-store' } })
    try {
      const url = new URL('/api/v1/start/search/suggestions', process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321')
      const query = new URL(request.url).searchParams.get('q') ?? ''
      if ([...query].length > 128) return Response.json({ error: { code: 400, message: 'Bad Request' }, request_id: '' }, { status: 400 })
      url.searchParams.set('q', query)
      const response = await fetch(url, { credentials: 'omit', redirect: 'error', signal: AbortSignal.any([request.signal, AbortSignal.timeout(4500)]) })
      if (response.headers.get('x-gofurry-region') !== (process.env.WEB_REGION ?? 'CN') || response.headers.get('x-gofurry-content-mode') !== 'SFW') return unavailable()
      const headers = new Headers({ 'Cache-Control': 'no-store', 'X-GoFurry-Region': response.headers.get('x-gofurry-region')!, 'X-GoFurry-Content-Mode': 'SFW' })
      if (response.headers.has('retry-after')) headers.set('Retry-After', response.headers.get('retry-after')!)
      return Response.json(await response.json(), { status: response.status, headers })
    } catch { return unavailable() }
  } } },
})
