import type { Card, CategoryCode, Term } from '../navigation/data'
import { categoryCodes } from '../navigation/contracts'
import { safeBusinessURL, safeImageURL, tagCodes, validLanguage } from '../navigation/data'

export type Technology = {
  kind: 'http' | 'tls' | 'fingerprint'; source: string; ruleset: string
  last_observed_at: string; observation_state: 'last_observed' | 'recent_observation'; first_observed_at?: string
  technology?: string; confidence?: 'high' | 'medium'; http_status?: number
}
export type SiteStats = { detail_views?: number; outbound_clicks?: number; window_start: string; window_end: string }
export type SiteProfileData = {
  reviewed: boolean
  addresses: { kind: 'primary' | 'alternate'; url: string }[]
  primaryCategory?: CategoryCode
  secondaryCategories: CategoryCode[]
  tags: Term[]
  languages: string[]
  logo?: string
  technology: Technology[]
  yearbooks: number[]
  related: Card[]
  interactions: boolean
  stats?: SiteStats
}

function category(value: unknown): value is CategoryCode {
  return categoryCodes.some(code => code === value)
}

function validDate(value: unknown): value is string {
  return typeof value === 'string' && value.length <= 35 && Number.isFinite(Date.parse(value))
}

function decodeTechnology(value: unknown): Technology[] {
  if (!Array.isArray(value) || value.length > 5) return []
  return value.flatMap<Technology>(item => {
    if (!item || typeof item.source !== 'string' || !/^[a-z0-9.-]{1,253}$/.test(item.source) ||
      !validDate(item.last_observed_at) || !['last_observed', 'recent_observation'].includes(item.observation_state) ||
      !['web-passive-1', 'gofurry-evidence-1'].includes(item.ruleset)) return []
    const base = { source: item.source, ruleset: item.ruleset, last_observed_at: item.last_observed_at,
      observation_state: item.observation_state,
      first_observed_at: validDate(item.first_observed_at) ? item.first_observed_at : undefined }
    if (item.kind === 'http' && Number.isInteger(item.http_status) && item.http_status >= 100 && item.http_status <= 599) {
      return [{ ...base, kind: 'http', http_status: item.http_status } as const]
    }
    if (item.kind === 'tls') return [{ ...base, kind: 'tls' } as const]
    if (item.kind === 'fingerprint' && typeof item.technology === 'string' && /^[a-z0-9-]{1,30}$/.test(item.technology) &&
      (item.confidence === 'high' || item.confidence === 'medium')) {
      return [{ ...base, kind: 'fingerprint', technology: item.technology, confidence: item.confidence } as const]
    }
    return []
  })
}

export function decodeSiteProfile(item: Record<string, unknown>): SiteProfileData {
  const primaryCategory = category(item.primary_category) ? item.primary_category : undefined
  const reviewed = item.reviewed === true
  if (!Array.isArray(item.addresses) || item.addresses.length > 16) throw new Error('Invalid Site addresses')
  const addresses = reviewed ? item.addresses.flatMap<SiteProfileData['addresses'][number]>(value => {
    const url = safeBusinessURL(typeof value?.url === 'string' ? value.url : undefined)
    return url && (value.kind === 'primary' || value.kind === 'alternate') ? [{ kind: value.kind, url }] : []
  }) : []
  const tags = reviewed && Array.isArray(item.tags) && item.tags.length <= 8 ? item.tags.filter(tag =>
    tag && tagCodes.some(code => code === tag.code) && typeof tag.zh === 'string' && typeof tag.en === 'string') : []
  return {
    reviewed, addresses, primaryCategory: reviewed ? primaryCategory : undefined, tags,
    secondaryCategories: reviewed && Array.isArray(item.secondary_categories) ? item.secondary_categories.filter(category).slice(0, 2) : [],
    languages: reviewed && Array.isArray(item.supported_languages) ? item.supported_languages.filter(code => typeof code === 'string' && validLanguage(code)).slice(0, 8) : [],
    logo: reviewed ? safeImageURL(typeof item.logo_url === 'string' ? item.logo_url : undefined, 'logo') : undefined,
    technology: reviewed ? decodeTechnology(item.technology) : [], yearbooks: [], related: [], interactions: false,
  }
}

export function decodeSiteStats(value: unknown): SiteStats | undefined {
  if (!value || typeof value !== 'object') return
  const item = value as Record<string, unknown>
  if (!validDate(item.window_start) || !validDate(item.window_end) ||
    !/^\d{4}-\d{2}-\d{2}$/.test(item.window_start) || !/^\d{4}-\d{2}-\d{2}$/.test(item.window_end)) return
  const positive = (count: unknown) => typeof count === 'number' && Number.isSafeInteger(count) && count > 0 ? count : undefined
  const views = positive(item.detail_views), clicks = positive(item.outbound_clicks)
  return views || clicks ? { window_start: item.window_start, window_end: item.window_end, detail_views: views, outbound_clicks: clicks } : undefined
}
