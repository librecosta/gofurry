import { useEffect, useRef, useState } from 'react'
import type { Locale } from '../preferences'
import { marketLabel, profileCopy } from './profile-copy'
import { countries, money, storyURL, utcDate, type Country, type GameSource, type Insights, type PriceDay } from './profile-data'
import { readGameExtra, type ProfilePage } from './profile-load'
import styles from './profile.module.scss'

type Point = { day: string; value: number; second?: number; label: string }

function Chart({ data, points, locale, format, label, second }: {
  data: Insights; points: Point[]; locale: Locale; format: (value: number) => string; label: string; second?: string
}) {
  const m = profileCopy[locale]
  const [active, setActive] = useState(0)
  const [focused, setFocused] = useState(false)
  if (points.length < 2) return <p className={styles.muted}>{points.length ? m.insufficient + ' ' + points[0].label : m.none}</p>
  const since = Date.parse(data.from_day)
  const topValue = Math.max(...points.map(p => Math.max(p.value, p.second ?? 0)))
  const max = topValue || 1
  const day = (point: Point) => (Date.parse(point.day) - since) / 86400000
  const x = (point: Point) => 16 + day(point) / Math.max(1, data.range - 1) * 464
  const y = (n: number) => 140 - n / max * 120
  const point = points[Math.min(active, points.length - 1)]
  return <div className={styles.chart}>
    <div className={styles.axisValues} aria-hidden="true"><span>{format(topValue)}</span><span>{format(0)}</span></div>
    <svg viewBox="0 0 496 156" role="img" aria-label={label} onPointerMove={event => {
      const bounds = event.currentTarget.getBoundingClientRect()
      const nearest = (event.clientX - bounds.left) * 496 / bounds.width
      setActive(points.reduce((best, candidate, i) => Math.abs(x(candidate) - nearest) < Math.abs(x(points[best]) - nearest) ? i : best, 0))
      setFocused(true)
    }} onPointerLeave={() => setFocused(false)} onPointerDown={event => { event.currentTarget.parentElement?.querySelector('input')?.focus() }}>
      <path className={styles.gridLine} d="M16 20H480 M16 80H480 M16 140H480" />
      {points.map((p, i) => {
        const previous = points[i - 1]
        return <g key={p.day}>
          {previous && day(p) - day(previous) === 1 && <>
            <path className={styles.series} d={["M",x(previous),y(previous.value),"L",x(p),y(p.value)].join(" ")} />
            {p.second !== undefined && previous.second !== undefined &&
              <path className={styles.secondSeries} d={["M",x(previous),y(previous.second),"L",x(p),y(p.second)].join(" ")} />}
          </>}
          <circle className={styles.seriesPoint} cx={x(p)} cy={y(p.value)} r={focused && p === point ? 5 : 2.7} />
          {p.second !== undefined && <circle className={styles.secondPoint} cx={x(p)} cy={y(p.second)} r="2.3" />}
        </g>
      })}
    </svg>
    <span className={styles.chartDates}>{utcDate(data.from_day, locale)} <span>UTC</span> {utcDate(data.through_day, locale)}</span>
    <label className={styles.chartScrubber}>{label}
      <input type="range" min="0" max={points.length - 1} value={Math.min(active, points.length - 1)}
        aria-valuetext={point.label} onFocus={() => setFocused(true)} onBlur={() => setFocused(false)}
        onChange={event => setActive(Number(event.target.value))} />
    </label>
    <output className={styles.chartValue}>{point.label}</output>
    {second && <p className={styles.legend}><span>{m.average}</span><span>{second}</span></p>}
  </div>
}

function priceLabel(point: PriceDay, data: Insights, locale: Locale) {
  const m = profileCopy[locale]
  if (point.state === 'free') return m.free
  const price = point.final === null ? '' : money(point.final, data.currency, locale)
  return point.state === 'zero_price' ? m.zeroPrice + ' · ' + price : price
}

function History({ data, locale }: { data: Insights; locale: Locale }) {
  const m = profileCopy[locale]
  const n = new Intl.NumberFormat(locale === 'zh' ? 'zh-CN' : 'en', { maximumFractionDigits: 2 })
  const players = data.player_days.map(p => ({ day: p.day, value: p.average, second: p.maximum,
    label: [utcDate(p.observed_at, locale, true), m.average + ' ' + n.format(p.average),
      m.maximum + ' ' + n.format(p.maximum), p.samples + ' ' + m.samples].join(' · ') }))
  const quotes = [...data.price_days].reverse()
  const prices = quotes.flatMap(p => p.final === null ? [] : [{ day: p.day, value: p.final,
    label: utcDate(p.observed_at, locale, true) + ' · ' + priceLabel(p, data, locale) }])
  const timeline = [
    ...data.price_changes.map(p => ({ time: p.observed_at, label: m.priceChange, text: priceLabel(p, data, locale), key: p.observation_id, url: undefined })),
    ...data.news.map(p => ({ time: p.published_at, label: m.officialStory, text: p.title, key: p.gid, url: storyURL(p) })),
  ].sort((a, b) => Date.parse(b.time) - Date.parse(a.time))
  return <>
    <article className={styles.historyPanel}>
      <h3>{m.history} · {marketLabel[locale][data.store_country]} · {data.currency}</h3>
      <p className={styles.muted}>{m.priceNote}</p>
      <p className={styles.coverage}>{m.coverage} {quotes.length} / {data.range}{data.price_locale ? ' · ' + m.storeLanguage + ' ' + data.price_locale : ''}</p>
      <Chart key={data.app_id + data.store_country + data.range} data={data} points={prices} locale={locale} format={amount => money(amount, data.currency, locale)} label={m.history} />
      {quotes.length > 0 && <details className={styles.observations}><summary>{m.values}</summary>
        <ul>{quotes.map(p => <li key={p.observation_id}><time dateTime={p.observed_at}>{utcDate(p.observed_at, locale, true)}</time> · {priceLabel(p, data, locale)}
          {p.discount_percent ? ' · −' + p.discount_percent + '%' : ''}</li>)}</ul></details>}
    </article>
    <article className={styles.historyPanel}>
      <h3>{m.trend}{data.app_id ? ' · ' + data.app_id : ''}</h3>
      <p className={styles.muted}>{m.onlySteam}</p>
      <p className={styles.coverage}>{m.coverage} {players.length} / {data.range}</p>
      <Chart key={data.app_id + ':' + data.range} data={data} points={players} locale={locale} format={value => n.format(value)} label={m.trend} second={m.maximum} />
      {players.length > 0 && <details className={styles.observations}><summary>{m.values}</summary>
        <ul>{players.map(p => <li key={p.day}>{p.label}</li>)}</ul></details>}
    </article>
    <article className={styles.historyPanel}>
      <h3>{m.timeline}</h3>
      {timeline.length ? <ol className={styles.timeline}>{timeline.map(item => <li key={item.label + item.key}>
        <time dateTime={item.time}>{utcDate(item.time, locale, true)}</time><span>{item.label}</span>
        {item.url ? <a href={item.url} rel="noopener noreferrer" referrerPolicy="no-referrer">{item.text}</a> : <strong>{item.text}</strong>}
      </li>)}</ol> : <p className={styles.muted}>{m.noTimeline}</p>}
    </article>
  </>
}

export default function ProfileInsights({ page, sources }: { page: ProfilePage; sources: GameSource[] }) {
  const { id, locale, region } = page
  const m = profileCopy[locale]
  const [app, setApp] = useState(sources[0]?.external_key)
  const [country, setCountry] = useState<Country>(region === 'CN' ? 'cn' : 'us')
  const [range, setRange] = useState(30)
  const [revision, retry] = useState(0)
  const [result, setResult] = useState<{ key: string; data?: Insights }>()
  const cache = useRef(new Map<string, Insights>())
  const key = [id, app, country, range].join('/')
  const data = result?.key === key ? result.data : undefined
  useEffect(() => {
    if (!app) return
    const current = cache.current.get(key)
    if (current) { setResult({ key, data: current }); return }
    const controller = new AbortController()
    readGameExtra({ data: { id, locale, region, app, country, range, view: 'insights' }, signal: controller.signal }).then(reply => {
      if (controller.signal.aborted) return
      const data = reply.state === 'ready' && 'price_days' in reply.data ? reply.data : undefined
      if (data) cache.current.set(key, data)
      setResult({ key, data })
    }, () => { if (!controller.signal.aborted) setResult({ key }) })
    return () => controller.abort()
  }, [key, id, locale, region, app, country, range, revision])
  return <div className={styles.insights}>
    <div className={styles.filters}>
      <label className={styles.control}>{m.app}
        <select value={app} onChange={event => { setApp(event.target.value); setRange(30) }}>
          {sources.map(source => <option key={source.external_key} value={source.external_key}>{source.external_key}</option>)}
        </select>
      </label>
      <label className={styles.control}>{m.market}
        <select value={country} onChange={event => { setCountry(event.target.value as Country); setRange(30) }}>
          {(Object.keys(countries) as Country[]).map(store => <option key={store} value={store}>{marketLabel[locale][store]} · {countries[store]}</option>)}
        </select>
      </label>
      <label className={styles.control}>{m.range}
        <select value={range} onChange={event => setRange(Number(event.target.value))}>
          {[...new Set([...(data?.ranges ?? [7,30]), range])].sort((a, b) => a - b).map(size => <option key={size} value={size}>{size} {m.days}</option>)}
        </select>
      </label>
    </div>
    {data ? <History data={data} locale={locale} /> : result?.key === key
      ? <div role="status"><p>{m.historyError}</p><button className={styles.button} onClick={() => { setResult(undefined); retry(revision + 1) }}>{m.retry}</button></div>
      : <p role="status">{m.loading}</p>}
  </div>
}
