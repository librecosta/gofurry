import { usePreferences } from '../preferences'

const messages = {
  zh: {
    official: '官方工具', widgets: '自选组件', showOfficial: '显示官方工具栏', showWidgets: '显示自选组件栏',
    calculator: '计算器', random: '随机工具', units: '单位换算',
    manage: '管理组件', close: '关闭', loading: '正在加载…',
    search: '搜索组件', categories: '组件分类', all: '全部组件', tabs: '组件管理',
    added: '已添加', library: '组件库', add: '添加', remove: '移除', empty: '还没有添加组件。',
    noMatches: '未找到匹配的组件。', previous: '上一页', next: '下一页', capacity: '最多可添加 7 个组件。',
    reorder: '移动顺序', reorderHelp: '拖动手柄，或使用上下方向键调整顺序。',
  },
  en: {
    official: 'Official tools', widgets: 'Widget rail', showOfficial: 'Show official tools', showWidgets: 'Show widget rail',
    calculator: 'Calculator', random: 'Random tools', units: 'Unit converter',
    manage: 'Manage widgets', close: 'Close', loading: 'Loading…',
    search: 'Search widgets', categories: 'Widget categories', all: 'All widgets', tabs: 'Widget management',
    added: 'Added', library: 'Library', add: 'Add', remove: 'Remove', empty: 'No widgets added yet.',
    noMatches: 'No matching widgets.', previous: 'Previous page', next: 'Next page', capacity: 'Add up to 7 widgets.',
    reorder: 'Reorder', reorderHelp: 'Drag the handle or use the Up and Down arrow keys.',
  },
}

export function useRailMessages() { return messages[usePreferences().locale] }
