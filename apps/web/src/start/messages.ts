import { usePreferences } from '../preferences'

const messages = {
  zh: {
    language: '切换为 English', themeLight: '切换为浅色主题', themeDark: '切换为深色主题',
    github: '打开 GoFurry GitHub（新标签页）', brand: 'GoFurry 品牌入口', user: '用户菜单', login: '登录', settings: '设置', unavailable: '暂不可用',
    categories: '搜索分类', platforms: '搜索平台', search: '搜索站点或内容…', submit: '搜索（新标签页）',
    suggestions: '搜索建议',
    weather: { title: '天气', details: '天气详情', unavailable: '天气暂不可用', stale: '缓存天气，暂未获取到更新数据。', fetched: '数据获取于', weatherSource: '天气数据：', locationSource: 'IP 定位：', provider: '和风天气 / QWeather', selectCity: '选择城市', automatic: '自动定位', citySearch: '搜索城市（中文 / English）', cityResults: '匹配城市', cityUnavailable: '城市搜索暂不可用', noCities: '未找到匹配城市', citySource: '城市数据：', back: '返回天气' },
    close: '关闭设置', clear: '清空搜索', navigation: '站内导航', home: '首页', sites: '导航', games: '游戏',
    languageSetting: '语言', appearance: '外观', system: '跟随系统', light: '浅色', dark: '深色',
    basic: '基础', advanced: '高级', githubLead: '用心打造，开源于 ', githubStop: '。',
    contentPreference: '内容偏好', contentHint: '仅保存偏好，当前后端内容仍为 SFW。', moreCategories: '滚动查看其他分类',
    company: '成都远岸开物科技有限公司', icpPending: 'ICP备案待配置', securityPending: '公安联网备案待配置', securityIcon: '公安备案',
    category: { search: '搜索', furry: '兽圈', games: '游戏', art: '艺术', novel: '小说' },
  },
  en: {
    language: 'Switch to 中文', themeLight: 'Switch to light theme', themeDark: 'Switch to dark theme',
    github: 'Open GoFurry GitHub (new tab)', brand: 'GoFurry brand', user: 'User menu', login: 'Sign in', settings: 'Settings', unavailable: 'Not available yet',
    categories: 'Search categories', platforms: 'Search platform', search: 'Search sites or content…', submit: 'Search (new tab)',
    suggestions: 'Search suggestions',
    weather: { title: 'Weather', details: 'Weather details', unavailable: 'Weather unavailable', stale: 'Cached weather; updated data is not available yet.', fetched: 'Fetched', weatherSource: 'Weather:', locationSource: 'IP location:', provider: 'QWeather', selectCity: 'Choose city', automatic: 'Automatic location', citySearch: 'Search cities (中文 / English)', cityResults: 'Matching cities', cityUnavailable: 'City search unavailable', noCities: 'No matching cities', citySource: 'City data:', back: 'Back to weather' },
    close: 'Close settings', clear: 'Clear search', navigation: 'Site navigation', home: 'Home', sites: 'Sites', games: 'Games',
    languageSetting: 'Language', appearance: 'Appearance', system: 'System', light: 'Light', dark: 'Dark',
    basic: 'Basic', advanced: 'Advanced', githubLead: 'Made with care. Open source on ', githubStop: '.',
    contentPreference: 'Content preference', contentHint: 'Preference only. Backend content is currently SFW.', moreCategories: 'Scroll for more categories',
    company: 'Chengdu Yuanan Kaiwu Technology Co., Ltd.', icpPending: 'ICP filing pending configuration', securityPending: 'Public security filing pending configuration', securityIcon: 'Public security filing',
    category: { search: 'Search', furry: 'Furry', games: 'Games', art: 'Art', novel: 'Novel' },
  },
}

export function useStartMessages() { return messages[usePreferences().locale] }
