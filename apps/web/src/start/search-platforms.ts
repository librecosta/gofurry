import type { ContentPreference, Locale, Region } from '../preferences'
import { encodeFalooKeyword } from './search-encoding'

export const categories = ['search', 'furry', 'games', 'art', 'novel'] as const
export type Category = typeof categories[number]
export type Platform = {
  id: string
  name: Record<Locale, string>
  category: Category
  regions: readonly Region[]
  nsfw?: true
} & ({ template: string; buildURL?: never } | { template?: never; buildURL: (keyword: string) => string })

const both = ['CN', 'INTL'] as const, cn = ['CN'] as const, intl = ['INTL'] as const
const platform = (id: string, category: Category, zh: string, en: string, template: string, regions: readonly Region[] = both, nsfw?: true): Platform => ({ id, category, name: { zh, en }, template, regions, ...(nsfw ? { nsfw } : {}) })

// C5's explicitly approved deployment catalog. Locale changes labels, not membership.
// External destinations own their content; link visibility is not content filtering.
export const platforms: readonly Platform[] = [
  platform('bing', 'search', '必应', 'Bing', 'https://www.bing.com/search?adlt=strict&q={kw}'),
  platform('google', 'search', 'Google', 'Google', 'https://www.google.com/search?safe=active&q={kw}', intl),
  platform('duckduckgo', 'search', 'DuckDuckGo', 'DuckDuckGo', 'https://duckduckgo.com/?kp=1&q={kw}', intl),
  platform('youtube', 'search', 'YouTube', 'YouTube', 'https://www.youtube.com/results?search_query={kw}', intl),
  platform('x', 'search', 'X', 'X', 'https://x.com/search?q={kw}', intl),
  platform('bluesky', 'search', 'Bluesky', 'Bluesky', 'https://bsky.app/search?q={kw}', intl),
  platform('reddit', 'search', 'Reddit', 'Reddit', 'https://www.reddit.com/search/?q={kw}', intl),
  platform('360', 'search', '360 搜索', '360 Search', 'https://www.so.com/s?q={kw}', cn),
  platform('sohu', 'search', '搜狐搜索', 'Sohu Search', 'https://search.sohu.com/?keyword={kw}', cn),
  platform('baidu', 'search', '百度', 'Baidu', 'https://www.baidu.com/s?wd={kw}', cn),
  platform('bilibili', 'search', '哔哩哔哩', 'Bilibili', 'https://search.bilibili.com/all?keyword={kw}', cn),
  platform('xiaohongshu', 'search', '小红书', 'Xiaohongshu', 'https://www.xiaohongshu.com/search_result?keyword={kw}', cn),
  platform('zhihu', 'search', '知乎', 'Zhihu', 'https://www.zhihu.com/search?type=content&q={kw}', cn),
  platform('weibo', 'search', '微博', 'Weibo', 'https://s.weibo.com/weibo?q={kw}', cn),

  platform('wikifur', 'furry', '兽维基', 'WikiFur', 'https://{locale}.wikifur.com/wiki/{kw}'),
  platform('furaffinity', 'furry', 'FurAffinity', 'FurAffinity', 'https://www.furaffinity.net/search/?q={kw}'),
  platform('e621', 'furry', 'e621', 'e621', 'https://e621.net/posts?tags={kw}', intl, true),
  platform('yiffparty', 'furry', 'YiffParty', 'YiffParty', 'https://yiff-party.com/search/?q={kw}', intl, true),
  platform('wilddream', 'furry', '野性疆界', 'WildDream', 'https://www.wilddream.net/Art/index/index?keyword={kw}'),
  platform('furrynovel', 'furry', '兽人控小说站', 'FurryNovel', 'https://furrynovel.com/zh/search?keyword={kw}'),
  platform('linpx', 'furry', 'Linpx', 'Linpx', 'https://linpx.ink/search?word={kw}'),

  platform('steam', 'games', 'Steam', 'Steam', 'https://store.steampowered.com/search?term={kw}'),
  platform('itchio', 'games', 'itch.io', 'itch.io', 'https://itch.io/search?q={kw}'),
  platform('epic', 'games', 'Epic Games', 'Epic Games', 'https://store.epicgames.com/browse?q={kw}'),
  platform('roblox', 'games', 'Roblox', 'Roblox', 'https://www.roblox.com/discover/?Keyword={kw}'),
  platform('gog', 'games', 'GOG', 'GOG', 'https://www.gog.com/en/games?query={kw}'),
  platform('gamejolt', 'games', 'Game Jolt', 'Game Jolt', 'https://gamejolt.com/search?q={kw}'),

  platform('pixiv', 'art', 'Pixiv', 'Pixiv', 'https://www.pixiv.net/tags/{kw}'),
  platform('deviantart', 'art', 'DeviantArt', 'DeviantArt', 'https://www.deviantart.com/search?q={kw}'),
  platform('artstation', 'art', 'ArtStation', 'ArtStation', 'https://www.artstation.com/search?query={kw}'),
  platform('pinterest', 'art', 'Pinterest', 'Pinterest', 'https://www.pinterest.com/search/pins/?q={kw}'),
  platform('zcool', 'art', '站酷', 'ZCOOL', 'https://www.zcool.com.cn/search/content?word={kw}'),
  platform('booth', 'art', 'BOOTH', 'BOOTH', 'https://booth.pm/ja/search/{kw}'),
  platform('sketchfab', 'art', 'Sketchfab', 'Sketchfab', 'https://sketchfab.com/search?q={kw}&type=models'),

  platform('wattpad', 'novel', 'Wattpad', 'Wattpad', 'https://www.wattpad.com/search/{kw}', intl),
  platform('royalroad', 'novel', 'Royal Road', 'Royal Road', 'https://www.royalroad.com/fictions/search?title={kw}', intl),
  platform('ao3', 'novel', 'AO3', 'AO3', 'https://archiveofourown.org/works/search?work_search%5Bquery%5D={kw}', intl),
  platform('webnovel', 'novel', 'WebNovel', 'WebNovel', 'https://www.webnovel.com/search?keywords={kw}', intl),
  platform('scribblehub', 'novel', 'Scribble Hub', 'Scribble Hub', 'https://www.scribblehub.com/?s={kw}&post_type=fictionposts', intl),
  platform('fanfiction', 'novel', 'FanFiction.net', 'FanFiction.net', 'https://www.fanfiction.net/search/?keywords={kw}&ready=1&type=story', intl),
  platform('qidian', 'novel', '起点中文网', 'Qidian', 'https://www.qidian.com/search?kw={kw}', cn),
  platform('fanqie', 'novel', '番茄小说', 'Fanqie Novel', 'https://fanqienovel.com/search/{kw}', cn),
  platform('qimao', 'novel', '七猫小说', 'Qimao', 'https://www.qimao.com/search/index/?keyword={kw}', cn),
  { id: 'faloo', name: { zh: '飞卢小说', en: 'Faloo' }, category: 'novel', regions: cn,
    buildURL: keyword => `https://b.faloo.com/l/0/1.html?t=1&k=${encodeFalooKeyword(keyword)}` },
  platform('sfacg', 'novel', '菠萝包轻小说', 'SFACG', 'https://s.sfacg.com/?Key={kw}&S=1&SS=0', cn),
]

export function regionalPlatforms(region: Region) {
  return platforms.filter(item => item.regions.includes(region))
}

export function availablePlatforms(region: Region, content: ContentPreference, showFurry: boolean) {
  return regionalPlatforms(region).filter(item => (showFurry || item.category !== 'furry') && (!item.nsfw || region === 'INTL' && content === 'NSFW'))
}

export function resolveSelection(available: readonly Platform[], category: Category, id: string) {
  const current = available.filter(item => item.category === category)
  const choices = current.length ? current : available.filter(item => item.category === 'search')
  // Every approved deployment has Bing; fail loudly for a broken static catalog.
  const target = choices.find(item => item.id === id) ?? choices[0]
  if (!target) throw new Error('Search catalog requires a default platform')
  return { category: target.category, target, choices }
}

export function searchURL(target: Platform, keyword: string, locale: Locale) {
  const query = keyword.trim()
  return target.buildURL ? target.buildURL(query) : target.template.replace('{locale}', locale).replace('{kw}', encodeURIComponent(query))
}
