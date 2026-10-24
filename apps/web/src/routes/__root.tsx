import { createRootRoute, HeadContent, Outlet, Scripts } from '@tanstack/react-router'
import { DevStyleX } from '../styles/dev-stylex'
import cssUrl from '../styles/app.css?url'

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: 'utf-8' },
      { name: 'viewport', content: 'width=device-width, initial-scale=1' },
      { title: 'GoFurry · Runtime skeleton' },
      { name: 'description', content: 'GoFurry V0.1-A SSR runtime verification.' },
    ],
    links: [{ rel: 'stylesheet', href: cssUrl }, { rel: 'icon', href: 'data:,' }],
  }),
  component: Root,
})

function Root() {
  return (
    <html lang="zh-CN">
      <head><HeadContent /><DevStyleX /></head>
      <body><Outlet /><Scripts /></body>
    </html>
  )
}
