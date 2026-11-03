import { createFileRoute, notFound } from '@tanstack/react-router'
import { defaultLocale } from '../locale-path'

export const Route = createFileRoute('/_start/$language/start')({
  beforeLoad: ({ params, context }) => {
    if ((params.language !== 'en' && params.language !== 'zh') || params.language === defaultLocale(context.preferences.region)) throw notFound()
  },
  loader: ({ context }) => context.preferences.locale,
  head: ({ loaderData: locale }) => ({ meta: [{ title: locale === 'zh' ? 'GoFurry · 起始页' : 'GoFurry · Start' }, { name: 'robots', content: 'noindex' }] }),
  headers: () => ({ 'X-Robots-Tag': 'noindex' }),
})
