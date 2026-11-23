import { lazy, Suspense, useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowLeftIcon } from '@phosphor-icons/react/dist/ssr/ArrowLeft'
import { ArrowUpRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowUpRight'
import { GameControllerIcon } from '@phosphor-icons/react/dist/ssr/GameController'
import { entityPath } from '../catalog/public-path'
import type { Locale } from '../preferences'
import { gamePath, genres } from './data'
import { text } from './messages'
import { marketLabel, profileCopy } from './profile-copy'
import { money, profileMediaURL, sourceURL, storyURL, utcDate, type GameSource, type Profile, type PublicGameMedia, type StoreDetails, type Story } from './profile-data'
import { selectPublishedMedia } from './published-media'
import { readGameExtra, type ProfilePage } from './profile-load'
import { GameFrame } from './pages'
import styles from './profile.module.scss'

const Gallery = lazy(() => import('./profile-gallery'))
const Insights = lazy(() => import('./profile-insights'))
const idOf = (source: GameSource) => source.platform_code + '/' + source.external_key

function platform(source: GameSource) {
  if (source.platform_code === 'itch_io') return 'itch.io'
  return source.platform_code === 'steam' ? 'Steam' : source.platform_code
}

function Cover({ url, published, hero }: { url?: string; published?: string; hero?: boolean }) {
  const [failed, setFailed] = useState<string[]>([])
  const safe = [published, profileMediaURL(url)].find(link => link && !failed.includes(link))
  return <div className={styles.cover} data-hero-cover={hero || undefined}
    data-published-cover={published === safe && !!published || undefined}>
    <GameControllerIcon size={42} aria-hidden="true" />
    {safe && <img src={safe} alt="" width={460} height={215} loading={hero ? 'eager' : 'lazy'}
      decoding="async" referrerPolicy="no-referrer" onError={() => setFailed(current => [...current, safe])} />}
  </div>
}

function StoreRail({ sources, locale, primary }: { sources: GameSource[]; locale: Locale; primary?: GameSource }) {
  const m = profileCopy[locale]
  return <aside id="stores" className={styles.rail} aria-labelledby="official-stores">
    <h2 id="official-stores">{m.stores}</h2>
    {sources.map(source => {
      const href = sourceURL(source)
      return <section className={styles.store} key={idOf(source)}>
        <h3>{platform(source)} {source.platform_code === 'steam' && <small>· {source.external_key}</small>}</h3>
        {!source.active && <p className={styles.muted}>{m.historical}</p>}
        {source.quotes.map(q => <div key={q.store_country} className={styles.offer}>
          <span>{marketLabel[locale][q.store_country]} · {q.currency}</span>
          <strong>{q.state === 'free' ? m.free : q.amounts && money(q.amounts.final, q.currency, locale)}
            {q.state === 'zero_price' && <small> · {m.zeroPrice}</small>}</strong>
          {q.amounts && q.amounts.discount_percent > 0 && q.amounts.initial > q.amounts.final &&
            <span><s>{money(q.amounts.initial, q.currency, locale)}</s> · −{q.amounts.discount_percent}%</span>}
          <small>{m.observed} <time dateTime={q.observed_at}>{utcDate(q.observed_at, locale, true)}</time></small>
          {q.confirmed_at !== q.observed_at && <small>{m.confirmed} <time dateTime={q.confirmed_at}>{utcDate(q.confirmed_at, locale, true)}</time></small>}
        </div>)}
        {source.platform_code === 'steam' && !source.quotes.length && <p className={styles.muted}>{m.noQuote}</p>}
        {source.players && <p className={styles.players}><strong>{new Intl.NumberFormat(locale).format(source.players.count)}</strong> {m.players}
          <small><time dateTime={source.players.observed_at}>{utcDate(source.players.observed_at, locale, true)}</time></small></p>}
        {href && source !== primary ? <a className={styles.storeLink} href={href} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">
          {m.visit} {platform(source)} <ArrowUpRightIcon size={16} aria-hidden="true" /></a>
          : !href && <p className={styles.muted}>{m.unavailableLink}</p>}
      </section>
    })}
    {!sources.length && <p className={styles.muted}>{m.unavailableLink}</p>}
  </aside>
}

function Facts({ source, locale }: { source: GameSource & { details: StoreDetails }; locale: Locale }) {
  const m = profileCopy[locale]
  const { details: d } = source
  const systems = (['windows','mac','linux'] as const).filter(system => d.platforms?.[system]).map(system => m[system])
  const names = (v?: { id: string; name: string }[]) => v?.map(label => label.name).join(' · ')
  const classified = genres.filter(code => d.genre_codes?.includes(code)).map(code => text[locale][code]).join(' · ')
  const rows = [
    [m.developers, d.developers?.join(' · ')], [m.publishers, d.publishers?.join(' · ')],
    [m.genres, classified || names(d.genres)], [m.type, d.type], [m.categories, names(d.categories)], [m.languages, d.supported_languages],
  ]
  const phase = d.release?.coming_soon === true ? m.upcoming : d.release?.coming_soon === false ? m.released : undefined
  return <article className={styles.factBlock}>
    <h3>Steam · {source.external_key}</h3>
    <p className={styles.observation}>{marketLabel[locale][d.store_country]} · {d.locale} · {m.observed}{' '}
      <time dateTime={d.observed_at}>{utcDate(d.observed_at, locale)}</time>{d.status === 'historical' && ' · ' + m.historical}</p>
    {systems.length > 0 && <ul className={styles.chips}>{systems.map(system => <li key={system}>{system}</li>)}</ul>}
    <dl className={styles.facts}>{rows.map(([name, value]) => value && <div key={name}><dt>{name}</dt><dd lang={d.locale}>{value}</dd></div>)}
      {(phase || d.release?.date_text) && <div><dt>{m.release}</dt><dd>{phase}
        {phase && d.release?.date_text ? ' · ' : ''}{d.release_day ? <time dateTime={d.release_day}>{utcDate(d.release_day, locale)}</time>
          : <span lang={d.locale}>{d.release?.date_text}</span>}</dd></div>}
    </dl>
  </article>
}

function News({ profile, page }: { profile: Profile; page: ProfilePage }) {
  const m = profileCopy[page.locale]
  const [news, setNews] = useState(profile.news)
  const [status, setStatus] = useState<'idle' | 'loading' | 'failed'>('idle')
  const [unavailable, setUnavailable] = useState(profile.news_status === 'unavailable')
  async function load() {
    if (status === 'loading') return
    setStatus('loading')
    try {
      const reply = await readGameExtra({ data: { id: page.id, locale: page.locale, region: page.region, view: 'news', after: news.after } })
      if (reply.state !== 'ready' || !('items' in reply.data)) throw new Error('News unavailable')
      const updated = reply.data
      setNews(previous => ({ after: updated.after, items: [...previous.items, ...updated.items.filter(item =>
        !previous.items.some(p => p.app_id === item.app_id && p.gid === item.gid))] }))
      setUnavailable(false)
      setStatus('idle')
    } catch { setStatus('failed') }
  }
  const rows = news.items.flatMap((item: Story) => {
    const href = storyURL(item)
    return href ? [<li key={item.app_id + '/' + item.gid}>
      <p><span>Steam · {item.app_id}</span><time dateTime={item.published_at}>{utcDate(item.published_at, page.locale)}</time></p>
      <a href={href} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">{item.title} <ArrowUpRightIcon size={15} aria-hidden="true" /></a>
    </li>] : []
  })
  return <section id="news" className={styles.section}>
    <h2>{m.news}</h2>
    {rows.length > 0 ? <ol className={styles.news}>{rows}</ol>
      : <p className={styles.muted}>{unavailable ? m.newsError : m.noNews}</p>}
    {status === 'failed' && <p role="status">{m.newsError}</p>}
    {(news.after || unavailable) && <button type="button" className={styles.button}
      disabled={status === 'loading'} onClick={() => void load()}>{status === 'loading' ? m.loading : status === 'failed' ? m.retry : m.more}</button>}
  </section>
}

function GameMediaSection({ media, published, locale, steamLink }: {
  media: GameSource[]; published?: PublicGameMedia; locale: Locale; steamLink?: string
}) {
  const m = profileCopy[locale]
  const [open, setOpen] = useState(false)
  const noScriptLinks = published ? [
    ...published.screenshots.map((shot, i) => ({ label: m.screenshot + ' ' + (i + 1), url: shot.full_url ?? shot.thumbnail_url })),
    ...published.movies.map((movie, i) => ({
      label: movie.name || m.video + ' ' + (i + 1),
      url: movie.sources.mp4_max ?? movie.sources.mp4_480 ?? movie.sources.webm_max ?? movie.sources.webm_480 ?? steamLink,
    })),
    ...[...(published.cover ? [published.cover] : []), ...published.images].map((image, i) => ({
      label: m.artwork + ' ' + (i + 1), url: image.url,
    })),
  ] : media.flatMap(({ media: legacy }) => [
    ...(legacy?.screenshots?.length ? legacy.screenshots : [legacy?.header ?? legacy?.capsule]).map((url, i) => ({
      label: m.screenshot + ' ' + (i + 1), url: profileMediaURL(url),
    })),
    ...(legacy?.movies ?? []).map((url, i) => ({ label: m.video + ' ' + (i + 1), url: profileMediaURL(url, true) })),
  ])
  return <section id="media" className={styles.section}>
    <h2>{m.media}</h2>
    {published?.fallback && <p className={styles.observation}>{m.mediaFallback} {published.locale === 'en'
      ? <span lang="en">English</span> : <span lang="zh">中文</span>}</p>}
    <button type="button" className={styles.button + ' ' + styles.galleryToggle} aria-expanded={open} aria-controls="game-gallery"
      onClick={() => setOpen(value => !value)}>{open ? m.closeMedia : m.openMedia}</button>
    <noscript>
      <p>{m.mediaNoJS}</p>
      <ul>{noScriptLinks.map(({ label, url }, i) => <li key={i}>
        {url ? <a href={url} rel="noreferrer">{label}</a> : label}
      </li>)}</ul>
    </noscript>
    <div id="game-gallery">
      {open && <Suspense fallback={<p role="status">{m.loading}</p>}>
        <Gallery sources={media} published={published} locale={locale} />
      </Suspense>}
    </div>
  </section>
}

export function GameProfileView({ page }: { page: ProfilePage }) {
  const m = profileCopy[page.locale]
  const { id, locale, region } = page
  const publishedMedia = page.state === 'ready' ? page.profile.published_media : undefined
  const published = useMemo(() => selectPublishedMedia(publishedMedia), [publishedMedia])
  const [insights, setInsights] = useState(false)
  const path = page.state === 'ready' ? entityPath('games', id, page.profile.slug, locale, region) : gamePath('directory', locale, region)
  const frame = { locale, region, path, criteria: {} }
  if (page.state !== 'ready') return <GameFrame page={frame} profileDocument><h1 className={styles.unavailable}>{m.unavailable}</h1></GameFrame>
  const p = page.profile
  const translation = p.localizations.find(entry => entry.locale === locale)
  const display = translation ?? p.localizations[0]
  const steam = p.sources.filter(s => s.platform_code === 'steam')
  const media = published ? [] : steam.filter(s => s.media)
  const showMedia = !!published || media.length > 0
  const facts = steam.filter((s): s is GameSource & { details: StoreDetails } => !!s.details)
  const about = facts.find(s => s.details.description)
  const mainSource = p.sources.find(s => s.preferred && s.active && sourceURL(s)) ??
    steam.find(s => s.active && sourceURL(s)) ?? p.sources.find(s => s.active && sourceURL(s))
  const links = [['overview', m.about], ...(showMedia ? [['media',m.media]] : []),
    ['insights',m.insights], ['news',m.news], ...(facts.length ? [['facts',m.details]] : [])]
  return <GameFrame page={frame} profileDocument>
    <div className={styles.profile}>
      <Link className={styles.back} reloadDocument to={gamePath('directory', locale, region)}><ArrowLeftIcon size={17} aria-hidden="true" />{m.browse}</Link>
      <header className={styles.hero} data-game-profile>
        <div>
          <h1 lang={display?.locale}>{display?.name ?? id}</h1>
          {display?.summary && <p className={styles.lede} lang={display.locale}>{display.summary}</p>}
          {!translation && display && <p className={styles.muted}>{m.untranslated}</p>}
          <ul className={styles.chips}>{[...new Set(p.sources.filter(s => s.active).map(platform))].map(label => <li key={label}>{label}</li>)}
            {facts[0] && (['windows','mac','linux'] as const).filter(system => facts[0].details.platforms?.[system]).map(system => <li key={system}>{m[system]}</li>)}
          </ul>
          {mainSource && <a className={styles.primary} href={sourceURL(mainSource)} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">
            {m.visit} {platform(mainSource)} <ArrowUpRightIcon size={18} aria-hidden="true" /></a>}
        </div>
        {(p.cover_url || published?.cover) && <Cover url={p.cover_url} published={published?.cover?.url} hero />}
      </header>
      <nav className={styles.sections} aria-label={m.about}>{links.map(([key,label]) => <a key={key} href={'#' + key}>{label}</a>)}
        <a href="#stores">{m.stores}</a>
      </nav>
      <div className={styles.layout}>
        <div className={styles.content}>
          <section id="overview" className={styles.section}>
            <h2>{m.about}</h2>
            {about?.details.description ? <>
              <p className={styles.description} lang={about.details.locale}>{about.details.description}</p>
              <p className={styles.observation}>{m.fromSteam} · {about.external_key} · <time dateTime={about.details.observed_at}>{utcDate(about.details.observed_at, locale)}</time></p>
            </> : <p className={styles.description} lang={display?.locale}>{display?.summary ?? m.noPresentation}</p>}
          </section>
          {showMedia && <GameMediaSection key={id + '/' + locale} media={media} published={published} locale={locale}
            steamLink={steam.map(sourceURL).find(Boolean)} />}
          <section id="insights" className={styles.section}>
            <h2>{m.insights}</h2>
            {steam.length === 0 ? <p className={styles.muted}>{m.noSteam}</p> : insights
              ? <Suspense fallback={<p role="status">{m.loading}</p>}><Insights page={page} sources={steam} /></Suspense>
              : <button type="button" className={styles.button} onClick={() => setInsights(true)}>{m.openInsights}</button>}
          </section>
          <News key={id + '/' + locale} profile={p} page={page} />
          {facts.length > 0 && <section id="facts" className={styles.section}><h2>{m.details}</h2>
            {facts.map(source => <Facts key={idOf(source)} source={source} locale={locale} />)}
          </section>}
        </div>
        <StoreRail sources={p.sources} locale={locale} primary={mainSource} />
      </div>
      {p.related.length > 0 && <section className={styles.related}><h2>{m.related}</h2><ul>{p.related.map(item => {
        const name = item.localizations.find(entry => entry.locale === locale) ?? item.localizations[0]
        return <li key={item.public_id}><Link to={entityPath('games', item.public_id, item.slug, locale, region)}>
          <Cover url={item.cover_url} /><strong lang={name?.locale}>{name?.name ?? item.public_id}</strong>
        </Link></li>
      })}</ul></section>}
    </div>
  </GameFrame>
}
