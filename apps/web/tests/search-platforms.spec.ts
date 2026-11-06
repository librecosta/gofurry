import { test, expect } from '@playwright/test'
import { platforms, regionalPlatforms, availablePlatforms, resolveSelection, searchURL, categories } from '../src/start/search-platforms'
import { encodeFalooKeyword } from '../src/start/search-encoding'

test('catalog: exact approved regional membership/order, unique IDs and no removed entries', () => {
  const shared = { games: ['steam', 'itchio', 'epic', 'roblox', 'gog', 'gamejolt'], art: ['pixiv', 'deviantart', 'artstation', 'pinterest', 'zcool', 'booth', 'sketchfab'] }
  const expected = {
    CN: { search: ['bing', '360', 'sohu', 'baidu', 'bilibili', 'xiaohongshu', 'zhihu', 'weibo'], furry: ['wikifur', 'furaffinity', 'wilddream', 'furrynovel', 'linpx'], ...shared, novel: ['qidian', 'fanqie', 'qimao', 'faloo', 'sfacg'] },
    INTL: { search: ['bing', 'google', 'duckduckgo', 'youtube', 'x', 'bluesky', 'reddit'], furry: ['wikifur', 'furaffinity', 'e621', 'yiffparty', 'wilddream', 'furrynovel', 'linpx'], ...shared, novel: ['wattpad', 'royalroad', 'ao3', 'webnovel', 'scribblehub', 'fanfiction'] },
  }
  expect(new Set(platforms.map(p => p.id)).size).toBe(platforms.length)
  for (const region of ['CN', 'INTL'] as const) {
    const catalog = regionalPlatforms(region)
    expect(catalog).toHaveLength(region === 'CN' ? 31 : 33)
    for (const category of categories) expect(catalog.filter(p => p.category === category).map(p => p.id)).toEqual(expected[region][category])
    expect(availablePlatforms(region, 'SFW', true).some(p => p.nsfw)).toBe(false)
    expect(availablePlatforms(region, 'NSFW', true).filter(p => p.nsfw).map(p => p.id)).toEqual(region === 'CN' ? [] : ['e621', 'yiffparty'])
    for (const content of ['SFW', 'NSFW'] as const) expect(availablePlatforms(region, content, false).some(p => p.category === 'furry')).toBe(false)
  }
})

test('catalog: every region/content/furry transition resolves a valid target, preserving valid IDs', () => {
  for (const region of ['CN', 'INTL'] as const) for (const content of ['SFW', 'NSFW'] as const) for (const furry of [true, false]) {
    const visible = availablePlatforms(region, content, furry)
    for (const previous of [...platforms, { category: 'search' as const, id: 'obsolete-id' }]) {
      const next = resolveSelection(visible, previous.category, previous.id)
      expect(visible).toContain(next.target)
      expect(next.choices).toContain(next.target)
      if (visible.some(p => p.id === previous.id)) expect(next.target.id).toBe(previous.id)
    }
    expect(resolveSelection(visible, 'search', 'unknown').target.id).toBe('bing')
    if (!furry) expect(resolveSelection(visible, 'furry', 'e621').target.id).toBe('bing')
    if (region === 'CN' || content === 'SFW') expect(resolveSelection(visible, 'furry', 'e621').target.id).toBe(furry ? 'wikifur' : 'bing')
  }
})

test('catalog: authoritative ten templates preserve only the required query parameters', () => {
  const expected = {
    sohu: 'https://search.sohu.com/?keyword={kw}', wilddream: 'https://www.wilddream.net/Art/index/index?keyword={kw}', linpx: 'https://linpx.ink/search?word={kw}',
    yiffparty: 'https://yiff-party.com/search/?q={kw}', epic: 'https://store.epicgames.com/browse?q={kw}', gog: 'https://www.gog.com/en/games?query={kw}',
    fanqie: 'https://fanqienovel.com/search/{kw}', qimao: 'https://www.qimao.com/search/index/?keyword={kw}', sfacg: 'https://s.sfacg.com/?Key={kw}&S=1&SS=0',
  }
  for (const query of ['  兽人 小说  ', 'Fox & Dragon? #1/100% + $']) {
    for (const [id, template] of Object.entries(expected)) expect(searchURL(platforms.find(p => p.id === id)!, query, 'zh')).toBe(template.replace('{kw}', encodeURIComponent(query.trim())))
    expect(searchURL(platforms.find(p => p.id === 'faloo')!, query, 'en')).toBe(`https://b.faloo.com/l/0/1.html?t=1&k=${encodeFalooKeyword(query.trim())}`)
  }
})

test('catalog: Faloo matches official UTF-16 legacy escape including Chinese, Latin-1 and surrogate pairs', () => {
  expect(encodeFalooKeyword('兽人 小说')).toBe('%u517D%u4EBA%20%u5C0F%u8BF4')
  expect(encodeFalooKeyword('Fox & 中文?/#%=é🦊')).toBe('Fox%20%26%20%u4E2D%u6587%3F/%23%25%3D%E9%uD83E%uDD8A')
  expect(encodeFalooKeyword('@*_+-./')).toBe('@*_+-./')
  expect(encodeFalooKeyword('兽人')).not.toBe(encodeURIComponent('兽人'))
})

test('catalog: HTTPS construction retains an encoded keyword and cannot inject URL credentials, query fields or fragments', () => {
  for (const p of platforms) for (const locale of ['zh', 'en'] as const) for (const keyword of ['兽人 fox 🦊', 'https://evil.example/?next=x&injected=1#fragment']) {
    const value = searchURL(p, keyword, locale), url = new URL(value)
    expect(url.protocol).toBe('https:'); expect(url.username).toBe(''); expect(url.password).toBe(''); expect(url.hash).toBe('')
    expect(url.searchParams.has('injected')).toBe(false)
    expect(value).toContain(p.id === 'faloo' ? encodeFalooKeyword(keyword) : encodeURIComponent(keyword))
    expect(value).not.toMatch(/\{(?:kw|locale)\}/)
  }
})
