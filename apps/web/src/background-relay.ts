import { webConfig } from './web-config.server'
// One deployment origin only; public metadata, never visitor headers or image bytes.
export async function relayBackgrounds(request: Request, id?: string) {
  const incoming = new URL(request.url)
  if (id && id !== 'random' && !/^[a-f\d-]{36}$/i.test(id)) return new Response(null, { status: 400 })
  const url = new URL(`/api/v1/start/backgrounds${id ? `/${id}` : ''}`, webConfig().api_origin)
  for (const key of id === 'random' ? ['exclude'] : id ? [] : ['page', 'page_size', 'q']) {
    const value = incoming.searchParams.get(key)
    if (value !== null) url.searchParams.set(key, value)
  }
  const unavailable = () => new Response(null, { status: 503, headers: { 'Cache-Control': 'no-store' } })
  try {
    const response = await fetch(url, { credentials: 'omit', redirect: 'error', signal: AbortSignal.any([request.signal, AbortSignal.timeout(5000)]) })
    if (response.headers.get('x-gofurry-region') !== (webConfig().region) || response.headers.get('x-gofurry-content-mode') !== 'SFW') return unavailable()
    return new Response(await response.text(), { status: response.status, headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', 'X-GoFurry-Region': response.headers.get('x-gofurry-region')!, 'X-GoFurry-Content-Mode': 'SFW' } })
  } catch { return unavailable() }
}
