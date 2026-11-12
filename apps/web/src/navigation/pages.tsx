import { CompassIcon } from '@phosphor-icons/react/dist/ssr/Compass'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowRight'
import { ChatsCircleIcon } from '@phosphor-icons/react/dist/ssr/ChatsCircle'
import { ImageIcon } from '@phosphor-icons/react/dist/ssr/Image'
import { BookOpenIcon } from '@phosphor-icons/react/dist/ssr/BookOpen'
import { ToolboxIcon } from '@phosphor-icons/react/dist/ssr/Toolbox'
import { StorefrontIcon } from '@phosphor-icons/react/dist/ssr/Storefront'
import { NewspaperIcon } from '@phosphor-icons/react/dist/ssr/Newspaper'
import { GameControllerIcon } from '@phosphor-icons/react/dist/ssr/GameController'
import { CalendarDotsIcon } from '@phosphor-icons/react/dist/ssr/CalendarDots'
import { localizedPath } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { categoryCodes } from './contracts'
import { criteriaQuery, directoryHref, emptyCriteria, normalizeKeyword, type Criteria, type Home } from './data'
import { FeaturedCard, SiteCard, TrendingRow, type CardContext } from './cards'
import { categories, copy } from './messages'
import type { Page } from './load'
import styles from './sites.module.scss'

const icons = { community: ChatsCircleIcon, gallery: ImageIcon, literature: BookOpenIcon, resources: ToolboxIcon,
  marketplace: StorefrontIcon, knowledge: NewspaperIcon, gaming: GameControllerIcon, events: CalendarDotsIcon }

export function SitesFrame({ page, children }: { page: Page; children: React.ReactNode }) {
  const { locale, region, criteria } = page
  const m = copy[locale]
  const alternate = locale === 'zh' ? 'en' : 'zh'
  const home = localizedPath('/sites', locale, region)
  const dir = directoryHref(locale, region)
  return <div className={styles.page}>
    <header className={styles.masthead}>
      <a className={styles.wordmark} href={home}><CompassIcon size={22} aria-hidden="true" />GoFurry</a>
      <nav aria-label={locale === 'zh' ? '网站频道' : 'Sites navigation'}>
        <a href={home} aria-current={page.page === 'home' ? 'page' : undefined}>{m.sites}</a>
        <a href={dir} aria-current={page.page === 'directory' ? 'page' : undefined}>{m.directory}</a>
        <a href={localizedPath(page.page === 'home' ? '/sites' : '/sites/directory', alternate, region) + (criteria ? criteriaQuery(criteria) : '')}
          lang={alternate} hrefLang={alternate}>{m.language}</a>
      </nav>
    </header>
    <main className={styles.main}>{children}</main>
    <footer className={styles.footer}><span>GoFurry</span><a href={dir}>{m.directory}</a><a href={localizedPath('/start', locale, region)}>{m.start}</a></footer>
  </div>
}

export function SitesFailure({ page }: { page: Page }) {
  const m = copy[page.locale]
  return <section className={styles.message}>
    <h1>{page.state === 'invalid' ? m.invalid : m.unavailable}</h1>
    <a className={styles.action} href={page.state === 'invalid' ? page.path : page.path + (page.criteria ? criteriaQuery(page.criteria) : '')}>
      {page.state === 'invalid' ? m.reset : m.retry}
    </a>
  </section>
}

export function HomePage({ page, home }: { page: Page; home: Home }) {
  const { locale, region } = page
  const m = copy[locale]
  const ctx: CardContext = { locale, region, interactions: home.interactions, tags: home.tag_registry }
  const dir = directoryHref(locale, region)
  const to = (criteria: Partial<Criteria>) => directoryHref(locale, region, { ...emptyCriteria, ...criteria })
  const secondary = [home.featured.secondary_1, home.featured.secondary_2].filter(item => item !== null)
  return <SitesFrame page={page}>
    <section className={styles.intro}>
      <div><h1>{m.homeTitle}</h1><p>{m.intro}</p></div>
      <div className={styles.introActions}>
        <HomeSearch locale={locale} region={region} />
        <a href={dir} className={styles.textAction}>{m.browse}<ArrowRightIcon size={17} aria-hidden="true" /></a>
      </div>
    </section>
    {(home.featured.primary || secondary.length > 0) && <section aria-labelledby="sites-featured">
      <h2 id="sites-featured">{m.editorial}</h2>
      <div className={`${styles.features} ${home.featured.primary && secondary.length ? styles.splitFeatures : ''}`}>
        {home.featured.primary && <FeaturedCard site={home.featured.primary} ctx={ctx} primary />}
        {secondary.length > 0 && <div className={`${styles.featureSecondary} ${!home.featured.primary ? styles.standalone : ''}`}>
          {secondary.map(site => <FeaturedCard key={site.public_id} site={site} ctx={ctx} />)}
        </div>}
      </div>
    </section>}
    <div className={styles.channelColumns}>
      <section aria-labelledby="sites-recent"><h2 id="sites-recent">{m.recent}</h2>
        {home.recent.length ? <div className={styles.recentGrid}>{home.recent.map(site => <SiteCard key={site.public_id} site={site} ctx={ctx} recent />)}</div>
          : <p className={styles.emptySlice}>{m.noRecent}</p>}
      </section>
      <section aria-labelledby="sites-trending" className={styles.trending}>
        <h2 id="sites-trending">{m.trending}</h2>
        {home.heat.status !== 'ready' ? <p className={styles.emptySlice}>{m.heatWaiting}</p> : home.trending.length
          ? <ol className={styles.rankList}>{home.trending.map(site => <TrendingRow key={site.public_id} site={site} ctx={ctx} />)}</ol>
          : <p className={styles.emptySlice}>{m.noTrend}</p>}
        <a className={styles.textAction} href={to({ sort: 'heat_7d' })}>{m.ranking}<ArrowRightIcon size={15} aria-hidden="true" /></a>
      </section>
    </div>
    <section aria-labelledby="sites-topics">
      <h2 id="sites-topics">{m.topics}</h2>
      <ul className={styles.topicGrid}>{categoryCodes.map(code => {
        const item = categories[code], Icon = icons[code]
        const count = home.categories.find(term => term.code === code)?.count ?? 0
        return <li key={code}><a href={to({ category: [code] })}>
          <Icon size={23} aria-hidden="true" /><span><strong>{item[locale]}</strong><span>{item[locale === 'zh' ? 'zhCaption' : 'enCaption']}</span></span>
          <small aria-label={m.results(count)}>{count}</small>
        </a></li>
      })}</ul>
    </section>
    {home.random && <section aria-labelledby="sites-random" className={styles.random}>
      <div><h2 id="sites-random">{m.random}</h2><a className={styles.textAction} href={dir}>{m.browse}<ArrowRightIcon size={17} aria-hidden="true" /></a></div>
      <SiteCard site={home.random} ctx={ctx} />
    </section>}
  </SitesFrame>
}

function HomeSearch({ locale, region }: { locale: Locale; region: Region }) {
  const m = copy[locale]
  return <form className={styles.search} action={directoryHref(locale, region)} role="search" onSubmit={event => {
    event.preventDefault()
    const q = normalizeKeyword(String(new FormData(event.currentTarget).get('q') ?? ''))
    window.location.assign(directoryHref(locale, region, { ...emptyCriteria, q }))
  }}>
    <MagnifyingGlassIcon size={21} aria-hidden="true" />
    <input name="q" type="search" aria-label={m.search} placeholder={m.placeholder} autoComplete="off" />
    <button className={styles.action} type="submit">{m.search}</button>
  </form>
}
