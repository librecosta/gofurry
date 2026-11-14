import { expect, test } from '@playwright/test'

const api = process.env.CATALOG_CN_API + '/api/v1/games'
const cn = process.env.CATALOG_CN_WEB!
const intl = process.env.CATALOG_INTL_WEB!
const cnCollection = process.env.GAME_COLLECTION_CN!
const intlCollection = process.env.GAME_COLLECTION_INTL!
const atlas = '2000000000000000'
const signal = '2000000000000001'
const cartographer = '2000000000000002'

test('discovery, scoped search, stable pages, and collection views', async ({ request }) => {
  const home = await (await request.get(api + '/discovery')).json()
  expect(home.featured.map((item: { slot: string }) => item.slot)).toEqual(['primary'])
  expect(home.featured[0].game.public_id).toBe(atlas)
  expect(home.featured[0].game.cover_url).toContain('steamstatic.com/')
  expect(home.collections[0].public_id).toBe(cnCollection)
  expect(home.offers).toEqual([])
  expect(home.released.map((row: { public_id: string }) => row.public_id)).toContain(atlas)
  expect(home.upcoming.map((row: { public_id: string }) => row.public_id)).toEqual([signal, cartographer])
  expect(home.upcoming[0].release).toMatchObject({ precision: 'day', day: '2021-01-02' })
  expect(home.upcoming[0].cover_url).toBeUndefined()
  expect(home.upcoming[1].release).toMatchObject({ text: 'Q3 2027', precision: 'quarter', day: null })
  expect(home.genres.find((row: { code: string }) => row.code === 'adventure').count).toBe(2)
  const read = async (query: string) => {
    const result = await request.get(api + '/directory' + query)
    expect(result.status()).toBe(200)
    return result.json()
  }
  for (const keyword of ['Burrow Works', 'An anthropomorphic', '狐狸图册', 'Ｆｏｘ　Ａｔｌａｓ']) {
    const query = '?' + new URLSearchParams({ q: keyword, genre: 'adventure,rpg', store: 'itch_io', system: 'linux', phase: 'released' })
    const result = await read(query)
    expect(result.items.map((row: { public_id: string }) => row.public_id)).toEqual([atlas])
    expect(result.items[0].platforms.map((row: { platform_code: string }) => row.platform_code)).toEqual(['itch_io', 'steam'])
    expect(result.items[0].price_summary).toBeUndefined()
  }
  const first = await read('?q=Fox&sort=name_asc&limit=8')
  const second = await read('?q=Fox&sort=name_asc&limit=8&after=' + first.after)
  expect(first.items).toHaveLength(8)
  expect(second.items).toHaveLength(8)
  expect(new Set([...first.items, ...second.items].map(row => row.public_id)).size).toBe(16)
  expect((await request.get(api + '/directory?q=Fox&genre=rpg&sort=name_asc&limit=8&after=' + first.after)).status()).toBe(400)
  expect((await request.get(api + '/directory?locale=zh&sort=release_desc&sort=name_asc')).status()).toBe(400)
  expect((await read('?q=Blocked+Fox')).items).toEqual([])
  expect((await request.post(api + '/collections/' + cnCollection, { data: {} })).status()).toBe(405)
  const collection = await (await request.get(api + '/collections/' + cnCollection)).json()
  expect(collection.members.items.map((row: { public_id: string }) => row.public_id)).toEqual([signal, atlas])
  const time = await (await request.get(api + '/collections/' + cnCollection + '?view=timeline')).json()
  expect(time.members.items.map((row: { public_id: string }) => row.public_id)).toEqual([atlas, signal])
  const list = await (await request.get(api + '/collections?q=Fox+Volumes')).json()
  expect(list.items.map((row: { public_id: string }) => row.public_id)).toEqual([cnCollection])
})

test('CN and INTL SSR, canonical redirects, fallback, and errors', async ({ request }) => {
  for (const [base, region, language, title] of [[cn, 'cn', 'zh', '游戏'], [intl, 'intl', 'en', 'Games']]) {
    const root = await request.get(base + '/games')
    expect(root.status()).toBe(200)
    const html = await root.text()
    expect(html).toContain('<html lang="' + language + '"')
    expect(html).toContain('<title>' + title + ' · GoFurry</title>')
    expect(html).toContain('rel="canonical" href="https://' + region + '.catalog.example/games"')
    expect(html).toContain('property="og:url"')
    expect(html).toContain(region === 'cn' ? '狐狸图册' : 'Fox Atlas International')
    const redirect = await request.get(base + '/' + language + '/games', { maxRedirects: 0 })
    expect(redirect.status()).toBe(301)
    expect(redirect.headers().location).toBe('/games')
    expect((await request.get(base + '/games/collections')).status()).toBe(200)
    expect((await request.get(base + '/games/non-public-id')).status()).toBe(404)
    expect((await request.get(base + '/games/collections/0000000000000000/stale')).status()).toBe(404)
    const invalid = await request.get(base + '/games/directory?after=invalid')
    expect(invalid.status()).toBe(400)
    expect(invalid.headers()['x-robots-tag']).toBe('noindex')
    const filtered = await request.get(base + '/games/directory?q=Fox')
    expect(filtered.status()).toBe(200)
    expect(filtered.headers()['x-robots-tag']).toBe('noindex, follow')
    expect(await filtered.text()).toContain('rel="canonical" href="https://' + region + '.catalog.example/games/directory"')
  }
  const path = '/games/collections/' + cnCollection
  const moved = await request.get(cn + path + '/old-slug', { maxRedirects: 0 })
  expect(moved.status()).toBe(301)
  expect(moved.headers().location).toBe(path + '/fox-volumes')
  const fallback = await request.get(intl + '/zh/games/collections/' + intlCollection + '/fox-volumes')
  expect(fallback.status()).toBe(200)
  expect(fallback.headers()['x-robots-tag']).toBe('noindex, follow')
  expect(await fallback.text()).toContain('rel="canonical" href="https://intl.catalog.example/games/collections/' + intlCollection + '/fox-volumes"')
})

test('desktop search with IME, next page, and Back restoration', async ({ page, context }) => {
  await context.route('https://cdn.cloudflare.steamstatic.com/**', route => route.abort())
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto(cn + '/en/games/directory')
  const search = page.getByRole('searchbox', { name: 'Search games' })
  await search.fill('Fox')
  await search.press('Enter')
  await expect(page).toHaveURL(/\/en\/games\/directory\?q=Fox$/)
  await page.getByRole('link', { name: 'Next page' }).click()
  await expect(page).toHaveURL(/q=Fox&after=/)
  await expect(page.locator('article')).toHaveCount(5)
  const current = page.url()
  const link = page.locator('article h3 a').last()
  const name = await link.textContent()
  await link.scrollIntoViewIfNeeded()
  const before = await page.evaluate(() => window.scrollY)
  await link.click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(name!)
  await page.goBack()
  await expect(page).toHaveURL(current)
  await expect(page.locator('article h3 a').last()).toHaveText(name!)
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThanOrEqual(Math.max(0, before - 16))
  await page.getByRole('link', { name: '中文' }).click()
  await expect(page).toHaveURL(/\/games\/directory\?q=Fox$/)
  const chinese = page.getByRole('searchbox', { name: '搜索游戏' })
  await chinese.focus()
  await chinese.dispatchEvent('compositionstart', { data: '' })
  await chinese.fill('狐狸')
  await page.waitForTimeout(380)
  await expect(page).toHaveURL(/q=Fox$/)
  await chinese.dispatchEvent('compositionend', { data: '狐狸' })
  await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBe('狐狸')
  await expect(page.getByRole('heading', { name: '狐狸图册' })).toBeVisible()
  await expect(page.locator('article')).toHaveCount(1)
})

test('touch, dark, reduced motion, cover fallback and Start menu at 390px', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 820 }, hasTouch: true, isMobile: true,
    colorScheme: 'dark', reducedMotion: 'reduce' })
  await context.route('https://cdn.cloudflare.steamstatic.com/**', route => route.abort())
  const page = await context.newPage()
  try {
    await page.goto(cn + '/start')
    await page.locator('header button:has([data-start-logo])').tap()
    await page.locator('[data-start-brand]').getByRole('link', { name: '游戏', exact: true }).tap()
    await expect(page).toHaveURL(/\/games$/)
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await expect(page.getByRole('heading', { name: '编辑精选' })).toBeVisible()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBe(390)
    await expect(page.locator('article img')).toHaveCount(0)
    await expect(page.locator('article').first()).toHaveCSS('transition-duration', '0s')
    await page.getByRole('link', { name: '狐狸故事集' }).tap()
    await page.getByRole('link', { name: '发行时间线' }).tap()
    await expect(page).toHaveURL(/view=timeline$/)
    await expect(page.getByRole('heading', { name: '已发售', exact: true })).toBeVisible()
  } finally {
    await context.close()
  }
})
