import type { Locale, Region } from '../preferences'

export const categories = ['search', 'furry', 'games', 'art', 'novel'] as const
export type Category = typeof categories[number]
type Platform = { id: string; name: Record<Locale, string>; template: string }
const platform = (id: string, zh: string, en: string, template: string): Platform => ({ id, name: { zh, en }, template })

// Explicit SFW-oriented allowlist; adult/mixed adult-art providers are omitted.
// General engines request strict safe search. External sites own their content.
export const platforms: Record<Category, Platform[]> = {
  search: [
    platform('bing', '必应', 'Bing', 'https://www.bing.com/search?adlt=strict&q={kw}'),
    platform('google', 'Google', 'Google', 'https://www.google.com/search?safe=active&q={kw}'),
    platform('duckduckgo', 'DuckDuckGo', 'DuckDuckGo', 'https://duckduckgo.com/?kp=1&q={kw}'),
    platform('bilibili', '哔哩哔哩', 'Bilibili', 'https://search.bilibili.com/all?keyword={kw}'),
    platform('xiaohongshu', '小红书', 'Rednote', 'https://www.xiaohongshu.com/search_result?keyword={kw}'),
    platform('zhihu', '知乎', 'Zhihu', 'https://www.zhihu.com/search?type=content&q={kw}'),
    platform('weibo', '微博', 'Weibo', 'https://s.weibo.com/weibo?q={kw}'),
  ],
  furry: [platform('wikifur', '兽维基', 'WikiFur', 'https://{locale}.wikifur.com/wiki/{kw}')],
  games: [platform('epic', 'Epic', 'Epic', 'https://store.epicgames.com/browse?q={kw}')],
  art: [platform('zcool', '站酷', 'ZCool', 'https://www.zcool.com.cn/search/content?word={kw}')],
  // Public-domain libraries replace the legacy novel providers' adult catalogs.
  novel: [
    platform('wikisource', '维基文库', 'Wikisource', 'https://{locale}.wikisource.org/w/index.php?search={kw}'),
    platform('gutenberg', '古登堡计划', 'Project Gutenberg', 'https://www.gutenberg.org/ebooks/search/?query={kw}'),
  ],
}

export function defaultPlatform(category: Category, region: Region) {
  return category === 'search' && region === 'INTL' ? platforms.search[1] : platforms[category][0]
}

export function searchURL(target: Platform, keyword: string, locale: Locale) {
  return target.template.replace('{locale}', locale).replace('{kw}', encodeURIComponent(keyword.trim()))
}
