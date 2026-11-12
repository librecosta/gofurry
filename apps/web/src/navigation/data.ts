import { defaultStringifySearch } from '@tanstack/react-router'
import { localizedPath } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { categoryCodes } from './contracts'

export type CategoryCode = typeof categoryCodes[number]
export const tagCodes = ['forum', 'portfolio', 'fursuit', 'tutorial', 'wiki', 'directory', 'podcast', 'webcomic', 'roleplay', 'modding', 'convention', 'translation'] as const
export const sorts = ['relevance', 'published_desc', 'heat_7d', 'name_asc'] as const
export type Sort = typeof sorts[number]
export type Term = { code: string; zh: string; en: string }
export type Card = {
  public_id: string; slug: string; localizations: { locale: Locale; name: string; summary?: string }[]
  primary_category: CategoryCode; tags: string[]; languages: string[]; first_published_at: string | null
  logo_url?: string; cover_url?: string; visit_url?: string; rank?: number
}
type Heat = { status: 'ready' | 'cold' | 'stale' }
export type Home = {
  version: 1; featured: { primary: Card | null; secondary_1: Card | null; secondary_2: Card | null }
  recent: Card[]; trending: Card[]; random: Card | null; categories: (Term & { count: number })[]; heat: Heat; interactions: boolean; tag_registry: Term[]
}
export type Directory = {
  version: 1; items: Card[]; total: number; next?: string; sort: Sort; effective_sort: Sort
  heat: Heat; tag_registry: Term[]; language_registry: string[]; interactions: boolean
}

export type Criteria = { q: string; category: string[]; tags: string[]; language: string[]; sort?: Sort }
export const emptyCriteria: Criteria = { q: '', category: [], tags: [], language: [] }
export const validLanguage = (code: string) => /^[a-z]{2,3}$/.test(code)
export const normalizeKeyword = (text: string) => Array.from(text.replace(/\p{Cc}/gu, '').trim()).slice(0, 80).join('')
const oneOf = (allowed: readonly string[]) => (value: string) => allowed.includes(value)

function codes(value: unknown, allow: (code: string) => boolean) {
  const list = Array.isArray(value) ? value : [value]
  return [...new Set(list.flatMap(code => typeof code === 'string' ? code.split(',') : []).filter(allow))].sort().slice(0, 8)
}

export function normalizeCriteria(raw: Record<string, unknown>): Criteria {
  const q = normalizeKeyword(typeof raw.q === 'string' ? raw.q : JSON.stringify(raw.q) ?? '')
  const sort = sorts.find(sort => raw.sort === sort)
  return {
    q, category: codes(raw.category, oneOf(categoryCodes)), tags: codes(raw.tags, oneOf(tagCodes)),
    language: codes(raw.language, validLanguage), sort: sort && sort !== (q ? 'relevance' : 'published_desc') && sort !== 'relevance' ? sort : undefined,
  }
}

export function criteriaParams(c: Criteria) {
  return {
    q: c.q || undefined, category: c.category.slice().sort().join(',') || undefined,
    tags: c.tags.slice().sort().join(',') || undefined, language: c.language.slice().sort().join(',') || undefined,
    sort: c.sort,
  }
}
export const criteriaQuery = (criteria: Criteria) => defaultStringifySearch(criteriaParams(criteria))
export const directoryPath = (locale: Locale, region: Region) => localizedPath('/sites/directory', locale, region)
export const directoryHref = (locale: Locale, region: Region, criteria = emptyCriteria) => directoryPath(locale, region) + criteriaQuery(criteria)

export function apiQuery(criteria: Criteria, locale: Locale, cursor?: string) {
  const params = new URLSearchParams({ locale })
  for (const [key, value] of Object.entries(criteriaParams(criteria))) if (value) params.set(key, value)
  if (cursor) params.set('cursor', cursor)
  return params.toString()
}

export function safeBusinessURL(raw?: string) {
  if (!raw || raw.length > 2048 || /[\\\p{Cc}\s]/u.test(raw)) return
  const url = URL.parse(raw)
  if (url && (url.protocol === 'https:' || url.protocol === 'http:') && !url.username && !url.password && !url.hash) return url.href
}
export function safeImageURL(raw: string | undefined, role: 'logo' | 'cover') {
  if (!raw || raw.length > 2048) return
  const url = URL.parse(raw)
  const suffix = role === 'logo' ? /\/sites\/logos\/[a-f0-9]{64}\.webp$/ : /\/sites\/covers\/[a-f0-9]{64}\.avif$/
  if (url?.protocol === 'https:' && !url.username && !url.password && !url.search && !url.hash && suffix.test(url.pathname)) return url.href
}

function object(input: unknown): Record<string, unknown> {
  if (!input || typeof input !== 'object' || Array.isArray(input)) throw new Error('Invalid Sites response')
  return input as Record<string, unknown>
}
function assert(condition: unknown): asserts condition {
  if (!condition) throw new Error('Invalid Sites response')
}
function strings(value: unknown, max: number): value is string[] {
  return Array.isArray(value) && value.length <= max && value.every(value => typeof value === 'string')
}
function term(value: unknown): value is Term {
  const data = object(value)
  return ['code', 'zh', 'en'].every(key => typeof data[key] === 'string')
}
function card(value: unknown): Card {
  const c = object(value)
  assert(typeof c.public_id === 'string' && /^[0123456789abcdefghjkmnpqrstvwxyz]{16}$/.test(c.public_id) &&
    typeof c.slug === 'string' && /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(c.slug) && c.slug.length <= 80 &&
    typeof c.primary_category === 'string' && oneOf(categoryCodes)(c.primary_category) && strings(c.tags, 8) &&
    strings(c.languages, 8) && Array.isArray(c.localizations) && c.localizations.length >= 1 && c.localizations.length <= 2 &&
    (c.first_published_at === null || typeof c.first_published_at === 'string') &&
    c.localizations.every(value => {
      const l = object(value)
      return (l.locale === 'zh' || l.locale === 'en') && typeof l.name === 'string' && (l.summary === undefined || typeof l.summary === 'string')
    }))
  return {
    public_id: c.public_id, slug: c.slug, primary_category: c.primary_category as CategoryCode, localizations: c.localizations,
    tags: c.tags, languages: c.languages, first_published_at: c.first_published_at,
    visit_url: safeBusinessURL(typeof c.visit_url === 'string' ? c.visit_url : undefined),
    logo_url: safeImageURL(typeof c.logo_url === 'string' ? c.logo_url : undefined, 'logo'),
    cover_url: safeImageURL(typeof c.cover_url === 'string' ? c.cover_url : undefined, 'cover'),
    rank: typeof c.rank === 'number' && Number.isInteger(c.rank) && c.rank >= 1 && c.rank <= 11 ? c.rank : undefined,
  }
}
function cards(value: unknown, max: number) {
  assert(Array.isArray(value) && value.length <= max)
  return value.map(card)
}
function heat(value: unknown): Heat {
  const status = object(value).status
  assert(status === 'ready' || status === 'cold' || status === 'stale')
  return { status }
}
const optionalCard = (value: unknown) => value === null ? null : card(value)

export function decodeHome(value: unknown): Home {
  const data = object(value), featured = object(data.featured)
  assert(data.version === 1 && Array.isArray(data.categories) && data.categories.length === 8)
  const categories = data.categories.map(item => {
    const c = object(item)
    const count = c.count
    assert(term(c) && typeof count === 'number' && Number.isSafeInteger(count) && count >= 0)
    return { code: c.code, zh: c.zh, en: c.en, count }
  })
  assert(categoryCodes.every(code => categories.some(item => item.code === code)))
  const tags = data.tag_registry ?? []
  assert(Array.isArray(tags) && tags.length <= 12 && tags.every(term))
  return {
    version: 1, featured: { primary: optionalCard(featured.primary), secondary_1: optionalCard(featured.secondary_1), secondary_2: optionalCard(featured.secondary_2) },
    recent: cards(data.recent, 8), trending: cards(data.trending, 8), random: optionalCard(data.random), categories,
    heat: heat(data.heat), interactions: data.interactions === true, tag_registry: tags,
  }
}
export function decodeDirectory(value: unknown): Directory {
  const d = object(value)
  const sort = sorts.find(sort => sort === d.sort), effective = sorts.find(sort => sort === d.effective_sort)
  assert(d.version === 1 && typeof d.total === 'number' && Number.isSafeInteger(d.total) && d.total >= 0 &&
    (d.next === undefined || typeof d.next === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(d.next)) &&
    Array.isArray(d.tag_registry) && d.tag_registry.length <= 12 && d.tag_registry.every(term) &&
    (d.language_registry === undefined || strings(d.language_registry, 1024)) && sort && effective)
  return {
    version: 1, items: cards(d.items, 24), total: d.total, next: d.next, sort, effective_sort: effective,
    heat: heat(d.heat), tag_registry: d.tag_registry, language_registry: (d.language_registry ?? []).filter(validLanguage), interactions: d.interactions === true,
  }
}
