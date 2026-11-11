import { createServerFn } from '@tanstack/react-start'
import { setResponseStatus } from '@tanstack/react-start/server'
import { isPublicID } from '../catalog/detail-server'
import { publicOrigin } from '../catalog/public-origin.server'
import type { Locale, Region } from '../preferences'

export type Name = { locale: Locale; name: string }
export type FactKind = 'technology' | 'domain' | 'http' | 'dns' | 'tls' | 'rdap'
export type Fact = {
  kind: FactKind; value: string; previous?: string; host: string; capability: string; vantage: string; dimension?: string;
  ruleset: string; evidence: string; signals?: string[]; confidence: 'high' | 'medium' | 'low' | 'verified';
  detected: string; observed: string; reference: string; prior_known?: boolean
}
export type Coverage = {
  period_start: string; period_end: string; recorded_from: string; recorded_to: string; days: number; period_days: number;
  partial_year: boolean; months: number[]; unknown: { from: string; to: string }[];
  sources: { host: string; capability: string; scopes: number }[]; sources_omitted: number
}
export type Snapshot = {
  public_id: string; slug: string; year: number; revision: number; published_at: string; site_available: boolean; current_names: Name[];
  document: { algorithm: number; names: Name[]; coverage: Coverage; timeline: Fact[]; year_end: Fact[]; timeline_omitted: number; year_end_omitted: number }
}
type Input = { id: string; year: number; region: Region }
type Read = { state: 'missing' } | { state: 'unavailable' } | { state: 'published'; snapshot: Snapshot; origin: string }

export const validYear = (value: number) => Number.isInteger(value) && value >= 2000 && value <= 9998
const localeOK = (value: unknown): value is Locale => value === 'en' || value === 'zh'
const date = (value: unknown) => typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)
const instant = (value: unknown) => typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(value)
const bounded = (value: unknown, limit: number): value is string => typeof value === 'string' && value.length > 0 && value.length <= limit
const count = (value: unknown, max: number): value is number => typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= max
const namesOK = (names: unknown): names is Name[] => Array.isArray(names) && names.length <= 2 &&
  names.every(n => n && localeOK(n.locale) && bounded(n.name, 320)) && new Set(names.map(n => n.locale)).size === names.length

function factsOK(facts: unknown, max: number): facts is Fact[] {
  return Array.isArray(facts) && facts.length <= max && facts.every(f => f &&
    ['technology', 'domain', 'http', 'dns', 'tls', 'rdap'].includes(f.kind) &&
    ['high', 'medium', 'low', 'verified'].includes(f.confidence) && /^[a-f0-9]{20}$/.test(f.reference) &&
    bounded(f.value, 253) && bounded(f.host, 253) && bounded(f.capability, 12) && bounded(f.vantage, 96) &&
    bounded(f.ruleset, 96) && bounded(f.evidence, 96) && instant(f.detected) && instant(f.observed) &&
    (f.previous === undefined || bounded(f.previous, 253)) &&
    (f.dimension === undefined || bounded(f.dimension, 8)) &&
    (f.prior_known === undefined || typeof f.prior_known === 'boolean') &&
    (f.signals === undefined || Array.isArray(f.signals) && f.signals.length <= 3 && f.signals.every((s: unknown) => bounded(s, 96))))
}

function decode(value: unknown, input: Input): Snapshot {
  if (!value || typeof value !== 'object') throw new Error('Invalid yearbook')
  const y = value as Snapshot
  const d = y.document
  const c = d?.coverage
  if (y.public_id !== input.id || y.year !== input.year || !bounded(y.slug, 80) || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(y.slug) ||
    !count(y.revision, 2147483647) || y.revision === 0 || !instant(y.published_at) || typeof y.site_available !== 'boolean' || !namesOK(y.current_names) ||
    d?.algorithm !== 1 || !namesOK(d.names) || !d.names.length || !factsOK(d.timeline, 40) || !factsOK(d.year_end, 24) ||
    !count(d.timeline_omitted, 8192) || !count(d.year_end_omitted, 8192) || !c || !date(c.period_start) || !date(c.period_end) ||
    !date(c.recorded_from) || !date(c.recorded_to) || !count(c.days, 366) || ![365, 366].includes(c.period_days) || typeof c.partial_year !== 'boolean' ||
    !Array.isArray(c.months) || c.months.length !== 12 || !c.months.every(mask => count(mask, 0x7fffffff)) ||
    !Array.isArray(c.unknown) || c.unknown.length > 183 || !c.unknown.every(r => r && date(r.from) && date(r.to)) ||
    !Array.isArray(c.sources) || c.sources.length > 24 || !c.sources.every(s => s && bounded(s.host, 253) && bounded(s.capability, 12) && count(s.scopes, 4096)) ||
    !count(c.sources_omitted, 4096)) throw new Error('Invalid yearbook')
  return y
}

async function readBounded(response: Response) {
  const reader = response.body?.getReader()
  if (!reader) throw new Error('Missing yearbook')
  const decoder = new TextDecoder('utf-8', { fatal: true })
  const parts: string[] = []
  let size = 0
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > 70 * 1024) throw new Error('Yearbook exceeds its public bound')
      parts.push(decoder.decode(value, { stream: true }))
    }
    return JSON.parse(parts.join('') + decoder.decode()) as unknown
  } finally {
    await reader.cancel()
  }
}

export const readYearbook = createServerFn({ method: 'GET' }).validator((input: Input) => {
  if (!input || typeof input.id !== 'string' || !isPublicID(input.id) || !validYear(input.year) ||
    (input.region !== 'CN' && input.region !== 'INTL')) throw new Error('Invalid yearbook request')
  return input
}).handler(async ({ data }): Promise<Read> => {
  try {
    if ((process.env.WEB_REGION ?? 'CN') !== data.region) throw new Error('Region mismatch')
    const url = new URL(`/api/v1/sites/${data.id}/yearbooks/${data.year}`, process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321')
    const response = await fetch(url, { signal: AbortSignal.timeout(3000), redirect: 'error' })
    if (response.headers.get('x-gofurry-region') !== data.region || response.headers.get('x-gofurry-content-mode') !== 'SFW') {
      await response.body?.cancel()
      throw new Error('API deployment mismatch')
    }
    if (response.status !== 200) {
      await response.body?.cancel()
      if (response.status === 404) return { state: 'missing' }
      throw new Error('Yearbook unavailable')
    }
    return { state: 'published', snapshot: decode(await readBounded(response), data), origin: publicOrigin() }
  } catch {
    setResponseStatus(503)
    return { state: 'unavailable' }
  }
})
