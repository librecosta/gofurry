import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_start/start')({
  loader: ({ context }) => context.preferences.locale,
  head: ({ loaderData: locale }) => ({ meta: [{ title: locale === 'zh' ? 'GoFurry · 起始页' : 'GoFurry · Start' }, { name: 'robots', content: 'noindex' }] }),
  headers: () => ({ 'X-Robots-Tag': 'noindex' }),
})
