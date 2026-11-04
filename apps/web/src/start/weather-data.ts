import type { Locale } from '../preferences'

// Keep only display data in browser memory, never the API's coordinates.
export type WeatherData = {
  state: 'ready' | 'stale' | 'unavailable'
  city: { zh: string; en: string } | null
  weather: { temperature_c: number; condition_code: string; condition_text: string; fetched_at: string } | null
  attributions: string[]
}

export const unavailableWeather: WeatherData = { state: 'unavailable', city: null, weather: null, attributions: [] }
export const weatherRefreshAge = 20 * 60 * 1000

function record(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value) }
function text(value: unknown, max = 256): value is string { return typeof value === 'string' && value.length > 0 && value.length <= max }

export function parseWeather(value: unknown): WeatherData {
  if (!record(value) || !['ready', 'stale', 'unavailable'].includes(String(value.state)) || !record(value.attribution)) throw new Error('Invalid weather')
  const attributions = value.attribution.weather_attributions
  if (!Array.isArray(attributions) || attributions.length > 32 || !attributions.every(item => text(item, 4096))) throw new Error('Invalid attribution')
  if (value.state === 'unavailable' && value.weather === null) return { ...unavailableWeather, attributions }
  const w = value.weather, city = record(value.location) ? value.location.city : null
  if (!record(w) || !record(city) || !text(city.zh) || !text(city.en)
    || typeof w.temperature_c !== 'number' || !Number.isFinite(w.temperature_c) || Math.abs(w.temperature_c) > 100
    || !text(w.condition_code, 3) || !/^[1-9]\d{2}$/.test(w.condition_code) || !text(w.condition_text)
    || !text(w.fetched_at, 64) || !Number.isFinite(Date.parse(w.fetched_at))
    || (value.state !== 'ready' && value.state !== 'stale')) throw new Error('Invalid weather')
  return { state: value.state, city: { zh: city.zh, en: city.en }, weather: {
    temperature_c: w.temperature_c, condition_code: w.condition_code, condition_text: w.condition_text, fetched_at: w.fetched_at,
  }, attributions }
}

export function safeAttributionURL(value: string): string | undefined {
  try {
    const url = new URL(value)
    if ((url.protocol === 'https:' || url.protocol === 'http:') && !url.username && !url.password) return url.href
  } catch { /* Provider text is rendered as text, never as HTML. */ }
}

export function weatherTemperature(value: number, locale: Locale) {
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value)}°C`
}

// A failed refresh may keep usable weather, but never weather older than one hour.
export function failedWeatherRefresh(current: WeatherData, now: number): WeatherData {
  const age = current.weather ? now - Date.parse(current.weather.fetched_at) : Infinity
  return age >= 0 && age < 60 * 60 * 1000 ? { ...current, state: 'stale' } : unavailableWeather
}
