import { notFound, redirect } from '@tanstack/react-router'
import { defaultLocale } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { isPublicID, readCatalogDetail, type Detail, type Kind } from './detail-server'
import { entityPath } from './public-path'
import styles from './detail.module.scss'

export type CatalogParams = { language?: string; publicId: string; slug?: string }
type DetailPage = {
  state: 'published'
  detail: Detail
  locale: Locale
  canonical?: string
  alternatives: { locale: Locale; path: string; url: string }[]
}
type Page = DetailPage | { state: 'unavailable'; locale: Locale }

export async function loadDetail(kind: Kind, params: CatalogParams, pathname: string, region: Region): Promise<Page> {
  const { language, publicId } = params
  // TanStack can also match a route as a prefix of a longer unknown path.
  if (!/^\/(?:(?:en|zh)\/)?(?:sites|games)\/[^/]+(?:\/[^/]+)?\/?$/.test(pathname)) throw notFound()
  if ((language !== undefined && language !== 'en' && language !== 'zh') || !isPublicID(publicId)) throw notFound()
  // Bare decimal legacy keys stay unmapped until an audited import. Full
  // typed paths also support the rare all-decimal 16-character Public ID.
  if (params.slug === undefined && /^[1-9][0-9]*$/.test(publicId)) throw notFound()
  const locale = language ?? defaultLocale(region)
  const result = await readCatalogDetail({ data: { kind, publicId, region } })
  if (result.state === 'missing') throw notFound()
  if (result.state === 'unavailable') return { state: 'unavailable', locale }
  const { detail, origin } = result
  const path = entityPath(kind, publicId, detail.slug, locale, region)
  if (pathname !== path) throw redirect({ href: path, statusCode: 301, headers: { 'Cache-Control': 'no-store' } })
  const alternatives = detail.localizations.map(entry => {
    const path = entityPath(kind, publicId, detail.slug, entry.locale, region)
    return { locale: entry.locale, path, url: new URL(path, origin).href }
  })
  return { state: 'published', detail, locale, canonical: alternatives.find(entry => entry.locale === locale)?.url, alternatives }
}

export function detailHeaders(data: Page | undefined): Record<string, string> {
  return data?.state === 'published' && data.canonical
    ? { 'Cache-Control': 'public, max-age=60' }
    : { 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }
}

export function detailHead(data: Page | undefined) {
  if (data?.state !== 'published') return { meta: [{ name: 'robots', content: 'noindex' }] }
  const translation = data.detail.localizations.find(entry => entry.locale === data.locale)
  const name = (translation ?? data.detail.localizations[0]).name
  return {
    meta: [
      { title: name },
      ...(translation?.summary ? [{ name: 'description', content: translation.summary }] : []),
      ...(!translation ? [{ name: 'robots', content: 'noindex' }] : []),
    ],
    links: [
      ...(data.canonical ? [{ rel: 'canonical', href: data.canonical }] : []),
      ...data.alternatives.map(item => ({ rel: 'alternate', hrefLang: item.locale, href: item.url })),
    ],
  }
}

const labels = {
  en: { unavailable: 'This detail is temporarily unavailable.', missing: 'This page was not found.', untranslated: 'An English translation is unavailable.', primary: 'Visit site', alternate: 'Alternate addresses', sources: 'Platforms' },
  zh: { unavailable: '详情暂时无法加载。', missing: '找不到此页面。', untranslated: '此页面暂无中文译文。', primary: '访问网站', alternate: '备用地址', sources: '发行平台' },
}

export function MissingDetail() {
  return <main className={styles.detail}><h1>404</h1><p lang="zh">{labels.zh.missing}</p><p lang="en">{labels.en.missing}</p></main>
}

export function DetailView({ data }: { data: Page }) {
  const copy = labels[data.locale]
  if (data.state === 'unavailable') return <main className={styles.detail}><h1>{copy.unavailable}</h1></main>
  const { detail, locale, alternatives } = data
  const translation = detail.localizations.find(entry => entry.locale === locale)
  const display = translation ?? detail.localizations[0]
  const primary = detail.kind === 'sites' ? detail.addresses.find(address => address.kind === 'primary') : undefined
  const alternates = detail.kind === 'sites' ? detail.addresses.filter(address => address.kind === 'alternate') : []
  return (
    <main className={styles.detail}>
      <h1 lang={display.locale}>{display.name}</h1>
      {translation?.summary && <p className={styles.summary}>{translation.summary}</p>}
      {!translation && <p>{copy.untranslated}</p>}
      {primary && <p><a href={primary.url} rel="noopener noreferrer">{copy.primary}</a></p>}
      {alternates.length > 0 && <section><h2>{copy.alternate}</h2><ul>{alternates.map(address =>
        <li key={address.url}><a href={address.url} rel="noopener noreferrer">{address.url}</a></li>)}</ul></section>}
      {detail.kind === 'games' && detail.sources.length > 0 && <section><h2>{copy.sources}</h2><ul>{detail.sources.map(source =>
        <li key={JSON.stringify([source.platform_code, source.external_key])}>{source.source_url
          ? <a href={source.source_url} rel="noopener noreferrer">{source.platform_code}</a>
          : source.platform_code}</li>)}</ul></section>}
      {alternatives.filter(item => item.locale !== locale).map(item =>
        <p key={item.locale}><a lang={item.locale} hrefLang={item.locale} href={item.path}>{item.locale === 'zh' ? '中文' : 'English'}</a></p>)}
    </main>
  )
}
