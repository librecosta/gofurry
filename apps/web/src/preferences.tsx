import { createContext, useContext, useState, type ReactNode } from 'react'
import { createServerFn } from '@tanstack/react-start'
import { getCookie, setResponseHeader } from '@tanstack/react-start/server'

export type Region = 'CN' | 'INTL'
export type Locale = 'zh' | 'en'
export type Theme = 'light' | 'dark'
export type Preferences = { region: Region; locale: Locale; theme: Theme }

// Region belongs to the deployment; no request parameter or preference changes it.
export const readPreferences = createServerFn({ method: 'GET' }).handler((): Preferences => {
  setResponseHeader('Cache-Control', 'private, no-store')
  const region = process.env.WEB_REGION ?? 'CN'
  if (region !== 'CN' && region !== 'INTL') throw new Error('WEB_REGION must be CN or INTL')
  const locale = getCookie('gf-locale')
  return {
    region,
    locale: locale === 'zh' || locale === 'en' ? locale : region === 'CN' ? 'zh' : 'en',
    theme: getCookie('gf-theme') === 'dark' ? 'dark' : 'light',
  }
})

type Context = Preferences & { setLocale: (locale: Locale) => void; setTheme: (theme: Theme) => void }
const PreferencesContext = createContext<Context | null>(null)

function remember(name: string, value: string) {
  document.cookie = `${name}=${value}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === 'https:' ? '; Secure' : ''}`
}

export function PreferencesProvider({ initial, children }: { initial: Preferences; children: ReactNode }) {
  const [locale, changeLocale] = useState(initial.locale)
  const [theme, changeTheme] = useState(initial.theme)
  return <PreferencesContext value={{
    region: initial.region, locale, theme,
    setLocale: (value) => { remember('gf-locale', value); changeLocale(value) },
    setTheme: (value) => { remember('gf-theme', value); changeTheme(value) },
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
