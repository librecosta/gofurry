import { createServerFn } from '@tanstack/react-start'
import { setResponseStatus } from '@tanstack/react-start/server'
import { notFound, redirect } from '@tanstack/react-router'
import { publicOrigin } from '../catalog/public-origin.server'
import { defaultLocale, localizedPath } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { criteriaQuery, normalizeCriteria, type Criteria, type Directory, type Home } from './data'
import { navigationRegion, readDirectory, readHome } from './read.server'
import { copy } from './messages'

type PageType = 'home' | 'directory'
type Lookup = { page: PageType; locale: Locale; region: Region; criteria?: Criteria }
type Result = { state: 'unavailable' | 'invalid' } | { state: 'ready'; origin: string; data: Home | Directory }
export type Page = Result & { page: PageType; path: string; locale: Locale; region: Region; criteria?: Criteria }

const readPage = createServerFn({ method: 'GET' }).validator((input: Lookup) => {
  if (!input || !['home', 'directory'].includes(input.page) || !['en', 'zh'].includes(input.locale) || !['CN', 'INTL'].includes(input.region)) {
    throw new Error('Invalid Sites page request')
  }
  return input
}).handler(async ({ data: input }): Promise<Result> => {
  try {
    if (navigationRegion() !== input.region) throw new Error('Sites deployment mismatch')
    const criteria = input.criteria
    const normalized = normalizeCriteria(criteria ?? {})
    const data = input.page === 'home' ? await readHome() : await readDirectory(normalized, input.locale)
    if (!data) {
      setResponseStatus(400)
      return { state: 'invalid' }
    }
    return { state: 'ready', origin: publicOrigin(), data }
  } catch {
    setResponseStatus(503)
    return { state: 'unavailable' }
  }
})

export async function loadSites(page: PageType, language: string | undefined, pathname: string, rawQuery: string,
  region: Region, criteria?: Criteria, signal?: AbortSignal): Promise<Page> {
  const base = page === 'home' ? '/sites' : '/sites/directory'
  if (language !== undefined && language !== 'zh' && language !== 'en') throw notFound()
  const actual = language ? '/' + language + base : base
  if (pathname !== actual && pathname !== actual + '/') throw notFound()
  const locale = language ?? defaultLocale(region)
  const path = localizedPath(base, locale, region)
  const query = criteria ? criteriaQuery(criteria) : ''
  if (pathname !== path || rawQuery !== query) throw redirect({ href: path + query, statusCode: 301, headers: { 'Cache-Control': 'no-store' } })
  const result = await readPage({ data: { page, locale, region, criteria }, signal })
  return { ...result, page, locale, region, path, criteria }
}

export function sitesHead(data?: Page) {
  if (!data) return { meta: [{ name: 'robots', content: 'noindex' }] }
  const m = copy[data.locale]
  const title = (data.page === 'home' ? m.homeTitle : m.directory) + ' · GoFurry'
  const meta = [{ title }, { name: 'description', content: data.page === 'home' ? m.intro : m.directoryDescription }]
  if (data.state !== 'ready') return { meta: [...meta, { name: 'robots', content: 'noindex' }] }
  const indexable = !data.criteria || !criteriaQuery(data.criteria)
  const base = data.page === 'home' ? '/sites' : '/sites/directory'
  return {
    meta: indexable ? meta : [...meta, { name: 'robots', content: 'noindex, follow' }],
    links: [
      { rel: 'canonical', href: new URL(data.path, data.origin).href },
      ...(['zh', 'en'] as const).map(locale => ({ rel: 'alternate', hrefLang: locale, href: new URL(localizedPath(base, locale, data.region), data.origin).href })),
    ],
  }
}

export function sitesHeaders(data?: Page): Record<string, string> {
  if (data?.state !== 'ready') return { 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }
  return { 'Cache-Control': 'public, max-age=0, must-revalidate',
    ...(data.criteria && criteriaQuery(data.criteria) ? { 'X-Robots-Tag': 'noindex, follow' } : {}) }
}
