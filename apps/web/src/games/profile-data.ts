import type { Locale } from '../preferences'
import { mediaURL, type Copy, type List } from './data'

export type Country = 'cn' | 'us' | 'hk'
export type Currency = 'CNY' | 'USD' | 'HKD'
export const countries = { cn: 'CNY', us: 'USD', hk: 'HKD' } as const

import type { artworkFamilies } from './published-media'

export type PublicGameMediaImage = {
  asset_type: keyof typeof artworkFamilies; asset_family: 'store' | 'library' | 'community'; url: string
}
export type PublicGameScreenshot = { screenshot_id: string; thumbnail_url?: string; full_url?: string }
export type SteamMovieSources = Partial<Record<'mp4_max' | 'mp4_480' | 'webm_max' | 'webm_480' | 'hls_h264' | 'dash_h264' | 'dash_av1', string>>
export type PublicGameMovie = { movie_id: string; name: string; thumbnail_url?: string; highlight?: boolean; sources: SteamMovieSources }
export type PublicGameMedia = {
  version: 2; locale: Locale; fallback: boolean; cover?: PublicGameMediaImage
  images: PublicGameMediaImage[]; screenshots: PublicGameScreenshot[]; movies: PublicGameMovie[]
}

type Evidence = { locale: string; store_country: Country; observed_at: string; status: 'observed' | 'historical' }
type Label = { id: string; name: string }
export type StoreDetails = Evidence & {
  description?: string; type?: string; supported_languages?: string; developers?: string[]; publishers?: string[]
  platforms?: { windows?: boolean; mac?: boolean; linux?: boolean }; categories?: Label[]; genres?: Label[]
  genre_codes?: string[]; release?: { coming_soon?: boolean; date_text?: string }; release_precision: string; release_day?: string
}
export type StoreMedia = Evidence & { header?: string; capsule?: string; screenshots?: string[]; movies?: string[] }
type PriceState = 'free' | 'zero_price' | 'priced'
export type Quote = { locale: string; store_country: Country; currency: Currency; state: PriceState; observed_at: string; confirmed_at: string
  amounts: null | { currency: Currency; initial: number; final: number; discount_percent: number } }
export type GameSource = { platform_code: string; external_key: string; active: boolean; preferred: boolean; source_url?: string
  details?: StoreDetails; media?: StoreMedia; quotes: Quote[]; players?: { count: number; observed_at: string } }
export type Story = { app_id: string; gid: string; title: string; url: string; published_at: string }
export type Related = { public_id: string; slug: string; localizations: Copy[]; cover_url?: string }
export type Profile = Related & { published_media?: PublicGameMedia; sources: GameSource[]; related: Related[]; news: List<Story>; news_status: 'available' | 'unavailable' }
export type PlayerDay = { day: string; samples: number; maximum: number; average: number; observed_at: string }
export type PriceDay = { day: string; state: PriceState; final: number | null; original: number | null; discount_percent: number | null
  observed_at: string; observation_id: string }
export type Insights = {
  app_id?: string; store_country: Country; currency: Currency; price_locale?: string
  range: number; ranges: number[]; from_day: string; through_day: string
  player_days: PlayerDay[]; price_days: PriceDay[]; price_changes: PriceDay[]; news: Story[]
}

export const validApp = (id: string) => /^[1-9][0-9]{0,9}$/.test(id) && Number(id) <= 4294967295

export function profileMediaURL(raw: string | undefined, video = false) {
  const safe = mediaURL(raw)
  if (!safe) return undefined
  return (video ? /\.(mp4|webm)$/i : /\.(jpe?g|png|webp|avif)$/i).test(new URL(safe).pathname) ? safe : undefined
}

export function sourceURL(source: GameSource) {
  const raw = source.source_url
  if (source.platform_code === 'steam' && validApp(source.external_key) &&
      raw === 'https://store.steampowered.com/app/' + source.external_key + '/') return raw
  if (source.platform_code === 'itch_io' && raw && /^https:\/\/[a-z0-9-]+\.itch\.io\/[a-z0-9_-]+\/?$/.test(raw)) return raw
  return undefined
}

export function storyURL(story: Story) {
  if (typeof story.url !== 'string' || !validApp(story.app_id) || story.url.length > 220) return undefined
  const root = 'https://steamcommunity.com/'
  for (const path of [root + 'announcements/detail/', root + 'games/' + story.app_id + '/announcements/detail/',
    root + 'ogg/' + story.app_id + '/announcements/detail/', 'https://store.steampowered.com/news/app/' + story.app_id + '/view/']) {
    if (story.url.startsWith(path) && /^[1-9][0-9]{0,19}$/.test(story.url.slice(path.length))) return story.url
  }
  return undefined
}

export function utcDate(raw: string, locale: Locale, clock = false) {
  const parsed = new Date(raw.length === 10 ? raw + 'T00:00:00Z' : raw)
  return Number.isFinite(parsed.valueOf()) ? new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en', {
    dateStyle: 'medium', ...(clock ? { timeStyle: 'short' } : {}), timeZone: 'UTC',
  }).format(parsed) + (clock ? ' UTC' : '') : raw
}

export function money(amount: number, currency: Currency, locale: Locale) {
  return new Intl.NumberFormat(locale === 'zh' ? 'zh-CN' : 'en', { style: 'currency', currency, currencyDisplay: 'code' }).format(amount / 100)
}
