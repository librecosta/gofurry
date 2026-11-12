import { useRef, useEffect, useState } from 'react'
import { ArrowUpRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowUpRight'
import { entityPath } from '../catalog/public-path'
import type { Locale, Region } from '../preferences'
import { type Card, type Term } from './data'
import { categories, copy } from './messages'
import { siteOutbound } from './site-interactions'
import styles from './sites.module.scss'

export type CardContext = { locale: Locale; region: Region; interactions: boolean; tags?: Term[] }

function AssetImage({ src, role, eager }: { src: string; role: 'logo' | 'cover'; eager?: boolean }) {
  const ref = useRef<HTMLImageElement>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => { if (ref.current?.complete && !ref.current.naturalWidth) setFailed(true) }, [])
  if (failed) return null
  return <img ref={ref} src={src} alt="" loading={eager ? 'eager' : 'lazy'} fetchPriority={eager ? 'high' : undefined}
    decoding="async" referrerPolicy="no-referrer" className={role === 'logo' ? styles.logoImage : styles.coverImage} onError={() => setFailed(true)} />
}

function localization(site: Card, locale: Locale) {
  return site.localizations.find(item => item.locale === locale) ?? site.localizations[0]!
}

function SiteLogo({ site, locale }: { site: Card; locale: Locale }) {
  const initial = Array.from(localization(site, locale).name.trim())[0]?.toLocaleUpperCase(locale)
  return <span className={styles.logo} aria-hidden="true"><span>{initial}</span>
    {site.logo_url && <AssetImage key={site.logo_url} src={site.logo_url} role="logo" />}
  </span>
}

function SiteName({ site, ctx }: { site: Card; ctx: CardContext }) {
  const label = localization(site, ctx.locale)
  return <a href={entityPath('sites', site.public_id, site.slug, ctx.locale, ctx.region)} lang={label.locale}>{label.name}</a>
}

function Visit({ site, ctx }: { site: Card; ctx: CardContext }) {
  if (!site.visit_url) return null
  const m = copy[ctx.locale]
  const visit = ctx.interactions ? (event: Parameters<typeof siteOutbound>[1]) => siteOutbound(site.public_id, event) : undefined
  return <a className={styles.visit} href={site.visit_url} target="_blank" rel="noopener noreferrer" onClick={visit} onAuxClick={visit}
    aria-label={`${m.visit} · ${localization(site, ctx.locale).name} (${m.newTab})`}>{m.visit}<ArrowUpRightIcon size={15} aria-hidden="true" /></a>
}

function addedDate(value: string | null, locale: Locale) {
  if (!value || !Number.isFinite(Date.parse(value))) return null
  return new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en-GB', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(value))
}

export function languageLabel(code: string, locale: Locale) {
  return new Intl.DisplayNames([locale], { type: 'language' }).of(code) ?? code
}

export function SiteCard({ site, ctx, recent }: { site: Card; ctx: CardContext; recent?: boolean }) {
  const label = localization(site, ctx.locale)
  const date = recent ? addedDate(site.first_published_at, ctx.locale) : null
  const facets = [...site.tags.slice(0, 2).map(tag => ctx.tags?.find(term => term.code === tag)?.[ctx.locale] ?? tag),
    ...site.languages.slice(0, 2).map(code => languageLabel(code, ctx.locale))]
  return <article className={styles.card} data-site-id={site.public_id}>
    <div className={styles.identity}><SiteLogo site={site} locale={ctx.locale} />
      <div><h3><SiteName site={site} ctx={ctx} /></h3><div className={styles.muted}>{categories[site.primary_category][ctx.locale]}</div></div>
    </div>
    {label.summary && <p className={styles.summary} lang={label.locale}>{label.summary}</p>}
    {facets.length > 0 && <ul className={styles.facets}>{facets.map((value, index) => <li key={index}>{value}</li>)}</ul>}
    <div className={styles.cardFooter}>{date && <time dateTime={site.first_published_at!}>{date}</time>}<Visit site={site} ctx={ctx} /></div>
  </article>
}

export function FeaturedCard({ site, ctx, primary }: { site: Card; ctx: CardContext; primary?: boolean }) {
  const label = localization(site, ctx.locale)
  return <article className={`${styles.feature} ${primary ? styles.featurePrimary : ''}`} data-site-id={site.public_id}>
    <div className={styles.featureCopy}>
      <SiteLogo site={site} locale={ctx.locale} />
      <div className={styles.featureLabel}>{categories[site.primary_category][ctx.locale]}</div>
      <h3><SiteName site={site} ctx={ctx} /></h3>
      {label.summary && <p className={styles.summary} lang={label.locale}>{label.summary}</p>}
      <Visit site={site} ctx={ctx} />
    </div>
    {site.cover_url && <div className={styles.cover}><AssetImage key={site.cover_url} src={site.cover_url} role="cover" eager={primary} /></div>}
  </article>
}

export function TrendingRow({ site, ctx }: { site: Card; ctx: CardContext }) {
  return <li className={styles.rankRow} value={site.rank} data-site-id={site.public_id}>
    <span className={styles.rank} aria-hidden="true">{site.rank ?? '–'}</span>
    <SiteLogo site={site} locale={ctx.locale} />
    <div className={styles.rankName}><SiteName site={site} ctx={ctx} /><div className={styles.muted}>{categories[site.primary_category][ctx.locale]}</div></div>
    <Visit site={site} ctx={ctx} />
  </li>
}
