import { createRootRoute, HeadContent, Outlet, Scripts } from '@tanstack/react-router'
import * as stylex from '@stylexjs/stylex'
import { DevStyleX } from '../styles/dev-stylex'
import { darkTheme, tokens } from '../styles/tokens.stylex'
import { PreferencesProvider, readPreferences, usePreferences } from '../preferences'
import cssUrl from '../styles/app.css?url'

export const Route = createRootRoute({
  beforeLoad: async () => ({ preferences: await readPreferences() }),
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
  return <PreferencesProvider initial={Route.useRouteContext().preferences}><Document /></PreferencesProvider>
}

function Document() {
  const { theme, locale, region } = usePreferences()
  return (
    <html lang={locale} data-theme={theme} data-region={region} data-content-mode="SFW" {...stylex.props(styles.document, theme === 'dark' && darkTheme)}>
      <head><HeadContent /><DevStyleX /></head>
      <body {...stylex.props(styles.body)}>
        <div aria-hidden="true" {...stylex.props(styles.dots)} />
        <Outlet /><Scripts />
      </body>
    </html>
  )
}

const styles = stylex.create({
  document: { fontFamily: tokens.fontSans, backgroundColor: tokens.pageBackground, color: tokens.text },
  body: { margin: 0, minHeight: '100vh' },
  dots: {
    position: 'absolute', top: 0, insetInline: 0, height: tokens.dotHeight,
    pointerEvents: 'none', zIndex: -1,
    backgroundImage: `radial-gradient(circle, ${tokens.dotColor} ${tokens.dotRadius}, transparent ${tokens.dotRadius})`,
    backgroundSize: `${tokens.dotSpacing} ${tokens.dotSpacing}`,
    maskImage: 'linear-gradient(to bottom, #000, transparent)',
  },
})
