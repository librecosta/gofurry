import { localizedPath } from '../locale-path'
import type { Locale, Region } from '../preferences'

export const genres = ['action', 'adventure', 'casual', 'free_to_play', 'indie', 'racing', 'rpg', 'simulation', 'sports', 'strategy'] as const
export const stores = ['steam', 'itch_io'] as const
export const systems = ['windows', 'mac', 'linux'] as const
export const phases = ['upcoming', 'released', 'unknown'] as const
export const prices = ['free', 'discount'] as const
export const sorts = ['added_desc', 'name_asc', 'release_desc', 'upcoming_asc'] as const

export type Copy = { locale: Locale; name: string; summary?: string }
export type Summary = { public_id: string; slug: string; localizations: Copy[]; first_published_at: string | null }
export type Game = Summary & {
  cover_url?: string
  platforms: { platform_code: string; external_key: string; source_url?: string }[]
  genres: typeof genres[number][]
  systems: typeof systems[number][]
  release_state: typeof phases[number]
  release?: { platform: 'steam'; app_id: string; text: string; precision: string; day: string | null; locale: 'en' | 'zh-cn' }
  price_summary?: { platform: 'steam'; app_id: string; store_country: 'cn' | 'us'; state: string; confirmed_at: string;
    amounts: { currency: 'CNY' | 'USD'; initial: number; final: number; discount_percent: number } | null }
}
export type List<T> = { items: T[]; after?: string }
export type Home = {
  featured: { slot: 'primary' | 'secondary_1' | 'secondary_2'; game: Game }[]
  recent: Game[]; upcoming: Game[]; released: Game[]; offers: Game[]
  genres: { code: typeof genres[number]; count: number }[]
  collections: Summary[]
  news: { public_id: string; slug: string; gid: string; app_id: string; title: string; url: string; published_at: string }[]
}
export type Collection = Summary & { members: List<Game> }
export type Kind = 'home' | 'directory' | 'collections' | 'members'
export type Criteria = { q?: string; genre?: string; store?: string; system?: string; phase?: string; price?: string; sort?: string; after?: string; view?: string }
export const publicID = /^[0123456789abcdefghjkmnpqrstvwxyz]{16}$/
export const publicSlug = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

export function gamePath(kind: Kind, locale: Locale, region: Region, entity?: Pick<Summary, 'public_id' | 'slug'>) {
  const suffix = kind === 'home' ? '' : kind === 'members' && entity
    ? '/collections/' + entity.public_id + '/' + entity.slug : '/' + kind
  return localizedPath('/games' + suffix, locale, region)
}

export function criteriaQuery(c: Criteria) {
  const query = new URLSearchParams()
  for (const key of ['q', 'genre', 'store', 'system', 'phase', 'price', 'sort', 'view', 'after'] as const) {
    const value = c[key]
    if (value && !(key === 'sort' && value === 'added_desc') && !(key === 'view' && value === 'editorial')) query.set(key, value)
  }
  return query.size ? '?' + query.toString() : ''
}

export function parseCriteria(raw: string, kind: Kind): Criteria | null {
  if (raw.length > 3500 || /%(?![0-9A-Fa-f]{2})/.test(raw)) return null
  const out: Criteria = {}
  const allowed = kind === 'directory' ? ['q','genre','store','system','phase','price','sort','after']
    : kind === 'collections' ? ['q','after'] : kind === 'members' ? ['view','after'] : []
  const seen = new Set<string>()
  for (const [key, value] of new URLSearchParams(raw)) {
    if (!allowed.includes(key) || seen.has(key) || /[\p{Cc}\uFFFD]/u.test(value)) return null
    seen.add(key)
    if (key === 'q') {
      if ([...value].length > 100) return null
      out.q = value.trim()
    } else if (key === 'genre') {
      const values = value.split(',')
      if (values.some(item => !genres.some(code => code === item)) || new Set(values).size !== values.length) return null
      out.genre = values.sort().join(',')
    } else if (key === 'after') {
      if (!/^[A-Za-z0-9_-]{1,2048}$/.test(value)) return null
      out.after = value
    } else {
      const choices = { store: stores, system: systems, phase: phases, price: prices, sort: sorts, view: ['editorial', 'timeline'] } as const
      if (!Object.hasOwn(choices, key)) return null
      const name = key as keyof typeof choices
      if (value && !choices[name].some(code => code === value)) return null
      if (value) out[name] = value
    }
  }
  return out
}

export function displayCopy(row: Summary, locale: Locale) {
  return row.localizations.find(item => item.locale === locale) ?? row.localizations[0]
}

export function mediaURL(raw?: string) {
  if (!raw || raw.length > 512 || /[\\<>\s\u0000-\u001f]/.test(raw)) return undefined
  try {
    const parsed = new URL(raw)
    const host = parsed.hostname
    if (parsed.protocol !== 'https:' || parsed.host !== host || parsed.username || parsed.password || parsed.search || parsed.hash ||
        parsed.pathname === '/' || /[\\<>\u0000-\u001f]/.test(decodeURIComponent(parsed.pathname))) return undefined
    if (host.endsWith('.steamstatic.com') || host.endsWith('.steamcontent.com') ||
        host === 'steamcdn-a.akamaihd.net' || host === 'steamuserimages-a.akamaihd.net') return parsed.href
  } catch { return undefined }
  return undefined
}
