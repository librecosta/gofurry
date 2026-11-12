import type { Locale, Region } from '../preferences'
import { apiQuery, decodeDirectory, decodeHome, normalizeCriteria, type Criteria } from './data'

const unavailable = () => new Response(null, { status: 503, headers: { 'Cache-Control': 'no-store' } })
const queryKeys = ['q', 'category', 'tags', 'language', 'sort', 'locale', 'cursor']

export function navigationRegion(): Region {
  const region = process.env.WEB_REGION ?? 'CN'
  if (region !== 'CN' && region !== 'INTL') throw new Error('Invalid Sites deployment')
  return region
}

export async function navigationAPI(path: 'discovery' | 'directory', query: string, signal?: AbortSignal) {
  const base = process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321'
  const url = new URL('/api/v1/sites/' + path, base)
  url.search = query
  const deadline = AbortSignal.timeout(3000)
  const response = await fetch(url, { signal: signal ? AbortSignal.any([deadline, signal]) : deadline, redirect: 'error' })
  if (response.headers.get('x-gofurry-region') !== navigationRegion() || response.headers.get('x-gofurry-content-mode') !== 'SFW') {
    await response.body?.cancel()
    throw new Error('Sites deployment mismatch')
  }
  return response
}

export async function readHome() {
  const response = await navigationAPI('discovery', '')
  if (!response.ok) { await response.body?.cancel(); throw new Error('Discovery unavailable') }
  return decodeHome(await response.json())
}

export async function readDirectory(criteria: Criteria, locale: Locale) {
  const response = await navigationAPI('directory', apiQuery(criteria, locale))
  if (response.status === 400) { await response.body?.cancel(); return null }
  if (!response.ok) { await response.body?.cancel(); throw new Error('Directory unavailable') }
  return decodeDirectory(await response.json())
}

export async function relayDirectory(request: Request) {
  try {
    const url = new URL(request.url)
    const params = url.searchParams
    if (url.search.length > 2048 || [...params.keys()].some(key => !queryKeys.includes(key) || params.getAll(key).length !== 1) ||
      !/^[A-Za-z0-9_-]{1,200}$/.test(params.get('cursor') ?? '') || !['en', 'zh'].includes(params.get('locale') ?? '')) {
      return new Response(null, { status: 400, headers: { 'Cache-Control': 'no-store' } })
    }
    const criteria = normalizeCriteria(Object.fromEntries(params))
    const response = await navigationAPI('directory', apiQuery(criteria, params.get('locale') as Locale, params.get('cursor')!), request.signal)
    if (response.status === 400) {
      await response.body?.cancel()
      return new Response(null, { status: 400, headers: { 'Cache-Control': 'no-store' } })
    }
    if (!response.ok) { await response.body?.cancel(); return unavailable() }
    return Response.json(decodeDirectory(await response.json()), {
      headers: { 'Cache-Control': 'no-store', 'X-GoFurry-Region': navigationRegion(), 'X-GoFurry-Content-Mode': 'SFW' },
    })
  } catch {
    return unavailable()
  }
}
