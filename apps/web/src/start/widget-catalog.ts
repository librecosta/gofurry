import { lazy } from 'react'
import { ChartBarIcon } from '@phosphor-icons/react/dist/ssr/ChartBar'
import { ImagesSquareIcon } from '@phosphor-icons/react/dist/ssr/ImagesSquare'
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
}, {
  id: 'asset-hub',
  name: { zh: 'Asset Hub', en: 'Asset Hub' },
  description: { zh: '常用创作素材与资源网站。', en: 'Creative assets and resource sites.' },
  category: 'navigation',
  Icon: ImagesSquareIcon,
  Panel: lazy(() => import('./tools/asset-hub')),
}, {
  id: 'game-data-hub',
  name: { zh: 'Game Data Hub', en: 'Game Data Hub' },
  description: { zh: '游戏销量、玩家数据与技术资料。', en: 'Game sales, player analytics and technical references.' },
  category: 'navigation',
  Icon: ChartBarIcon,
  Panel: lazy(() => import('./tools/game-data-hub')),
}] as const

export type WidgetId = typeof widgetCatalog[number]['id']
export const registeredWidgets = widgetCatalog.map(widget => widget.id)
export function getWidget(id: WidgetId) {
  return widgetCatalog.find(widget => widget.id === id)!
}
