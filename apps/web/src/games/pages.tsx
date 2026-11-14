import { Link, useNavigate } from '@tanstack/react-router'
import { GameControllerIcon } from '@phosphor-icons/react/dist/ssr/GameController'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowRight'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { localizedPath } from '../locale-path'
import type { Locale } from '../preferences'
import { criteriaQuery, displayCopy, gamePath, genres, mediaURL, phases, prices, sorts, stores, systems,
  type Collection, type Criteria, type Game, type Home, type List, type Summary } from './data'
import type { Page } from './load'
import { text } from './messages'
import styles from './games.module.scss'

function Frame({ page, children }: { page: Page; children: ReactNode }) {
  const { locale, region } = page
  const m = text[locale]
  const alternate = locale === 'en' ? 'zh' : 'en'
  const href = localizedPath(page.path, alternate, region) + criteriaQuery({ ...page.criteria, after: undefined })
  return <div className={styles.page}>
    <header className={styles.masthead}>
      <Link className={styles.brand} to={gamePath('home', locale, region)}>
        <GameControllerIcon size={22} aria-hidden="true" />GoFurry
      </Link>
      <nav aria-label={m.home}>
        <Link to={gamePath('home', locale, region)} activeOptions={{ exact: true, includeSearch: false }}>{m.home}</Link>
        <Link to={gamePath('directory', locale, region)} activeOptions={{ exact: true, includeSearch: false }}>{m.directory}</Link>
        <Link to={gamePath('collections', locale, region)} activeOptions={{ includeSearch: false }}>{m.collections}</Link>
        <Link to={href} lang={alternate} hrefLang={alternate} preload={false}>{m.language}</Link>
      </nav>
    </header>
    <main className={styles.main}>{children}</main>
    <footer className={styles.footer}><span>GoFurry</span><Link to={localizedPath('/start', locale, region)}>{m.start}</Link></footer>
  </div>
}

function dateLabel(raw: string, locale: Locale) {
  const date = new Date(raw.length === 10 ? raw + 'T00:00:00Z' : raw)
  if (Number.isNaN(date.valueOf())) return raw
  return new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en', { dateStyle: 'medium', timeZone: 'UTC' }).format(date)
}

function gameHref(row: Pick<Summary, 'public_id' | 'slug'>, page: Page) {
  return localizedPath('/games/' + row.public_id + '/' + row.slug, page.locale, page.region)
}

function Cover({ game, eager }: { game: Game; eager: boolean }) {
  const src = mediaURL(game.cover_url)
  const [failed, setFailed] = useState<string>()
  return <div className={styles.cover}>
    <GameControllerIcon size={36} aria-hidden="true" />
    {src && src !== failed && <img src={src} alt="" width={460} height={215}
      loading={eager ? 'eager' : 'lazy'} decoding="async" referrerPolicy="no-referrer" onError={() => setFailed(src)} />}
  </div>
}

function Card({ game, page, eager = false, compact = false }: { game: Game; page: Page; eager?: boolean; compact?: boolean }) {
  const m = text[page.locale]
  const localized = displayCopy(game, page.locale)
  const quote = game.price_summary
  const amounts = quote?.amounts
  const price = amounts && Number.isSafeInteger(amounts.final) && amounts.final >= 0 && ['CNY','USD'].includes(amounts.currency)
    ? new Intl.NumberFormat(page.locale === 'zh' ? 'zh-CN' : 'en', { style: 'currency', currency: amounts.currency }).format(amounts.final / 100)
    : quote?.state === 'free' ? m.free : undefined
  const release = game.release
  const platforms = [...new Set(game.platforms.map(platform => platform.platform_code))]
  return <article className={compact ? styles.compact : styles.card}>
    {!compact && <Cover game={game} eager={eager} />}
    <div className={styles.cardBody}>
      <h3><Link to={gameHref(game, page)} lang={localized.locale}>{localized.name}</Link></h3>
      {!compact && localized.summary && <p className={styles.summary} lang={localized.locale}>{localized.summary}</p>}
      <ul className={styles.markers}>
        {platforms.map(code => <li key={code}>{code === 'steam' || code === 'itch_io' ? m[code] : code}</li>)}
        {game.genres.slice(0, 3).map(code => <li key={code}>{m[code]}</li>)}
      </ul>
      {game.systems.length > 0 && !compact && <p className={styles.metadata}>{game.systems.map(os => m[os]).join(' · ')}</p>}
      <p className={styles.metadata}>
        {release && <span>{m.steamRelease} · </span>}{m[game.release_state]}
        {release?.day ? <> · <time dateTime={release.day}>{dateLabel(release.day, page.locale)}</time></>
          : release?.text && release.precision !== 'tba' ? <> · <span lang={release.locale}>{release.text}</span></>
            : game.release_state === 'upcoming' ? <> · {m.tba}</> : null}
      </p>
      {game.first_published_at && !compact && <p className={styles.metadata}>
        {m.recent} · <time dateTime={game.first_published_at}>{dateLabel(game.first_published_at, page.locale)}</time>
      </p>}
      {price && quote && <p className={styles.quote}>
        <strong>{price}</strong> {amounts && amounts.discount_percent > 0 && amounts.final < amounts.initial ? '−' + amounts.discount_percent + '%' : ''}
        <small>Steam · {quote.store_country.toUpperCase()} · {m.quoteDate} <time dateTime={quote.confirmed_at}>{dateLabel(quote.confirmed_at, page.locale)}</time></small>
      </p>}
    </div>
  </article>
}

function CollectionTile({ item, page }: { item: Summary; page: Page }) {
  const localized = displayCopy(item, page.locale)
  return <article className={styles.collectionTile} lang={localized.locale}>
    <h3><Link to={gamePath('members', page.locale, page.region, item)}>{localized.name}<ArrowRightIcon size={18} aria-hidden="true" /></Link></h3>
    {localized.summary && <p>{localized.summary}</p>}
  </article>
}

function SearchField({ label, hint, value, onChange, onComposition, onCommit }: {
  label: string; hint?: string; value?: string; onChange?: (value: string) => void;
  onComposition?: (active: boolean, value: string) => void; onCommit?: React.KeyboardEventHandler<HTMLInputElement>
}) {
  return <label className={styles.searchBox}>
    <MagnifyingGlassIcon size={21} aria-hidden="true" />
    <span className={styles.srOnly}>{label}</span>
    <input type="search" name="q" value={value} maxLength={100} placeholder={hint ?? label}
      onChange={onChange ? event => onChange(event.target.value) : undefined}
      onCompositionStart={event => onComposition?.(true, event.currentTarget.value)}
      onCompositionEnd={event => onComposition?.(false, event.currentTarget.value)} onKeyDown={onCommit} />
  </label>
}

function Lane({ heading, items, page, compact = false }: { heading: string; items: Game[]; page: Page; compact?: boolean }) {
  if (!items.length) return null
  if (compact) return <section className={styles.lane}><h2>{heading}</h2>
    <ul className={styles.stack}>{items.map(game => <li key={game.public_id}><Card game={game} page={page} compact /></li>)}</ul></section>
  return <section className={styles.lane}><h2>{heading}</h2>
    <div className={styles.grid}>{items.map(game => <Card key={game.public_id} game={game} page={page} />)}</div></section>
}

function HomePage({ page, home }: { page: Page; home: Home }) {
  const m = text[page.locale]
  const directory = gamePath('directory', page.locale, page.region)
  const genresAvailable = home.genres.filter(genre => genre.count > 0)
  return <>
    <section className={styles.hero}>
      <h1>{m.home}</h1>
      <p>{m.intro}</p>
      <form action={directory} className={styles.search}>
        <SearchField label={m.search} hint={m.searchHint} /><button className={styles.primary}>{m.submit}</button>
      </form>
      <Link className={styles.inline} to={directory}>{m.browse}<ArrowRightIcon size={18} aria-hidden="true" /></Link>
    </section>
    {home.featured.length > 0 && <section className={styles.lane}>
      <h2>{m.featured}</h2>
      <div className={styles.featureGrid}>{home.featured.map(({ slot, game }) =>
        <div className={slot === 'primary' ? styles.featurePrimary : styles.featureSecondary} key={slot}>
          <Card game={game} page={page} eager={slot === 'primary'} />
        </div>)}</div>
    </section>}
    <Lane heading={m.recent} page={page} items={home.recent} />
    {!home.recent.length && <p className={styles.empty}>{m.emptyHome}</p>}
    {(home.upcoming.length > 0 || home.released.length > 0) && <div className={styles.pair}>
      <Lane heading={m.upcoming} page={page} items={home.upcoming} compact />
      <Lane heading={m.recentReleased} page={page} items={home.released} compact />
    </div>}
    {genresAvailable.length > 0 && <section className={styles.lane}>
      <h2>{m.explore}</h2>
      <ul className={styles.genres}>{genresAvailable.map(item => <li key={item.code}>
        <Link to={directory + criteriaQuery({ genre: item.code })}>{m[item.code]} <span>{item.count}</span></Link>
      </li>)}</ul>
    </section>}
    <Lane heading={m.offers} page={page} items={home.offers} />
    {home.collections.length > 0 && <section className={styles.lane}><h2>{m.curated}</h2>
      <div className={styles.collectionGrid}>{home.collections.map(item =>
        <CollectionTile key={item.public_id} item={item} page={page} />)}</div>
    </section>}
    {home.news.length > 0 && <section className={styles.lane}><h2>{m.news}</h2>
      <ul className={styles.news}>{home.news.map(item => <li key={item.public_id + ':' + item.gid}>
        <div><a href={item.url} rel="noopener noreferrer">{item.title}</a><Link to={gameHref(item, page)}>{m.steam} · {item.app_id}</Link></div>
        <time dateTime={item.published_at}>{dateLabel(item.published_at, page.locale)}</time>
      </li>)}</ul>
    </section>}
  </>
}

function DirectoryFilters({ page }: { page: Page }) {
  const m = text[page.locale]
  const navigate = useNavigate()
  const [draft, setDraft] = useState(page.criteria)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const composing = useRef(false)
  useEffect(() => {
    clearTimeout(timer.current)
    setDraft(page.criteria)
    return () => clearTimeout(timer.current)
  }, [page.query])

  function submit(state: Criteria, replace = false) {
    clearTimeout(timer.current)
    const query = criteriaQuery({ ...state, q: state.q?.trim(), after: undefined })
    if (query !== page.query) void navigate({ to: page.path + query, replace, resetScroll: false })
  }
  function changeKeyword(value: string, ready: boolean) {
    clearTimeout(timer.current)
    const next = { ...draft, q: value }
    setDraft(next)
    if (ready) timer.current = setTimeout(() => submit(next, true), 300)
  }
  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!composing.current) submit(draft)
  }
  const set = (key: keyof Criteria, value: string) => {
    clearTimeout(timer.current)
    setDraft(prev => ({ ...prev, [key]: value }))
  }
  const selectedGenres = draft.genre?.split(',').filter(Boolean) ?? []
  const dimensions = [['store', stores], ['system', systems], ['phase', phases], ['price', prices], ['sort', sorts]] as const
  return <form className={styles.filters} action={page.path} onSubmit={handleSubmit}>
    <div className={styles.search}>
      <SearchField label={m.search} hint={m.searchHint} value={draft.q ?? ''}
        onChange={value => changeKeyword(value, !composing.current)}
        onComposition={(active, value) => { composing.current = active; changeKeyword(value, !active) }}
        onCommit={event => { if (event.key === 'Enter' && (event.nativeEvent.isComposing || composing.current)) event.preventDefault() }} />
      <button className={styles.primary}>{m.submit}</button>
    </div>
    <div className={styles.selects}>
      {dimensions.map(([key, options]) => <label key={key}>{m[key === 'sort' ? 'order' : key]}
        <select name={key} value={draft[key] ?? (key === 'sort' ? 'added_desc' : '')} onChange={event => set(key, event.target.value)}>
          {key !== 'sort' && <option value="">{m.all}</option>}
          {options.map(value => <option value={value} key={value}>{m[value]}</option>)}
        </select>
      </label>)}
    </div>
    <fieldset className={styles.genreFilters}>
      <legend>{m.genre}</legend><div>
        {genres.map(code => <label key={code}><input type="checkbox" checked={selectedGenres.includes(code)}
          onChange={event => set('genre', (event.target.checked ? [...selectedGenres, code] : selectedGenres.filter(item => item !== code)).sort().join(','))} />
          {m[code]}</label>)}
      </div>
      {draft.genre && <input type="hidden" name="genre" value={draft.genre} />}
    </fieldset>
    {page.query && <Link className={styles.inline} to={page.path}>{m.reset}</Link>}
  </form>
}

function Pagination({ page, after }: { page: Page; after?: string }) {
  const m = text[page.locale]
  if (!after && !page.criteria.after) return null
  return <nav aria-label={m.directory} className={styles.pagination}>
    {page.criteria.after && <Link to={page.path + criteriaQuery({ ...page.criteria, after: undefined })}>{m.first}</Link>}
    {after && <Link rel="next" to={page.path + criteriaQuery({ ...page.criteria, after })}>{m.more}<ArrowRightIcon size={17} aria-hidden="true" /></Link>}
  </nav>
}

function DirectoryPage({ page, list }: { page: Page; list: List<Game> }) {
  const m = text[page.locale]
  return <>
    <section className={styles.pageHeading}><h1>{m.directory}</h1><p>{m.directoryIntro}</p></section>
    <DirectoryFilters page={page} />
    <div className={styles.results} aria-live="polite">
      {list.items.length > 0 ? <div className={styles.grid} aria-label={m.directory}>
        {list.items.map(game => <Card key={game.public_id} game={game} page={page} />)}
      </div> : <p className={styles.empty}>{m.emptyGames}</p>}
    </div>
    <Pagination page={page} after={list.after} />
  </>
}

function CollectionsPage({ page, list }: { page: Page; list: List<Summary> }) {
  const m = text[page.locale]
  return <>
    <section className={styles.pageHeading}><h1>{m.collections}</h1><p>{m.collectionsIntro}</p></section>
    <form action={page.path} className={styles.search} key={page.query}>
      <label className={styles.searchBox}>
        <MagnifyingGlassIcon size={21} aria-hidden="true" /><span className={styles.srOnly}>{m.searchCollections}</span>
        <input type="search" name="q" defaultValue={page.criteria.q} maxLength={100} placeholder={m.searchCollections} />
      </label>
      <button className={styles.primary}>{m.submit}</button>
    </form>
    <div className={styles.results}>
      {list.items.length > 0 ? <div className={styles.collectionGrid}>{list.items.map(item =>
        <CollectionTile key={item.public_id} item={item} page={page} />)}</div> : <p className={styles.empty}>{m.emptyCollections}</p>}
    </div>
    <Pagination page={page} after={list.after} />
  </>
}

function timelinePhase(game: Game) {
  return game.release?.day && game.release_state !== 'unknown' ? game.release_state : 'groupUnknown'
}

function CollectionPage({ page, collection }: { page: Page; collection: Collection }) {
  const m = text[page.locale]
  const heading = displayCopy(collection, page.locale)
  const items = collection.members.items
  const timeline = page.criteria.view === 'timeline'
  const groups = ['released','upcoming','groupUnknown'] as const
  const hasDates = items.some(item => timelinePhase(item) !== 'groupUnknown')
  return <>
    <section className={styles.pageHeading}>
      <Link className={styles.inline} to={gamePath('collections', page.locale, page.region)}>{m.collections}</Link>
      <h1 lang={heading.locale}>{heading.name}</h1>
      {heading.summary && <p lang={heading.locale}>{heading.summary}</p>}
      {heading.locale !== page.locale && <p className={styles.metadata}>{m.fallback}</p>}
    </section>
    {(timeline || hasDates || !!page.criteria.after || !!collection.members.after) && <nav className={styles.switcher} aria-label={m.collectionOrder}>
      <Link to={page.path} aria-current={!timeline ? 'page' : undefined}>{m.editorial}</Link>
      <Link to={page.path + '?view=timeline'} aria-current={timeline ? 'page' : undefined}>{m.timeline}</Link>
    </nav>}
    {!items.length ? <p className={styles.empty}>{m.emptyMembers}</p> : timeline
      ? groups.map(group => <Lane key={group} heading={m[group]} items={items.filter(game => timelinePhase(game) === group)} page={page} />)
      : <div className={styles.grid}>{items.map(game => <Card key={game.public_id} game={game} page={page} />)}</div>}
    <Pagination page={page} after={collection.members.after} />
  </>
}

export function GameChannelPage({ page }: { page: Page }) {
  const m = text[page.locale]
  let body: ReactNode
  if (page.state !== 'ready') body = <section className={styles.pageHeading}>
    <h1>{page.state === 'invalid' ? m.invalid : m.unavailable}</h1>
    <a className={styles.inline} href={page.path + (page.state === 'invalid' ? '' : page.query)}>
      {page.state === 'invalid' ? m.reset : m.retry}<ArrowRightIcon size={18} aria-hidden="true" /></a>
  </section>
  else if ('featured' in page.data) body = <HomePage page={page} home={page.data} />
  else if ('members' in page.data) body = <CollectionPage page={page} collection={page.data} />
  else if (page.kind === 'directory') body = <DirectoryPage page={page} list={page.data as List<Game>} />
  else body = <CollectionsPage page={page} list={page.data} />
  return <Frame page={page}>{body}</Frame>
}

export function MissingGamePage() {
  return <main className={styles.missing}><h1>404</h1><p lang="zh">找不到此游戏频道页面。</p><p lang="en">This games page was not found.</p></main>
}
