import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { createServerFn } from '@tanstack/react-start'
import { getCookie, setResponseHeader } from '@tanstack/react-start/server'
import { isStartPath, localeForPath, localizedPath } from './locale-path'

export type Region = 'CN' | 'INTL'
export type Locale = 'zh' | 'en'
export type Theme = 'light' | 'dark'
export type ThemePreference = Theme | 'system'
export type ContentPreference = 'SFW' | 'NSFW'
export type Preferences = { region: Region; locale: Locale; theme: Theme; themePreference: ThemePreference; contentPreference: ContentPreference; quickAccess: boolean; showFurrySearch: boolean }

// Region belongs to the deployment; no request parameter or preference changes it.
export const readPreferences = createServerFn({ method: 'GET' }).validator((pathname: string) => {
  if (typeof pathname !== 'string' || pathname.length > 2048) throw new Error('Invalid pathname')
  return pathname
}).handler(({ data: pathname }): Preferences => {
  setResponseHeader('Cache-Control', 'private, no-store')
  const region = process.env.WEB_REGION ?? 'CN'
  if (region !== 'CN' && region !== 'INTL') throw new Error('WEB_REGION must be CN or INTL')
  const locale = getCookie('gf-locale')
  const savedTheme = getCookie('gf-theme')
  const themePreference = savedTheme === 'dark' || savedTheme === 'light' ? savedTheme : 'system'
  return {
    region,
    locale: isStartPath(pathname) ? localeForPath(pathname, region) : locale === 'zh' || locale === 'en' ? locale : region === 'CN' ? 'zh' : 'en',
    theme: themePreference === 'dark' ? 'dark' : 'light', themePreference,
    quickAccess: getCookie(`gf-quick-access-${region}`) === '1',
    showFurrySearch: getCookie(`gf-furry-search-${region}`) !== '0',
    contentPreference: region === 'INTL' && getCookie('gf-content-preference') === 'NSFW' ? 'NSFW' : 'SFW',
  }
})

type Context = Preferences & { setLocale: (locale: Locale) => void; setTheme: (theme: ThemePreference) => void; setContentPreference: (value: ContentPreference) => void; setQuickAccess: (value: boolean) => void; setShowFurrySearch: (value: boolean) => void }
const PreferencesContext = createContext<Context | null>(null)

function remember(name: string, value: string) {
  document.cookie = `${name}=${value}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === 'https:' ? '; Secure' : ''}`
}

export function PreferencesProvider({ initial, children }: { initial: Preferences; children: ReactNode }) {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const [legacyLocale, changeLocale] = useState<Locale | null>(null)
  const [theme, changeTheme] = useState(initial.theme)
  const [themePreference, changeThemePreference] = useState(initial.themePreference)
  const [contentPreference, changeContentPreference] = useState(initial.contentPreference)
  const [quickAccess, changeQuickAccess] = useState(initial.quickAccess)
  const [showFurrySearch, changeShowFurrySearch] = useState(initial.showFurrySearch)
  useEffect(() => { changeLocale(null) }, [pathname])
  useEffect(() => {
    const media = matchMedia('(prefers-color-scheme: dark)')
    const update = () => changeTheme(themePreference === 'system' ? media.matches ? 'dark' : 'light' : themePreference)
    update()
    if (themePreference !== 'system') return
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [themePreference])
  return <PreferencesContext value={{
    region: initial.region, locale: isStartPath(pathname) ? localeForPath(pathname, initial.region) : legacyLocale ?? initial.locale, theme, themePreference, contentPreference, quickAccess, showFurrySearch,
    setShowFurrySearch: value => {
      try { remember(`gf-furry-search-${initial.region}`, value ? '1' : '0') } catch { /* In-memory preference remains usable. */ }
      changeShowFurrySearch(value)
    },
    setQuickAccess: value => {
      try { remember(`gf-quick-access-${initial.region}`, value ? '1' : '0') } catch { /* In-memory preference remains usable. */ }
      changeQuickAccess(value)
    },
    setLocale: (value) => {
      remember('gf-locale', value)
      if (isStartPath(pathname)) void navigate({ to: localizedPath(pathname, value, initial.region), resetScroll: false })
      else changeLocale(value) // Existing engineering diagnostics are not business URL routes.
    },
    setTheme: (value) => {
      remember('gf-theme', value); changeThemePreference(value)
      changeTheme(value === 'system' ? matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light' : value)
    },
    setContentPreference: (value) => {
      const next = initial.region === 'INTL' ? value : 'SFW'
      remember('gf-content-preference', next); changeContentPreference(next)
    },
  }}>{children}</PreferencesContext>
}

export function usePreferences() {
  const value = useContext(PreferencesContext)
  if (!value) throw new Error('PreferencesProvider is required')
  return value
}

const messages = {
  zh: { interact: '验证交互', title: '基础交互检查', description: '支持键盘、焦点与主题偏好。', close: '关闭', disabled: '禁用状态', status: '运行状态', back: '返回', theme: '主题', language: '语言', search: '搜索网络' },
  en: { interact: 'Check interaction', title: 'Interaction check', description: 'Keyboard, focus and theme preferences.', close: 'Close', disabled: 'Disabled', status: 'Runtime status', back: 'Back', theme: 'Theme', language: 'Language', search: 'Search the web' },
}
export function useMessages() { return messages[usePreferences().locale] }
