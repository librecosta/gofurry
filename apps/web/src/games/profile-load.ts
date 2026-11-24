import { webConfig } from '../web-config.server'
import { createServerFn } from '@tanstack/react-start'
import { getRequest, setResponseStatus } from '@tanstack/react-start/server'
import { notFound, redirect } from '@tanstack/react-router'
import { entityPath } from '../catalog/public-path'
import { publicOrigin } from '../catalog/public-origin.server'
import type { CatalogParams } from '../catalog/detail'
import { defaultLocale } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { publicID, publicSlug, type List } from './data'
import { profileMediaPolicy } from './published-media'
import { countries, newsArticleID, profileMediaURL, storyURL, validApp, type NewsArticle, type Country, type Insights, type Profile, type PublicSteamChange, type Related, type Story } from './profile-data'

type Identity = { id: string; locale: Locale; region: Region }
type Extra = Identity & { app?: string; country?: Country; range?: number; after?: string }
type Page = { state: 'ready'; profile: Profile; origin: string }
export type ProfilePage = Identity & (Page & { article?: NewsArticle | null } | { state: 'unavailable' })

function validate<T extends Identity>(input: T): T {
  if (!input || typeof input.id !== 'string' || input.id.length !== 16 || !publicID.test(input.id) ||
      !['en','zh'].includes(input.locale) || !['CN','INTL'].includes(input.region)) throw new Error('Invalid game profile request')
  return input
}

async function fetchProfile(input: Identity, suffix: string, query: URLSearchParams, limit = 0) {
  const region = webConfig().region
  if (region !== input.region) throw new Error('Game API region mismatch')
  const url = new URL('/api/v1/games/' + input.id + suffix, webConfig().api_origin)
  url.search = query.toString()
  const response = await fetch(url, { signal: AbortSignal.any([AbortSignal.timeout(3000), getRequest().signal]), redirect: 'error' })
  if (response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW' || !response.ok) {
    await response.body?.cancel()
    if (response.status === 404 && response.headers.get('x-gofurry-region') === region &&
        response.headers.get('x-gofurry-content-mode') === 'SFW') return null
    throw new Error('Game API unavailable')
  }
  if (!limit) return await response.json()
  if (!response.body) throw new Error('Missing game API response')
  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8', { fatal: true })
  let size = 0
  let text = ''
  try {
    for (;;) {
      const { value, done } = await reader.read()
      if (done) return JSON.parse(text + decoder.decode())
      size += value.byteLength
      if (size > limit) throw new Error('Game API response exceeded its budget')
      text += decoder.decode(value, { stream: true })
    }
  } finally { await reader.cancel() }
}

function identityOK(p: Related | undefined, id?: string) {
  return p && p.public_id.length === 16 && publicID.test(p.public_id) && (id === undefined || id === p.public_id) &&
    p.slug.length <= 80 && publicSlug.exec(p.slug)?.[0] === p.slug && Array.isArray(p.localizations) && p.localizations.length <= 2 &&
    p.localizations.every(c => ['zh','en'].includes(c.locale) && typeof c.name === 'string')
}

const readProfile = createServerFn({ method: 'GET' }).validator(validate<Identity>).handler(async ({ data }): Promise<Page | { state: 'missing' | 'unavailable' }> => {
  try {
    const p: Profile | null = await fetchProfile(data, '', new URLSearchParams({ profile: '1', locale: data.locale }))
    if (!p) return { state: 'missing' }
    if (!identityOK(p, data.id) || !Array.isArray(p.sources) || p.sources.length > 16 ||
      !Array.isArray(p.related) || p.related.length > 6 || p.related.some(item => !identityOK(item)) ||
      !Array.isArray(p.news?.items) || p.news.items.length > 6) throw new Error('Invalid profile')
    return { state: 'ready', profile: p, origin: publicOrigin() }
  } catch {
    setResponseStatus(503)
    return { state: 'unavailable' }
  }
})

export const readGameExtra = createServerFn({ method: 'GET' }).validator((input: Extra & { view: 'insights' | 'news' }) => {
  validate(input)
  if (!['news','insights'].includes(input.view) || input.app !== undefined && !validApp(input.app) ||
      input.country !== undefined && !Object.hasOwn(countries, input.country) ||
      input.range !== undefined && ![7,30,90,365].includes(input.range) ||
      input.after !== undefined && !/^[A-Za-z0-9_-]{1,512}$/.test(input.after)) throw new Error('Invalid game insights request')
  return input
}).handler(async ({ data }): Promise<{ state: 'ready'; data: Insights | List<Story> } | { state: 'unavailable' }> => {
  try {
    const query = new URLSearchParams({ locale: data.locale })
    if (data.app) query.set('app_id', data.app)
    if (data.view === 'insights') {
      query.set('locale', data.locale)
      if (data.country) query.set('country', data.country)
      if (data.range) query.set('range', String(data.range))
    } else if (data.after) query.set('after', data.after)
    const result = await fetchProfile(data, '/' + data.view, query)
    if (!result) throw new Error('Game is unavailable')
    if (data.view === 'news') {
      if (!Array.isArray(result.items) || result.items.length > 6) throw new Error('Invalid news response')
    } else if (!Array.isArray(result.price_days) || result.price_days.length > 365 || !Array.isArray(result.player_days) || result.player_days.length > 365 ||
        !Array.isArray(result.price_changes) || result.price_changes.length > 60 || !Array.isArray(result.news) || result.news.length > 20) {
      throw new Error('Invalid insights response')
    }
    return { state: 'ready', data: result }
  } catch {
    return { state: 'unavailable' }
  }
})

export const readGameChanges = createServerFn({ method: 'GET' }).validator((input: Identity & { after?: string }) => {
  validate(input)
  if (input.after !== undefined && !/^\d{4}-\d{2}-\d{2}~[0-9a-f-]{36}$/.test(input.after)) throw new Error('Invalid change cursor')
  return input
}).handler(async ({ data }): Promise<List<PublicSteamChange> | null> => {
  try {
    const params = new URLSearchParams()
    if (data.after) params.set('after', data.after)
    const result: List<PublicSteamChange> | null = await fetchProfile(data, '/changes', params, 128 << 10)
    return result && Array.isArray(result.items) && result.items.length <= 30 ? result : null
  } catch { return null }
})

const readArticle = createServerFn({ method: 'GET' }).validator((input: Identity & { article: string }) => {
  validate(input)
  if (!newsArticleID(input.article)) throw new Error('Invalid article ID')
  return input
}).handler(async ({ data }): Promise<NewsArticle | null> => {
  try {
    const article: NewsArticle | null = await fetchProfile(data, '/news/' + data.article, new URLSearchParams(), 512 << 10)
    if (!article || article.id !== data.article || !['en', 'zh'].includes(article.locale) ||
      typeof article.title !== 'string' || typeof article.html !== 'string' || !validApp(article.app_id) ||
      !storyURL({ app_id: article.app_id, url: article.url, title: article.title, published_at: article.published_at, gid: '' })) return null
    return article
  } catch { return null }
})

export async function loadGameProfile(params: CatalogParams, pathname: string, region: Region, signal: AbortSignal, search = ''): Promise<ProfilePage> {
  const article = new URLSearchParams(search).get('article')
  const articleID = newsArticleID(article) ? article : undefined
  const { publicId: id, slug, language } = params
  const path = (language ? '/' + language : '') + '/games/' + id + (slug ? '/' + slug : '')
  if ((pathname !== path && pathname !== path + '/') || id.length !== 16 || !publicID.test(id) ||
      language !== undefined && language !== 'en' && language !== 'zh' || !slug && /^[1-9][0-9]*$/.test(id)) throw notFound()
  const locale = language ?? defaultLocale(region)
  const result = await readProfile({ data: { id, locale, region }, signal })
  if (result.state === 'missing') throw notFound()
  if (result.state !== 'ready') return { state: 'unavailable', locale, region, id }
  const canonicalPath = entityPath('games', id, result.profile.slug, locale, region)
  const articlePath = articleID ? '?article=' + articleID + '#news' : ''
  if (pathname !== canonicalPath) throw redirect({ href: canonicalPath + articlePath, statusCode: 301, headers: { 'Cache-Control': 'no-store' } })
  const fullArticle = articleID ? await readArticle({ data: { id, locale, region, article: articleID }, signal }) : undefined
  return { ...result, locale, region, id, article: fullArticle }
}

export function profileHead(page?: ProfilePage) {
  if (page?.state !== 'ready') return { meta: [{ name: 'robots', content: 'noindex' }] }
  const { profile, origin, id, locale, region } = page
  const translation = profile.localizations.find(c => c.locale === locale)
  const display = translation ?? profile.localizations[0]
  const name = display?.name ?? id
  const cover = profileMediaURL(profile.cover_url)
  const canonical = translation ? new URL(entityPath('games', id, profile.slug, locale, region), origin).href : undefined
  return {
    meta: [{ title: name }, { property: 'og:title', content: name }, { property: 'og:type', content: 'website' },
      ...(canonical ? [{ property: 'og:url', content: canonical }] : []),
      ...(cover ? [{ property: 'og:image', content: cover }] : []),
      ...(translation?.summary ? [{ name: 'description', content: translation.summary }, { property: 'og:description', content: translation.summary }] : []),
      ...(!translation ? [{ name: 'robots', content: 'noindex, follow' }] : []),
    ],
    links: [...(canonical ? [{ rel: 'canonical', href: canonical }] : []), ...profile.localizations.map(c => ({
      rel: 'alternate', hrefLang: c.locale, href: new URL(entityPath('games', id, profile.slug, c.locale, region), origin).href,
    }))],
  }
}

export function profileHeaders(page?: ProfilePage): Record<string, string> {
  // The catalog enters and leaves profiles via document navigation so this policy stays with the profile.
  const mediaHeaders = { 'Content-Security-Policy': profileMediaPolicy, 'Referrer-Policy': 'no-referrer' }
  if (page?.state !== 'ready' || !page.profile.localizations.some(c => c.locale === page.locale)) {
    return { ...mediaHeaders, 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }
  }
  return { ...mediaHeaders, 'Cache-Control': 'public, max-age=0, must-revalidate' }
}
