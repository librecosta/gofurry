import { createServerFn } from '@tanstack/react-start'
import { getRequest, setResponseStatus } from '@tanstack/react-start/server'
import { notFound, redirect } from '@tanstack/react-router'
import { defaultLocale } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { publicOrigin } from '../catalog/public-origin.server'
import { criteriaQuery, displayCopy, gamePath, parseCriteria, publicID, type Collection, type Criteria, type Home, type Kind, type List, type Game, type Summary } from './data'
import { text } from './messages'
import { readGames } from './read.server'

type Lookup = { kind: Kind; locale: Locale; region: Region; query: string; id?: string }
type Result = { state: 'unavailable' | 'invalid' | 'missing' } | { state: 'ready'; origin: string; data: Home | List<Game> | List<Summary> | Collection }
export type Page = Result & { kind: Kind; locale: Locale; region: Region; path: string; criteria: Criteria; query: string }

const readPage = createServerFn({ method: 'GET' }).validator((input: Lookup) => {
  if (!input || !['home','directory','collections','members'].includes(input.kind) || !['en','zh'].includes(input.locale) ||
      !['CN','INTL'].includes(input.region) || typeof input.query !== 'string' ||
      (input.kind === 'members' && !publicID.test(input.id ?? ''))) throw new Error('Invalid game page request')
  return input
}).handler(async ({ data: input }): Promise<Result> => {
  const criteria = parseCriteria(input.query, input.kind)
  if (!criteria) {
    setResponseStatus(400)
    return { state: 'invalid' }
  }
  try {
    const result = await readGames(input.kind, input.locale, input.region, criteria, getRequest().signal, input.id)
    if (result.state === 'missing') return result
    if (result.state === 'invalid') {
      setResponseStatus(400)
      return result
    }
    return { ...result, origin: publicOrigin() }
  } catch {
    setResponseStatus(503)
    return { state: 'unavailable' }
  }
})

export async function loadGames(kind: Kind, params: { language?: string; publicId?: string; slug?: string },
  pathname: string, query: string, region: Region, signal: AbortSignal): Promise<Page> {
  const { language, publicId, slug } = params
  if (language !== undefined && language !== 'zh' && language !== 'en') throw notFound()
  const locale = language ?? defaultLocale(region)
  const base = kind === 'members' ? '/games/collections/' + publicId + (slug ? '/' + slug : '') : '/games' + (kind === 'home' ? '' : '/' + kind)
  const actual = (language ? '/' + language : '') + base
  if ((pathname !== actual && pathname !== actual + '/') || (kind === 'members' && !publicID.test(publicId ?? ''))) throw notFound()
  const criteria = parseCriteria(query, kind)
  // Validate URL state before canonicalizing. The server also validates keyset cursors.
  const result = await readPage({ data: { kind, locale, region, query, id: publicId }, signal })
  if (result.state === 'missing') throw notFound()
  const collection = result.state === 'ready' && 'members' in result.data ? result.data : undefined
  const path = kind === 'members' && !collection ? pathname : gamePath(kind, locale, region, collection)
  if (result.state === 'ready' && criteria && (path !== pathname || criteriaQuery(criteria) !== query)) {
    throw redirect({ href: path + criteriaQuery(criteria), statusCode: 301, headers: { 'Cache-Control': 'no-store' } })
  }
  return { ...result, kind, locale, region, path, query, criteria: criteria ?? {} }
}

export function gameHead(page?: Page) {
  if (page?.state !== 'ready') return { meta: [{ name: 'robots', content: 'noindex' }] }
  const m = text[page.locale]
  const collection = 'members' in page.data ? page.data : undefined
  const label = collection ? displayCopy(collection, page.locale) : undefined
  const translated = !collection || label?.locale === page.locale
  const name = label?.name ?? (page.kind === 'directory' ? m.directory : page.kind === 'collections' ? m.collections : m.home)
  const title = name + ' · GoFurry'
  const description = collection ? (translated ? label?.summary : undefined)
    : page.kind === 'directory' ? m.directoryIntro : page.kind === 'collections' ? m.collectionsIntro : m.intro
  const canonicalLocale = translated ? page.locale : label?.locale ?? page.locale
  const canonical = new URL(gamePath(page.kind, canonicalLocale, page.region, collection), page.origin).href
  const alternates = collection ? collection.localizations.map(item => item.locale) : ['en','zh'] as const
  return {
    meta: [{ title }, { property: 'og:title', content: title }, { property: 'og:type', content: 'website' },
      { property: 'og:url', content: canonical },
      ...(description ? [{ name: 'description', content: description }, { property: 'og:description', content: description }] : []),
      ...(!translated || page.query ? [{ name: 'robots', content: 'noindex, follow' }] : []),
    ],
    links: [{ rel: 'canonical', href: canonical }, ...alternates.map(locale => ({
      rel: 'alternate', hrefLang: locale, href: new URL(gamePath(page.kind, locale, page.region, collection), page.origin).href,
    }))],
  }
}

export function gameHeaders(page?: Page): Record<string,string> {
  if (page?.state !== 'ready') return { 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }
  const noIndex = !!page.query || ('members' in page.data && !page.data.localizations.some(item => item.locale === page.locale))
  return { 'Cache-Control': 'public, max-age=0, must-revalidate', ...(noIndex ? { 'X-Robots-Tag': 'noindex, follow' } : {}) }
}
