import { Link } from '@tanstack/react-router'
import { CompassIcon } from '@phosphor-icons/react/dist/ssr/Compass'
import { localizedPath } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { criteriaParams, directoryHref } from './data'
import type { Page } from './load'
import { copy } from './messages'
import styles from './sites.module.scss'

type FramePage = Page | { page: 'detail'; locale: Locale; region: Region; alternatePath?: string }

export function SitesFrame({ page, children }: { page: FramePage; children: React.ReactNode }) {
  const { locale, region } = page
  const m = copy[locale]
  const alternate = locale === 'zh' ? 'en' : 'zh'
  const home = localizedPath('/sites', locale, region)
  const dir = directoryHref(locale, region)
  const alternatePath = page.page === 'detail' ? page.alternatePath :
    localizedPath(page.page === 'home' ? '/sites' : '/sites/directory', alternate, region)
  const alternateSearch = page.page !== 'detail' && page.criteria ? criteriaParams(page.criteria) : undefined
  return <div className={styles.page}>
    <header className={styles.masthead}>
      <a className={styles.wordmark} href={home}><CompassIcon size={22} aria-hidden="true" />GoFurry</a>
      <nav aria-label={locale === 'zh' ? '网站频道' : 'Sites navigation'}>
        <Link to={home} activeOptions={{ exact: true, includeSearch: false }}>{m.sites}</Link>
        <Link to={dir} activeOptions={{ exact: true, includeSearch: false }}>{m.directory}</Link>
        {alternatePath && <Link to={alternatePath} search={alternateSearch} lang={alternate} hrefLang={alternate} preload={false}>{m.language}</Link>}
      </nav>
    </header>
    <main className={styles.main}>{children}</main>
    <footer className={styles.footer}><span>GoFurry</span><Link to={dir}>{m.directory}</Link><Link to={localizedPath('/start', locale, region)}>{m.start}</Link></footer>
  </div>
}
