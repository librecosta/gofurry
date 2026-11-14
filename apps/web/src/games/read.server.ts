import type { Locale, Region } from '../preferences'
import { criteriaQuery, publicID, publicSlug, type Collection, type Criteria, type Game, type Home, type Kind, type List, type Summary } from './data'

function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function summary(value: unknown): value is Summary {
  return record(value) && typeof value.public_id === 'string' && publicID.test(value.public_id) &&
    typeof value.slug === 'string' && value.slug.length <= 80 && publicSlug.test(value.slug) &&
    Array.isArray(value.localizations) && value.localizations.length > 0 && value.localizations.length <= 2 &&
    value.localizations.every(item => record(item) && (item.locale === 'en' || item.locale === 'zh') &&
      typeof item.name === 'string' && item.name.length > 0 && (item.summary === undefined || typeof item.summary === 'string'))
}

function game(value: unknown): value is Game {
  return summary(value) && 'platforms' in value && Array.isArray(value.platforms) && 'genres' in value && Array.isArray(value.genres) &&
    'systems' in value && Array.isArray(value.systems) && 'release_state' in value &&
    ['released','upcoming','unknown'].some(phase => phase === value.release_state)
}

function news(value: unknown) {
  if (!record(value) || typeof value.app_id !== 'string' || !/^[1-9]\d{0,9}$/.test(value.app_id) ||
      typeof value.public_id !== 'string' || !publicID.test(value.public_id) || typeof value.slug !== 'string' || !publicSlug.test(value.slug) ||
      typeof value.title !== 'string' || typeof value.gid !== 'string' || typeof value.published_at !== 'string' || typeof value.url !== 'string') return false
  return new RegExp('^https://(?:steamcommunity\\.com/games/' + value.app_id +
    '/announcements/detail/[0-9]+/?|store\\.steampowered\\.com/news/app/' + value.app_id + '/view/[0-9]+/?)$').test(value.url)
}

function list<T>(value: unknown, isItem: (candidate: unknown) => candidate is T): value is List<T> {
  return record(value) && Array.isArray(value.items) && value.items.length <= 48 && value.items.every(isItem) &&
    (value.after === undefined || (typeof value.after === 'string' && /^[A-Za-z0-9_-]{1,2048}$/.test(value.after)))
}

function decode(kind: Kind, value: unknown): Home | List<Game> | List<Summary> | Collection {
  if (kind === 'directory' && list(value, game)) return value
  if (kind === 'collections' && list(value, summary)) return value
  if (kind === 'members' && summary(value) && 'members' in value && list(value.members, game)) return { ...value, members: value.members }
  if (kind === 'home' && record(value) && Array.isArray(value.featured) && value.featured.every(item =>
      record(item) && ['primary','secondary_1','secondary_2'].includes(String(item.slot)) && game(item.game)) &&
      (['recent','upcoming','released','offers'] as const).every(key => Array.isArray(value[key]) && value[key].every(game)) &&
      Array.isArray(value.genres) && Array.isArray(value.collections) && value.collections.every(summary) &&
      Array.isArray(value.news) && value.news.every(news)) return value as Home
  throw new Error('Invalid game channel response')
}

export async function readGames(kind: Kind, locale: Locale, region: Region, criteria: Criteria, signal: AbortSignal, id?: string) {
  if ((process.env.WEB_REGION ?? 'CN') !== region) throw new Error('Game deployment mismatch')
  const resource = kind === 'members' ? 'collections/' + id : kind === 'home' ? 'discovery' : kind
  const url = new URL('/api/v1/games/' + resource, process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321')
  url.search = criteriaQuery(criteria)
  url.searchParams.set('locale', locale)
  const response = await fetch(url, { redirect: 'error', signal: AbortSignal.any([signal, AbortSignal.timeout(3000)]) })
  if (response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW') {
    await response.body?.cancel()
    throw new Error('Game API region or visibility mismatch')
  }
  if (!response.ok) {
    await response.body?.cancel()
    if (response.status === 400) return { state: 'invalid' } as const
    if (response.status === 404 && kind === 'members') return { state: 'missing' } as const
    throw new Error('Game API unavailable')
  }
  return { state: 'ready', data: decode(kind, await response.json()) } as const
}
