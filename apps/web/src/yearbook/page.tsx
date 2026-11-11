import { notFound, redirect } from '@tanstack/react-router'
import { isPublicID } from '../catalog/detail-server'
import { entityPath } from '../catalog/public-path'
import { defaultLocale } from '../locale-path'
import type { Locale, Region } from '../preferences'
import { yearbookPath } from './path'
import { readYearbook, validYear, type Coverage, type Fact, type Snapshot } from './read'
import styles from './yearbook.module.scss'

type Params = { language?: string; publicId: string; slug: string; year: string }
type Page = { state: 'unavailable'; locale: Locale } | {
  state: 'published'; snapshot: Snapshot; locale: Locale; region: Region; canonical?: string;
  alternatives: { locale: Locale; path: string; href: string }[]
}

export async function loadYearbook(params: Params, pathname: string, region: Region): Promise<Page> {
  const { language, publicId, year: rawYear } = params
  if (!/^\/(?:(?:en|zh)\/)?sites\/[^/]+\/[^/]+\/yearbook\/[0-9]{4}\/?$/.test(pathname) ||
    (language !== undefined && language !== 'en' && language !== 'zh') || !isPublicID(publicId) ||
    !/^[0-9]{4}$/.test(rawYear) || !validYear(Number(rawYear))) throw notFound()
  const locale = language ?? defaultLocale(region)
  const year = Number(rawYear)
  const result = await readYearbook({ data: { id: publicId, year, region } })
  if (result.state === 'missing') throw notFound()
  if (result.state === 'unavailable') return { state: 'unavailable', locale }
  const { snapshot, origin } = result
  const canonicalPath = yearbookPath(publicId, snapshot.slug, year, locale, region)
  if (pathname !== canonicalPath) throw redirect({ href: canonicalPath, statusCode: 301, headers: { 'Cache-Control': 'no-store' } })
  const alternatives = snapshot.document.names.map(name => {
    const path = yearbookPath(publicId, snapshot.slug, year, name.locale, region)
    return { locale: name.locale, path, href: new URL(path, origin).href }
  })
  return { state: 'published', snapshot, locale, region, alternatives, canonical: alternatives.find(a => a.locale === locale)?.href }
}

export function yearbookHeaders(data?: Page): Record<string, string> {
  return data?.state === 'published' && data.canonical
    ? { 'Cache-Control': 'public, max-age=0, must-revalidate' }
    : { 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }
}

const copy = {
  en: {
    title: (name: string, year: number) => `${name} · ${year} Site Yearbook`,
    description: (name: string, year: number) => `Confirmed observations of ${name} in ${year}, with coverage, recorded changes and their evidence.`,
    yearbook: (year: number) => `${year} Site Yearbook`, back: 'Site details', unavailable: 'The yearbook is temporarily unavailable.',
    missing: 'Yearbook not found.', untranslated: 'The archived site name is unavailable in English.',
    nameAtGeneration: 'Name recorded when this revision was generated', currentName: 'Current name', coverage: 'Evidence coverage',
    yearPeriod: 'Calendar year', recorded: 'Recorded dates', utc: 'All dates are UTC.', days: 'Confirmed days', unknown: 'Unknown days',
    dayCount: (n: number, total: number) => `${n} of ${total}`, gaps: (n: number) => `Unknown date ranges (${n})`,
    unknownMeaning: 'Unknown means there is no confirmation for that date.', sources: 'Sources and capabilities',
    scopeCount: (n: number) => `${n} ${n === 1 ? 'scope' : 'scopes'}`, partial: 'The recorded period covers only part of the year.',
    timeline: 'Recorded changes', noChanges: 'No material changes were verified in the recorded evidence.',
    end: 'Last verified observations at year end', noEnd: 'No qualifying fact was verified for the bindings active at year end.',
    about: 'About these observations', aboutText: 'Dates show when evidence was detected or last observed. A change in technology signals, DNS answers or certificates can depend on the collection region and detector version. These samples do not measure uptime.',
    evidence: 'Evidence', provenance: 'Source and collection region', ruleset: 'Rule version', ref: 'Evidence reference',
    detected: 'First detected', observed: 'Last observed', prior: 'Known before this year', more: (n: number) => `${n} additional records are outside this summary.`,
    revision: (n: number) => `Revision ${n}`, noLocale: 'en',
    kinds: { technology: 'Technology signal', domain: 'Primary host observed', http: 'HTTP destination observed', dns: 'DNS answer fingerprint', tls: 'TLS certificate fingerprint', rdap: 'Registration record observed' },
    confidence: { high: 'High confidence', medium: 'Medium confidence', low: 'Low confidence', verified: 'Verified observation' },
  },
  zh: {
    title: (name: string, year: number) => `${name} · ${year} 年站点年鉴`,
    description: (name: string, year: number) => `${name} 在 ${year} 年有证据支持的观测覆盖、变化记录与出处。`,
    yearbook: (year: number) => `${year} 年站点年鉴`, back: '站点详情', unavailable: '年鉴暂时无法加载。',
    missing: '找不到此年鉴。', untranslated: '归档站点名称暂无中文译名。',
    nameAtGeneration: '生成此修订时记录的名称', currentName: '当前名称', coverage: '证据覆盖',
    yearPeriod: '统计年度', recorded: '有记录的日期', utc: '日期均以 UTC 计。', days: '确认天数', unknown: '未知日期',
    dayCount: (n: number, total: number) => `${n} / ${total} 天`, gaps: (n: number) => `未知日期区间（${n}）`,
    unknownMeaning: '未知表示该日缺少可确认的证据。', sources: '来源与采集能力', scopeCount: (n: number) => `${n} 个范围`,
    partial: '记录只覆盖了该年度的一部分。', timeline: '记录中的变化', noChanges: '现有证据未确认需要列入时间线的变化。',
    end: '年末最近可验证观测', noEnd: '年末仍有效的绑定暂无符合条件的可验证事实。',
    about: '关于这些观测', aboutText: '日期表示证据首次被检测或最近被观测的时间。技术信号、DNS 应答及证书的变化可能受采集区域或规则版本影响。这些样本不用于计算可用率。',
    evidence: '证据', provenance: '来源与采集区域', ruleset: '规则版本', ref: '证据引用',
    detected: '首次检测', observed: '最近观测', prior: '本年度开始前已知', more: (n: number) => `另有 ${n} 条记录超出本摘要范围。`,
    revision: (n: number) => `修订 ${n}`, noLocale: 'zh',
    kinds: { technology: '技术信号', domain: '观测到的主域名', http: '观测到的 HTTP 目的域名', dns: 'DNS 应答指纹', tls: 'TLS 证书指纹', rdap: '观测到的注册记录' },
    confidence: { high: '高置信度', medium: '中置信度', low: '低置信度', verified: '已验证的观测' },
  },
}

export function yearbookHead(data?: Page) {
  if (data?.state !== 'published') return { meta: [{ name: 'robots', content: 'noindex' }] }
  const { snapshot, locale } = data
  const name = snapshot.document.names.find(n => n.locale === locale)
  return {
    meta: name ? [{ title: copy[locale].title(name.name, snapshot.year) },
      { name: 'description', content: copy[locale].description(name.name, snapshot.year) }]
      : [{ title: copy[locale].yearbook(snapshot.year) }, { name: 'robots', content: 'noindex' }],
    links: [
      ...(data.canonical ? [{ rel: 'canonical', href: data.canonical }] : []),
      ...data.alternatives.map(a => ({ rel: 'alternate', hrefLang: a.locale, href: a.href })),
    ],
  }
}

export function MissingYearbook() {
  return <main className={styles.sheet}><h1>404</h1><p lang="zh">{copy.zh.missing}</p><p lang="en">{copy.en.missing}</p></main>
}

function MonthGrid({ coverage, locale, year }: { coverage: Coverage; locale: Locale; year: number }) {
  const formatter = new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en', { month: 'short', timeZone: 'UTC' })
  return <div className={styles.months} aria-hidden="true">{coverage.months.map((mask, month) => {
    const days = new Date(Date.UTC(year, month + 1, 0)).getUTCDate()
    return <div className={styles.month} key={month}>
      <span>{formatter.format(new Date(Date.UTC(year, month)))}</span>
      <div className={styles.days}>{Array.from({ length: days }, (_, d) =>
        <span className={mask & (1 << d) ? styles.confirmed : styles.unknown} key={d} />)}</div>
    </div>
  })}</div>
}

function CoverageSection({ data, locale, year }: { data: Coverage; locale: Locale; year: number }) {
  const m = copy[locale]
  return <section className={styles.section} aria-labelledby="coverage-heading">
    <h2 id="coverage-heading">{m.coverage}</h2>
    <dl className={styles.coverage}>
      <div><dt>{m.days}</dt><dd><strong>{m.dayCount(data.days, data.period_days)}</strong></dd></div>
      <div><dt>{m.recorded}</dt><dd>{data.recorded_from} – {data.recorded_to}</dd></div>
      <div><dt>{m.yearPeriod}</dt><dd>{year}-01-01 – {year}-12-31</dd></div>
    </dl>
    <MonthGrid coverage={data} locale={locale} year={year} />
    <p className={styles.legend}><span className={styles.confirmed} aria-hidden="true" /> {m.days}<span className={styles.unknown} aria-hidden="true" /> {m.unknown}</p>
    {data.partial_year && <p>{m.partial}</p>}
    <p>{m.unknownMeaning} {m.utc}</p>
    {data.unknown.length > 0 && <details><summary>{m.gaps(data.unknown.length)}</summary>
      <ul className={styles.ranges}>{data.unknown.map(r => <li key={r.from}>{r.from}{r.to !== r.from && ` – ${r.to}`}</li>)}</ul>
    </details>}
    <details><summary>{m.sources}</summary><ul>{data.sources.map(source =>
      <li key={source.host + source.capability}>{source.host} · {source.capability} · {m.scopeCount(source.scopes)}</li>)}</ul>
      {data.sources_omitted > 0 && <p>{m.more(data.sources_omitted)}</p>}
    </details>
  </section>
}

function Evidence({ fact, locale, timeline }: { fact: Fact; locale: Locale; timeline: boolean }) {
  const m = copy[locale]
  const timestamp = timeline ? fact.detected : fact.observed
  return <li className={styles.fact}>
    <time dateTime={timestamp}>{timestamp.slice(0, 10)}</time>
    <div>
      <h3>{m.kinds[fact.kind]}{fact.dimension && ` · ${fact.dimension}`}</h3>
      <p>{fact.previous && <>{fact.previous} <span aria-hidden="true">→</span>{' '}</>}{fact.value}</p>
      {fact.prior_known && <p>{m.prior}</p>}
      <details>
        <summary>{m.evidence} · {m.confidence[fact.confidence]}</summary>
        <dl className={styles.provenance}>
          <div><dt>{m.provenance}</dt><dd>{fact.host} · {fact.capability} · {fact.vantage}</dd></div>
          <div><dt>{m.detected}</dt><dd><time dateTime={fact.detected}>{fact.detected}</time></dd></div>
          <div><dt>{m.observed}</dt><dd><time dateTime={fact.observed}>{fact.observed}</time></dd></div>
          <div><dt>{m.ruleset}</dt><dd>{fact.ruleset}</dd></div>
          <div><dt>{m.evidence}</dt><dd>{[fact.evidence, ...(fact.signals ?? [])].join(' · ')}</dd></div>
          <div><dt>{m.ref}</dt><dd><code>{fact.reference}</code></dd></div>
        </dl>
      </details>
    </div>
  </li>
}

export function YearbookView({ data }: { data: Page }) {
  const m = copy[data.locale]
  if (data.state === 'unavailable') return <main className={styles.sheet}><h1>{m.unavailable}</h1></main>
  const { snapshot: y, locale, region, alternatives } = data
  const d = y.document
  const localized = d.names.find(n => n.locale === locale)
  const display = localized ?? d.names[0]
  const current = y.current_names.find(n => n.locale === locale)
  return <main className={styles.sheet}>
    <header className={styles.header}>
      <nav className={styles.nav}>
        {y.site_available && <a href={entityPath('sites', y.public_id, y.slug, locale, region)}>{m.back}</a>}
        {alternatives.filter(a => a.locale !== locale).map(a =>
          <a key={a.locale} lang={a.locale} hrefLang={a.locale} href={a.path}>{a.locale === 'zh' ? '中文' : 'English'}</a>)}
      </nav>
      <h1>{m.yearbook(y.year)}</h1>
      <p className={styles.name} lang={display.locale}>{display.name}</p>
      <p>{m.nameAtGeneration}</p>
      {!localized && <p>{m.untranslated}</p>}
      {current && current.name !== display.name && <p>{m.currentName} · {current.name}</p>}
      <p>{m.revision(y.revision)} · <time dateTime={y.published_at}>{y.published_at.slice(0, 10)}</time></p>
    </header>
    <CoverageSection data={d.coverage} locale={locale} year={y.year} />
    <section className={styles.section}>
      <h2>{m.timeline}</h2>
      {d.timeline.length ? <ol className={styles.facts}>{d.timeline.map(f => <Evidence key={f.reference + f.kind} fact={f} locale={locale} timeline />)}</ol>
        : <p>{m.noChanges}</p>}
      {d.timeline_omitted > 0 && <p>{m.more(d.timeline_omitted)}</p>}
    </section>
    <section className={styles.section}>
      <h2>{m.end}</h2>
      {d.year_end.length ? <ul className={styles.facts}>{d.year_end.map(f => <Evidence key={f.reference} fact={f} locale={locale} timeline={false} />)}</ul>
        : <p>{m.noEnd}</p>}
      {d.year_end_omitted > 0 && <p>{m.more(d.year_end_omitted)}</p>}
    </section>
    <footer className={styles.section}><h2>{m.about}</h2><p>{m.aboutText}</p></footer>
  </main>
}
