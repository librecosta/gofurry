import { createFileRoute } from '@tanstack/react-router'

// Production same-origin fallback. No browser IP/coordinates/credentials are
// forwarded: real visitor resolution belongs to the configured trusted edge.
export const Route = createFileRoute('/api/v1/start/weather')({
  server: { handlers: { GET: async ({ request }) => {
    const unavailable = () => Response.json({ error: { code: 503, message: 'Service Unavailable' }, request_id: '' }, { status: 503, headers: { 'Cache-Control': 'private, no-store' } })
    const lang = new URL(request.url).searchParams.get('lang') ?? 'zh'
    if (lang !== 'zh' && lang !== 'en') return Response.json({ error: { code: 400, message: 'Bad Request' }, request_id: '' }, { status: 400 })
    try {
      const url = new URL('/api/v1/start/weather', process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321')
      url.searchParams.set('lang', lang)
      const response = await fetch(url, { credentials: 'omit', redirect: 'error', signal: AbortSignal.any([request.signal, AbortSignal.timeout(5000)]) })
      if (response.headers.get('x-gofurry-region') !== (process.env.WEB_REGION ?? 'CN') || response.headers.get('x-gofurry-content-mode') !== 'SFW') return unavailable()
      const headers = new Headers({ 'Cache-Control': 'private, no-store', 'X-GoFurry-Region': response.headers.get('x-gofurry-region')!, 'X-GoFurry-Content-Mode': 'SFW' })
      if (response.headers.has('retry-after')) headers.set('Retry-After', response.headers.get('retry-after')!)
      return Response.json(await response.json(), { status: response.status, headers })
    } catch { return unavailable() }
  } } },
})
