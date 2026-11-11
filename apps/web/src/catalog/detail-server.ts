import { createServerFn } from '@tanstack/react-start'
import { getRequestUrl, setResponseStatus } from '@tanstack/react-start/server'
import type { Region } from '../preferences'

export type Kind = 'sites' | 'games'
export type Localized = { locale: 'zh' | 'en'; name: string; summary?: string }
type Identity = { public_id: string; slug: string; localizations: Localized[] }
export type Detail =
  | (Identity & { kind: 'sites'; addresses: { kind: 'primary' | 'alternate'; url: string }[] })
  | (Identity & { kind: 'games'; sources: { platform_code: string; external_key: string; source_url?: string }[] })
type Lookup = { kind: Kind; publicId: string; region: Region }
type Result = { state: 'missing' } | { state: 'unavailable' } | { state: 'published'; detail: Detail; origin: string }

const publicID = /^[0123456789abcdefghjkmnpqrstvwxyz]{16}$/
const slug = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

export function isPublicID(value: string) { return value.length === 16 && publicID.test(value) }

function publicOrigin() {
  const configured = process.env.WEB_PUBLIC_ORIGIN
  const url = configured ? new URL(configured) : getRequestUrl({ xForwardedHost: false, xForwardedProto: false })
  if (url.username || url.password || !['http:', 'https:'].includes(url.protocol)) throw new Error('Invalid public origin')
  if (configured ? (url.pathname !== '/' || url.search || url.hash) : !['127.0.0.1', '[::1]', 'localhost'].includes(url.hostname)) {
    throw new Error('An explicit WEB_PUBLIC_ORIGIN is required')
  }
  return url.origin
}

function webAddress(value: unknown): value is string {
  if (typeof value !== 'string' || value.length > 2048) return false
  const parsed = URL.parse(value)
  return Boolean(parsed && (parsed.protocol === 'https:' || parsed.protocol === 'http:') && !parsed.username && !parsed.password && !parsed.hash)
}

function decode(input: unknown, kind: Kind, id: string): Detail {
  if (!input || typeof input !== 'object') throw new Error('Invalid API payload')
  const item = input as Record<string, unknown>
  if (item.public_id !== id || typeof item.slug !== 'string' || item.slug.length > 80 || slug.exec(item.slug)?.[0] !== item.slug ||
    !Array.isArray(item.localizations) || item.localizations.length === 0 || item.localizations.length > 2) throw new Error('Invalid API identity')
  const localizations: Localized[] = item.localizations.map(value => {
    if (!value || (value.locale !== 'zh' && value.locale !== 'en') || typeof value.name !== 'string' || !value.name.trim() ||
      (value.summary !== undefined && typeof value.summary !== 'string')) throw new Error('Invalid API localization')
    return { locale: value.locale, name: value.name, summary: value.summary }
  })
  if (new Set(localizations.map(value => value.locale)).size !== localizations.length) throw new Error('Duplicate API localization')
  const identity = { public_id: id, slug: item.slug, localizations }
  if (kind === 'sites') {
    if (!Array.isArray(item.addresses) || item.addresses.length > 16) throw new Error('Invalid API addresses')
    const addresses = item.addresses.map(value => {
      if (!value || (value.kind !== 'primary' && value.kind !== 'alternate') || !webAddress(value.url)) throw new Error('Invalid API address')
      return { kind: value.kind as 'primary' | 'alternate', url: value.url }
    })
    if (addresses.filter(value => value.kind === 'primary').length !== 1) throw new Error('Missing primary address')
    return { ...identity, kind, addresses }
  }
  if (!Array.isArray(item.sources) || item.sources.length > 16) throw new Error('Invalid API sources')
  return { ...identity, kind, sources: item.sources.map(value => {
    if (!value || typeof value.platform_code !== 'string' || typeof value.external_key !== 'string' ||
      (value.source_url !== undefined && !webAddress(value.source_url))) throw new Error('Invalid API source')
    return { platform_code: value.platform_code, external_key: value.external_key, source_url: value.source_url }
  }) }
}

export const readCatalogDetail = createServerFn({ method: 'GET' }).validator((input: Lookup) => {
  if (!input || !['sites', 'games'].includes(input.kind) || typeof input.publicId !== 'string' || !isPublicID(input.publicId) ||
    (input.region !== 'CN' && input.region !== 'INTL')) throw new Error('Invalid catalog request')
  return input
}).handler(async ({ data: { kind, publicId, region } }): Promise<Result> => {
  try {
    const deployment = process.env.WEB_REGION ?? 'CN'
    if (deployment !== region) throw new Error('Region mismatch')
    const url = new URL(`/api/v1/${kind}/${publicId}`, process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321')
    const response = await fetch(url, { signal: AbortSignal.timeout(3000), redirect: 'error' })
    if (response.headers.get('x-gofurry-region') !== deployment || response.headers.get('x-gofurry-content-mode') !== 'SFW') {
      await response.body?.cancel()
      throw new Error('API deployment mismatch')
    }
    if (response.status !== 200) {
      await response.body?.cancel()
      if (response.status === 404) return { state: 'missing' }
      throw new Error('Catalog API unavailable')
    }
    return { state: 'published', detail: decode(await response.json(), kind, publicId), origin: publicOrigin() }
  } catch {
    setResponseStatus(503)
    return { state: 'unavailable' }
  }
})
