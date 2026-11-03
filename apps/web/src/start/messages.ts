import { usePreferences } from '../preferences'

const messages = {
  zh: {
    language: '切换为 English', themeLight: '切换为浅色主题', themeDark: '切换为深色主题',
    github: '打开 GoFurry GitHub（新标签页）', user: '用户菜单', login: '登录', settings: '设置', unavailable: '暂不可用',
    categories: '搜索分类', platforms: '搜索平台', search: '搜索站点或内容…', submit: '搜索（新标签页）',
    company: '成都远岸开物科技有限公司', icpPending: 'ICP备案待配置', securityPending: '公安联网备案待配置', securityIcon: '公安备案',
    category: { search: '搜索', furry: '兽圈', games: '游戏', art: '艺术', novel: '小说' },
  },
  en: {
    language: 'Switch to 中文', themeLight: 'Switch to light theme', themeDark: 'Switch to dark theme',
    github: 'Open GoFurry GitHub (new tab)', user: 'User menu', login: 'Sign in', settings: 'Settings', unavailable: 'Not available yet',
    categories: 'Search categories', platforms: 'Search platform', search: 'Search sites or content…', submit: 'Search (new tab)',
    company: 'Chengdu Yuanan Kaiwu Technology Co., Ltd.', icpPending: 'ICP filing pending configuration', securityPending: 'Public security filing pending configuration', securityIcon: 'Public security filing',
    category: { search: 'Search', furry: 'Furry', games: 'Games', art: 'Art', novel: 'Novel' },
  },
}

export function useStartMessages() { return messages[usePreferences().locale] }
