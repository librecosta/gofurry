import { useState } from 'react'
import type { Locale } from '../preferences'
import { marketLabel, profileCopy } from './profile-copy'
import { money, utcDate, type PublicSteamChange, type SteamFacts } from './profile-data'
import { readGameChanges, type ProfilePage } from './profile-load'
import styles from './profile.module.scss'

const copy = {
  zh: {
    title: 'Steam 发行与价格事实', release: '官方发行', available: '已可用', upcoming: '即将发行',
    unknown: '未知', none: '无发行日期', tba: '待定', first: '首次正式可用', platforms: '平台支持',
    yes: '支持', no: '不支持', staleFact: '历史观察', unavailable: '来源已撤回', languages: '游戏语言', interface: '界面', subtitles: '字幕', audio: '完整语音',
    prices: '地区报价', stale: '历史报价', player: '近 30 天的玩家采样', observed: '观测', missing: '缺测',
    recent: 'Steam 变化记录', all: '查看变化历史', more: '更多变化', failed: '变化记录暂不可用。', empty: '暂无可核实的变化。',
    samples: (count: number, days: number, missing: number) => count + ' 次 · ' + days + ' 天有数据 · ' + missing + ' 天缺测',
  },
  en: {
    title: 'Steam release and price facts', release: 'Official release', available: 'Available', upcoming: 'Upcoming',
    unknown: 'Unknown', none: 'No release date', tba: 'To be announced', first: 'First available', platforms: 'Platform support',
    yes: 'Supported', no: 'Unsupported', staleFact: 'Historical observation', unavailable: 'Source withdrawn', languages: 'Game languages', interface: 'Interface', subtitles: 'Subtitles', audio: 'Full audio',
    prices: 'Regional price', stale: 'Historical quote', player: 'Player sampling over the last 30 UTC days', observed: 'Observed', missing: 'Missing',
    recent: 'Steam change history', all: 'View change history', more: 'More changes', failed: 'Change history is temporarily unavailable.', empty: 'No verified changes yet.',
    samples: (count: number, days: number, missing: number) => count + ' samples · ' + days + ' days observed · ' + missing + ' days missing',
  },
}
const events: Record<string, [string, string]> = {
  game_became_available: ['观察到可用','Observed available'], game_availability_withdrawn: ['回到待发行状态','Returned to upcoming'],
  game_release_plan_changed: ['发行计划变化','Release plan changed'], game_became_free: ['转为免费','Became free'],
  game_became_paid: ['转为付费','Became paid'], game_price_state_changed: ['价格状态变化','Price state changed'],
  game_price_currency_changed: ['报价币种变化','Quote currency changed'], game_price_decreased: ['价格下降','Price decreased'],
  game_price_increased: ['价格上涨','Price increased'], game_discount_started: ['折扣开始','Discount started'],
  game_discount_ended: ['折扣结束','Discount ended'], game_discount_changed: ['折扣变化','Discount changed'],
  windows_support_added: ['新增 Windows 支持','Windows support added'], windows_support_removed: ['移除 Windows 支持','Windows support removed'],
  mac_support_added: ['新增 macOS 支持','macOS support added'], mac_support_removed: ['移除 macOS 支持','macOS support removed'],
  linux_support_added: ['新增 Linux 支持','Linux support added'], linux_support_removed: ['移除 Linux 支持','Linux support removed'],
}
const precisionLabels: Record<string, [string, string]> = {
  day: ['精确到日','Day'], month: ['精确到月','Month'], quarter: ['精确到季度','Quarter'], year: ['精确到年','Year'],
}
const calendarWindow = (from: string, to: string, locale: Locale) => from === to ? utcDate(from, locale) : utcDate(from, locale) + ' – ' + utcDate(to, locale)

function ChangeHistory({ changes, page }: { changes: PublicSteamChange[]; page: ProfilePage }) {
  const m = copy[page.locale]
  const [loaded, setLoaded] = useState<{ items: PublicSteamChange[]; after?: string }>()
  const [status, setStatus] = useState<'idle' | 'loading' | 'failed'>('idle')
  async function load() {
    setStatus('loading')
    try {
      const result = await readGameChanges({ data: { id: page.id, region: page.region, locale: page.locale, after: loaded?.after } })
      if (!result) { setStatus('failed'); return }
      setLoaded(previous => ({ after: result.after, items: [...(previous?.items ?? []), ...result.items] }))
      setStatus('idle')
    } catch { setStatus('failed') }
  }
  const items = loaded?.items ?? changes
  return <div>
    <h3>{m.recent}</h3>
    {items.length ? <ul className={styles.news}>{items.map((item,index) => <li key={item.day + item.code + item.market + index}>
      <time dateTime={item.day}>{utcDate(item.day,page.locale)} UTC</time>
      <strong>{events[item.code]?.[page.locale === 'zh' ? 0 : 1] ?? item.code}</strong>
      <span>{marketLabel[page.locale][item.market]}</span>
    </li>)}</ul> : <p className={styles.muted}>{m.empty}</p>}
    {status === 'failed' && <p role="status">{m.failed}</p>}
    {(!loaded || loaded.after) && <button type="button" className={styles.button} disabled={status === 'loading'} onClick={() => void load()}>
      {status === 'loading' ? profileCopy[page.locale].loading : loaded ? m.more : m.all}
    </button>}
  </div>
}

export function SteamProfileFacts({ value, page }: { value: SteamFacts; page: ProfilePage }) {
  const locale = page.locale
  const m = copy[locale]
  const p = profileCopy[locale]
  const languageIndex = locale === 'zh' ? 0 : 1
  const known = (v?: boolean) => v === undefined ? m.unknown : v ? m.yes : m.no
  const quality = (key: string) => value.quality[key]?.quality === 'stale' ? ' (' + m.staleFact + ')' :
    value.quality[key]?.quality === 'unavailable' ? ' (' + m.unavailable + ')' : ''
  const release = value.official_release
  const releaseDate = release?.window_start && release.window_end ? calendarWindow(release.window_start,release.window_end,locale)
    : release?.precision === 'tba' ? m.tba : release?.precision === 'none' ? m.none : m.unknown
  return <section id="steam-facts" className={styles.section}>
    <h2>{m.title}</h2>
    <dl className={styles.facts}>
      {release && <div><dt>{m.release}</dt><dd>{release.state === 'available' ? m.available : release.state === 'upcoming' ? m.upcoming : m.unknown}
        {' · '}{releaseDate}{precisionLabels[release.precision] && ' · ' + precisionLabels[release.precision][languageIndex]}
        {value.release_observed_at && <small>{' · '}{m.observed} <time dateTime={value.release_observed_at}>{utcDate(value.release_observed_at,locale)}</time></small>}
      </dd></div>}
      {value.first_available && <div><dt>{m.first}</dt><dd>{calendarWindow(value.first_available.window_start,value.first_available.window_end,locale)}</dd></div>}
      <div><dt>{m.platforms}</dt><dd>{(['windows','mac','linux'] as const).map(key => p[key] + ' ' + known(value.platforms[key]) + quality(key)).join(' · ')}</dd></div>
      {value.prices.map(price => <div key={price.market}><dt>{m.prices} · {marketLabel[locale][price.market]}</dt>
        <dd>{price.state === 'free' ? p.free : price.final === undefined ? m.unknown : money(price.final,price.currency,locale)}
          {price.discount ? ' · −' + price.discount + '%' : ''}
          {price.quality === 'stale' && ' · ' + m.stale}
          {' · '}<time dateTime={price.observed_at}>{utcDate(price.observed_at,locale)}</time></dd></div>)}
      <div><dt>{m.player}</dt><dd>{m.samples(value.players.samples,value.players.days,value.players.missing_days)}</dd></div>
    </dl>
    {!!value.languages?.length && <details><summary>{m.languages}</summary><ul>
      {value.languages.map(item => <li key={item.sort_order}>{item.steam_name} · {m.interface} {known(item.interface)} · {m.subtitles} {known(item.subtitles)} · {m.audio} {known(item.full_audio)}</li>)}
    </ul></details>}
    <ChangeHistory changes={value.changes} page={page} />
  </section>
}
