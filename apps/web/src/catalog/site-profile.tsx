import { Link } from '@tanstack/react-router'
import { ArrowDownIcon } from '@phosphor-icons/react/dist/ssr/ArrowDown'
import { ArrowUpRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowUpRight'
import { BookOpenIcon } from '@phosphor-icons/react/dist/ssr/BookOpen'
import { CaretRightIcon } from '@phosphor-icons/react/dist/ssr/CaretRight'
import { SiteCard, SiteLogo, languageLabel } from '../navigation/cards'
import { criteriaParams, directoryPath, emptyCriteria, type Criteria } from '../navigation/data'
import { SitesFrame } from '../navigation/frame'
import { categories, copy as navigationCopy } from '../navigation/messages'
import { SiteDwell, siteOutbound } from '../navigation/site-interactions'
import type { Locale } from '../preferences'
import { yearbookPath } from '../yearbook/path'
import type { DetailPageData } from './detail'
import type { Technology } from './site-profile-data'
import styles from './site-profile.module.scss'

const copy = {
  zh: {
    breadcrumb: '当前位置', profile: '查看站点资料', about: '关于网站', information: '站点信息', primary: '主分类', secondary: '补充分类',
    primaryAddress: '主要地址', alternates: '备用地址', attention: '近期关注', views: '有效浏览', clicks: '外链点击',
    related: '相关网站', technology: '技术概览', recorded: 'Harvest 上次确认', historical: '历史观察', observed: '首次记录',
    status: '记录的 HTTP 响应', tls: '当时通过 TLS 验证', fingerprint: '被动识别',
    high: '高置信度', medium: '中等置信度', yearbooks: '已发布年鉴', year: (year: number) => `${year} 年站点年鉴`,
    untranslated: '此页面暂无中文译文，以下为现有资料。', unavailable: '站点资料暂时无法加载。',
  },
  en: {
    breadcrumb: 'Breadcrumb', profile: 'Explore this profile', about: 'About this site', information: 'Site information', primary: 'Primary category', secondary: 'Additional categories',
    primaryAddress: 'Primary address', alternates: 'Alternate addresses', attention: 'Recent activity', views: 'Qualified detail views', clicks: 'Outbound clicks',
    related: 'Related sites', technology: 'Technology overview', recorded: 'Last confirmed by Harvest', historical: 'Last observed', observed: 'First recorded',
    status: 'Recorded HTTP response', tls: 'TLS verified at observation', fingerprint: 'Passively identified',
    high: 'High confidence', medium: 'Medium confidence', yearbooks: 'Published yearbooks', year: (year: number) => `${year} Site Yearbook`,
    untranslated: 'An English translation is unavailable. The available profile is shown below.', unavailable: 'This site profile is temporarily unavailable.',
  },
}

const technologyNames: Record<string, string> = {
  wordpress: 'WordPress', ghost: 'Ghost', drupal: 'Drupal', discourse: 'Discourse', xenforo: 'XenForo', mediawiki: 'MediaWiki',
  nextjs: 'Next.js', nuxt: 'Nuxt', astro: 'Astro', vercel: 'Vercel', netlify: 'Netlify', 'github-pages': 'GitHub Pages',
  cloudflare: 'Cloudflare', fastly: 'Fastly', cloudfront: 'Amazon CloudFront', nginx: 'NGINX', apache: 'Apache', caddy: 'Caddy',
}

function dateLabel(value: string, locale: Locale) {
  return new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en-GB', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(value))
}

function Fact({ fact, locale }: { fact: Technology; locale: Locale }) {
  const m = copy[locale]
  const name = fact.kind === 'http' ? `${m.status} ${fact.http_status}` :
    fact.kind === 'tls' ? m.tls : technologyNames[fact.technology ?? '']
  if (!name) return null
  return <li>
    <strong>{name}</strong>
    {fact.confidence && <small>{m.fingerprint} · {m[fact.confidence]}</small>}
    <small>{fact.source}</small>
    <small>{fact.observation_state === 'last_observed' ? m.historical : m.recorded} <time dateTime={fact.last_observed_at}>{dateLabel(fact.last_observed_at, locale)}</time></small>
    {fact.first_observed_at && <small>{m.observed} <time dateTime={fact.first_observed_at}>{dateLabel(fact.first_observed_at, locale)}</time></small>}
  </li>
}

function host(address: string) { return new URL(address).host }

export function SiteProfile({ data }: { data: DetailPageData }) {
  const { locale, region } = data
  const m = copy[locale], channel = navigationCopy[locale], directory = directoryPath(locale, region)
  const frame = { page: 'detail', locale, region } as const
  if (data.state === 'unavailable' || data.detail.kind !== 'sites') return <SitesFrame page={frame}>
    <section className={styles.failure}><h1>{m.unavailable}</h1><a href="">{channel.retry}</a></section>
  </SitesFrame>

  const { detail, alternatives } = data
  const translation = detail.localizations.find(item => item.locale === locale)
  const display = translation ?? detail.localizations[0]!
  const primary = detail.addresses.find(address => address.kind === 'primary')
  const alternates = detail.addresses.filter(address => address.kind === 'alternate')
  const visit = detail.interactions ? (event: Parameters<typeof siteOutbound>[1]) => siteOutbound(detail.public_id, event) : undefined
  const to = (filter: Partial<Criteria>) => criteriaParams({ ...emptyCriteria, ...filter })
  const hasInformation = detail.reviewed
  const hasMain = Boolean(display.body || hasInformation || detail.stats)
  const hasAside = Boolean(detail.related.length || detail.technology.length || detail.yearbooks.length)
  const alternatePath = alternatives.find(item => item.locale !== locale)?.path
  return <SitesFrame page={{ ...frame, alternatePath }}>
    <article className={styles.profile}>
      {detail.interactions && <SiteDwell publicID={detail.public_id} />}
      <nav className={styles.breadcrumb} aria-label={m.breadcrumb}>
        <Link to={directory}>{channel.directory}</Link>
        <CaretRightIcon size={14} aria-hidden="true" />
        {detail.primaryCategory ? <Link to={directory} search={to({ category: [detail.primaryCategory] })}>{categories[detail.primaryCategory][locale]}</Link> :
          <span lang={display.locale}>{display.name}</span>}
      </nav>
      <header className={styles.hero}>
        <SiteLogo className={styles.mark} src={detail.logo} name={display.name} locale={display.locale} />
        <div className={styles.heroCopy}>
          <h1 lang={display.locale}>{display.name}</h1>
          {primary && <p className={styles.domain}>{host(primary.url)}</p>}
          {!translation && <p className={styles.translation}>{m.untranslated}</p>}
          {display.summary && <p className={styles.intro} lang={display.locale}>{display.summary}</p>}
          {detail.reviewed && <ul className={styles.chips}>
            {detail.primaryCategory ? <li><Link data-primary-category to={directory} search={to({ category: [detail.primaryCategory] })}>{categories[detail.primaryCategory][locale]}</Link></li>
              : <li>{channel.uncategorized}</li>}
            {detail.secondaryCategories.map(category => <li key={category}>
              <Link to={directory} search={to({ category: [category] })}>{categories[category][locale]}</Link></li>)}
            {detail.tags.slice(0, 3).map(tag => <li key={tag.code}><Link to={directory} search={to({ tags: [tag.code] })}>{tag[locale]}</Link></li>)}
            {detail.languages.slice(0, 3).map(code => <li key={code}><Link to={directory} search={to({ language: [code] })}>{languageLabel(code, locale)}</Link></li>)}
          </ul>}
          {(primary || hasMain || hasAside) && <div className={styles.actions}>
            {primary && <a className={styles.visit} href={primary.url} target="_blank" rel="noopener noreferrer" aria-description={channel.newTab}
              onClick={visit} onAuxClick={visit}>{channel.visit}<ArrowUpRightIcon size={17} aria-hidden="true" /></a>}
            {(hasMain || hasAside) && <a className={styles.jump} href="#site-profile">{m.profile}<ArrowDownIcon size={16} aria-hidden="true" /></a>}
          </div>}
        </div>
      </header>
      {(hasMain || hasAside) && <div className={styles.columns} data-two-columns={hasMain && hasAside || undefined} id="site-profile" tabIndex={-1}>
        {hasMain && <div className={styles.sections}>
          {display.body && <section aria-labelledby="site-about" lang={display.locale}>
            <h2 id="site-about" lang={locale}>{m.about}</h2>
            <div className={styles.prose}>{display.body.split(/\n[ \n]*\n/).map((paragraph, index) => <p key={index}>{paragraph}</p>)}</div>
          </section>}
          {hasInformation && <section aria-labelledby="site-information">
            <h2 id="site-information">{m.information}</h2>
            <dl className={styles.information}>
              <div><dt>{m.primary}</dt><dd>{detail.primaryCategory ? <Link to={directory} search={to({ category: [detail.primaryCategory] })}>
                {categories[detail.primaryCategory][locale]}</Link> : channel.uncategorized}</dd></div>
              {detail.secondaryCategories.length > 0 && <div><dt>{m.secondary}</dt><dd>{detail.secondaryCategories.map(category =>
                <Link key={category} to={directory} search={to({ category: [category] })}>{categories[category][locale]}</Link>)}</dd></div>}
              {detail.tags.length > 0 && <div><dt>{channel.tags}</dt><dd>{detail.tags.map(tag =>
                <Link key={tag.code} to={directory} search={to({ tags: [tag.code] })}>{tag[locale]}</Link>)}</dd></div>}
              {detail.languages.length > 0 && <div><dt>{channel.languages}</dt><dd>{detail.languages.map(code =>
                <Link key={code} to={directory} search={to({ language: [code] })}>{languageLabel(code, locale)}</Link>)}</dd></div>}
              {primary && <div><dt>{m.primaryAddress}</dt><dd><a href={primary.url} target="_blank" rel="noopener noreferrer" onClick={visit} onAuxClick={visit}>
                {host(primary.url)} <ArrowUpRightIcon size={13} aria-hidden="true" /></a></dd></div>}
            </dl>
            {alternates.length > 0 && <details className={styles.alternates}>
              <summary>{m.alternates} ({alternates.length})</summary>
              <ul>{alternates.map(address => <li key={address.url}><a href={address.url} target="_blank" rel="noopener noreferrer" onClick={visit} onAuxClick={visit}>
                {host(address.url)} <ArrowUpRightIcon size={13} aria-hidden="true" /></a></li>)}</ul>
            </details>}
          </section>}
          {detail.stats && <section aria-labelledby="site-attention">
            <h2 id="site-attention">{m.attention}</h2>
            <p className={styles.window}><time dateTime={detail.stats.window_start}>{dateLabel(detail.stats.window_start, locale)}</time>
              {' – '}<time dateTime={detail.stats.window_end}>{dateLabel(detail.stats.window_end, locale)}</time> · UTC</p>
            <dl className={styles.counts}>
              {detail.stats.detail_views && <div><dt>{m.views}</dt><dd>{detail.stats.detail_views.toLocaleString(locale)}</dd></div>}
              {detail.stats.outbound_clicks && <div><dt>{m.clicks}</dt><dd>{detail.stats.outbound_clicks.toLocaleString(locale)}</dd></div>}
            </dl>
          </section>}
        </div>}
        {hasAside && <aside className={styles.sections}>
          {detail.related.length > 0 && <section aria-labelledby="site-related">
            <h2 id="site-related">{m.related}</h2>
            <div>{detail.related.map(site => <SiteCard key={site.public_id} site={site} ctx={{ locale, region, interactions: false }} compact />)}</div>
          </section>}
          {detail.technology.length > 0 && <section aria-labelledby="site-technology">
            <h2 id="site-technology">{m.technology}</h2>
            <ul className={styles.facts}>{detail.technology.map(fact => <Fact key={fact.technology ?? fact.kind} fact={fact} locale={locale} />)}</ul>
          </section>}
          {detail.yearbooks.length > 0 && <section aria-labelledby="site-yearbooks">
            <h2 id="site-yearbooks">{m.yearbooks}</h2>
            <ul className={styles.years}>{detail.yearbooks.map(year => <li key={year}>
              <Link to={yearbookPath(detail.public_id, detail.slug, year, locale, region)}>
                <BookOpenIcon size={19} aria-hidden="true" />{m.year(year)}<CaretRightIcon size={16} aria-hidden="true" />
              </Link></li>)}</ul>
          </section>}
        </aside>}
      </div>}
    </article>
  </SitesFrame>
}
