import type { Locale, Region } from './preferences'

export function defaultLocale(region: Region): Locale { return region === 'CN' ? 'zh' : 'en' }
export function localeForPath(pathname: string, region: Region): Locale {
  const prefix = pathname.split('/')[1]
  return prefix === 'zh' || prefix === 'en' ? prefix : defaultLocale(region)
}
export function localizedPath(pathname: string, locale: Locale, region: Region) {
  const path = pathname.replace(/^\/(?:en|zh)(?=\/|$)/, '') || '/'
  return locale === defaultLocale(region) ? path : `/${locale}${path}`
}
export function isStartPath(pathname: string) { return /^\/(?:(?:en|zh)\/)?start\/?$/.test(pathname) }
