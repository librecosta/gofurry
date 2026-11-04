import { createRootRoute, HeadContent, Outlet, Scripts } from '@tanstack/react-router'
import styles from '../styles/document.module.scss'
import { PreferencesProvider, readPreferences, usePreferences } from '../preferences'
import { QuickAccessProvider } from '../start/quick-access-data'
import cssUrl from '../styles/global.scss?url'

export const Route = createRootRoute({
  beforeLoad: async ({ location }) => ({ preferences: await readPreferences({ data: location.pathname }) }),
  headers: () => ({ 'Cache-Control': 'private, no-store', Vary: 'Cookie' }),
  head: () => ({
    meta: [
      { charSet: 'utf-8' },
      { name: 'viewport', content: 'width=device-width, initial-scale=1' },
      { title: 'GoFurry · Runtime skeleton' },
      { name: 'description', content: 'GoFurry engineering foundation.' },
    ],
    links: [{ rel: 'stylesheet', href: cssUrl }, { rel: 'icon', href: 'data:,' }],
  }),
  component: Root,
})

function Root() {
  return <PreferencesProvider initial={Route.useRouteContext().preferences}><QuickAccessProvider><Document /></QuickAccessProvider></PreferencesProvider>
}

function Document() {
  const { theme, themePreference, locale, region } = usePreferences()
  return (
    <html lang={locale} data-theme={theme} data-theme-preference={themePreference} data-region={region} data-content-mode="SFW" className={styles.document}>
      <head><HeadContent /></head>
      <body className={styles.body}>
        <div aria-hidden="true" data-page-dots className={styles.dots} />
        <Outlet /><Scripts />
      </body>
    </html>
  )
}
