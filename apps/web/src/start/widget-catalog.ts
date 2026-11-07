import { lazy } from 'react'
import { SparkleIcon } from '@phosphor-icons/react/dist/ssr/Sparkle'
import type { Locale } from '../preferences'

type CategoryLabels = Record<Locale, string>
type WidgetCategory = { label: CategoryLabels; children?: Record<string, CategoryLabels> }

export const widgetCategories = {
  navigation: { label: { zh: '导航', en: 'Navigation' } },
} satisfies Record<string, WidgetCategory>

export const widgetCatalog = [{
  id: 'ai-hub',
  name: { zh: 'AI Hub', en: 'AI Hub' },
  description: { zh: '打开常用 AI 平台的官方网页。', en: 'Open the official sites of popular AI platforms.' },
  category: 'navigation' as keyof typeof widgetCategories,
  Icon: SparkleIcon,
  Panel: lazy(() => import('./tools/ai-hub')),
}] as const

export type WidgetId = typeof widgetCatalog[number]['id']
export const registeredWidgets = widgetCatalog.map(widget => widget.id)
export function getWidget(id: WidgetId) {
  return widgetCatalog.find(widget => widget.id === id)!
}
