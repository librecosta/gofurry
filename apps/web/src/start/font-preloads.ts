import navigationFont from '@fontsource-variable/noto-sans-sc/files/noto-sans-sc-113-wght-normal.woff2?url'
import platformFont from '@fontsource-variable/noto-sans-sc/files/noto-sans-sc-105-wght-normal.woff2?url'
import type { Locale } from '../preferences'

// With the pinned Noto package, these subsets contain offscreen menu glyphs
// (e.g. 暂 / 乎 and 哔 / 哩). Fetch them with SSR, not the first popup paint.
// Latin and the other visible Chinese subsets already load with the page.
export function startFontPreloads(locale: Locale | undefined) {
  return locale === 'zh' ? [navigationFont, platformFont].map(href => ({
    rel: 'preload', as: 'font', type: 'font/woff2', crossOrigin: 'anonymous' as const, href,
  })) : []
}
