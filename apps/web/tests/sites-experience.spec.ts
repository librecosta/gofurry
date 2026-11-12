import { expect, test, type BrowserContext } from '@playwright/test'
import { readFile, mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import { categoryCodes } from '../src/navigation/contracts'
import { categories } from '../src/navigation/messages'

const cn = process.env.CATALOG_CN_WEB!
const intl = process.env.CATALOG_INTL_WEB!
const edge = process.env.CATALOG_CN_EDGE!
const api = process.env.CATALOG_CN_API!
const assetHost = 'https://wallpapers.catalog.example'
const id = '0000000000001001'
const asset = (file: string) => readFile(new URL('../../../services/api/internal/storage/testdata/' + file, import.meta.url))

async function assets(context: BrowserContext, failed = false) {
  await context.route(assetHost + '/sites/**', async route => {
    if (failed) return route.fulfill({ status: 503 })
    const logo = route.request().url().endsWith('.webp')
    await route.fulfill({ contentType: logo ? 'image/webp' : 'image/avif', body: await asset(logo ? 'site-logo.webp' : 'site-cover.avif') })
  })
}

test('regional SSR, canonical URLs, fixed editorial layout and projected data', async ({ request }) => {
  const routes = [
    [cn, '/sites', 'zh', '网站发现', 'cn'], [cn, '/en/sites', 'en', 'Discover furry sites', 'cn'],
    [intl, '/sites', 'en', 'Discover furry sites', 'intl'], [intl, '/zh/sites', 'zh', '网站发现', 'intl'],
    [cn, '/sites/directory', 'zh', '网站目录', 'cn'], [cn, '/en/sites/directory', 'en', 'Site directory', 'cn'],
    [intl, '/sites/directory', 'en', 'Site directory', 'intl'], [intl, '/zh/sites/directory', 'zh', '网站目录', 'intl'],
  ] as const
  for (const [base, path, lang, title, region] of routes) {
    const response = await request.get(base + path)
    expect(response.status()).toBe(200)
    expect(response.headers()['cache-control']).toBe('public, max-age=0, must-revalidate')
    expect(response.headers()['x-robots-tag']).toBeUndefined()
    const html = await response.text()
    expect(html).toContain('<html lang="' + lang + '"')
    expect(html).toContain('<title>' + title + ' · GoFurry</title>')
    expect(html).toContain('href="https://' + region + '.catalog.example' + path + '"')
    expect(html).toMatch(/hrefLang="(?:en|zh)"/i)
    expect(html).toContain('data-site-id=')
    expect(html).not.toMatch(/Restricted specimen|Archived specimen|Runtime skeleton/)
  }
  const alias = await request.get(cn + '/zh/sites/directory?tags=tutorial,forum&category=resources,gallery', { maxRedirects: 0 })
  expect(alias.status()).toBe(301)
  expect(alias.headers().location).toBe('/sites/directory?category=gallery%2Cresources&tags=forum%2Ctutorial')
  expect((await request.get(intl + '/en/sites', { maxRedirects: 0 })).headers().location).toBe('/sites')
  expect((await request.get(cn + '/fr/sites', { maxRedirects: 0 })).status()).toBe(404)
  expect((await request.get(cn + '/sites/not-a-public-id', { maxRedirects: 0 })).status()).toBe(404)
  const filtered = await request.get(cn + '/en/sites/directory?q=absent%25_')
  expect(filtered.status()).toBe(200)
  expect(filtered.headers()['x-robots-tag']).toBe('noindex, follow')
  expect(await filtered.text()).toContain('No sites match these filters.')
  const start = performance.now()
  const homeResponse = await request.get(api + '/api/v1/sites/discovery')
  const homeBytes = (await homeResponse.body()).length
  const home = await homeResponse.json()
  expect(home.categories.map((term: { code: typeof categoryCodes[number]; zh: string; en: string }) => {
    expect(term.zh).toBe(categories[term.code].zh)
    expect(term.en).toBe(categories[term.code].en)
    return term.code
  })).toEqual(categoryCodes)
  expect(home.featured.primary.logo_url).toMatch(/\/sites\/logos\/[a-f0-9]{64}\.webp$/)
  expect(home.featured.primary.cover_url).toMatch(/\/sites\/covers\/[a-f0-9]{64}\.avif$/)
  expect(home.featured.primary.visit_url).toBe('https://field-guide.example/entry/1?view=guide')
  expect(home.featured.secondary_1.public_id).toBe('0000000000001002')
  expect(home.featured.secondary_2.public_id).toBe('0000000000001003')
  expect(home.trending.map((item: { rank: number }) => item.rank)).toEqual([2, 3, 4, 5, 6, 7, 8, 9])
  expect(home.recent).toHaveLength(8)
  expect(home.random).not.toBeNull()
  expect(home.recent.every((site: { cover_url?: string }) => !site.cover_url)).toBeTruthy()
  expect(JSON.stringify(home)).not.toMatch(/asset_key|user:fixture|Restricted specimen|Archived specimen/)
  const directoryResponse = await request.get(api + '/api/v1/sites/directory?locale=en')
  const directoryBytes = (await directoryResponse.body()).length
  const directory = await directoryResponse.json()
  expect(directory.items).toHaveLength(24)
  expect(directory.language_registry).toEqual(['en', 'ja', 'zh'])
  expect(directory.items.some((item: { cover_url?: string }) => item.cover_url)).toBeFalsy()
  expect(directory.items.find((item: { public_id: string }) => item.public_id === '0000000000001010')?.visit_url).toBeUndefined()
  console.log(`Sites payload: discovery ${homeBytes} B, directory ${directoryBytes} B, two local API reads ${(performance.now() - start).toFixed(1)} ms`)
})

test('discovery links, reviewed images, safe native visit and locale fallback', async ({ page, context }) => {
  await assets(context)
  await context.route('https://field-guide.example/**', route => route.fulfill({ body: 'Destination fixture' }))
  await page.route('**/interactions', route => route.fulfill({ status: 503 }))
  await page.goto(edge + '/sites')
  await expect(page.getByRole('heading', { name: '编辑精选' })).toBeVisible()
  const editor = page.getByRole('region', { name: '编辑精选' })
  await expect(editor.getByRole('link', { name: 'Lantern Sketchbook', exact: true })).toHaveAttribute('lang', 'en')
  const primary = editor.locator('[data-site-id="' + id + '"]')
  await expect(primary.getByRole('link', { name: '松林兽圈', exact: true })).toHaveAttribute('href', '/sites/' + id + '/field-guide-1')
  await expect(primary.locator('img[fetchpriority="high"]')).toHaveAttribute('loading', 'eager')
  await expect(primary.locator('img[src$=".webp"]')).toHaveAttribute('loading', 'lazy')
  await expect.poll(() => primary.locator('img[src$=".avif"]').evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(960)
  const visit = primary.getByRole('link', { name: /^访问网站/ })
  await expect(visit).toHaveAttribute('rel', 'noopener noreferrer')
  const sent = page.waitForRequest(request => request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/' + id + '/interactions'))
  const tab = context.waitForEvent('page')
  await visit.focus()
  await expect(visit).toBeFocused()
  await visit.press('Enter')
  const destination = await tab
  await expect(destination.locator('body')).toHaveText('Destination fixture')
  expect(await destination.evaluate(() => window.opener)).toBeNull()
  expect((await sent).method()).toBe('POST')
  await destination.close()
  await page.getByRole('link', { name: /作品展示/ }).click()
  await expect(page).toHaveURL(/\/sites\/directory\?category=gallery$/)
  await expect(page.getByRole('button', { name: '作品展示', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await page.goto(intl + '/sites/directory?sort=heat_7d')
  await expect(page.getByRole('status').filter({ hasText: 'Weekly heat is unavailable.' })).toBeVisible()
})

test('directory shares filters, preserves history and appends each Public ID once', async ({ page, context }) => {
  await assets(context)
  await page.goto(edge + '/en/sites/directory')
  const cards = page.locator('[data-site-id]')
  await expect(cards).toHaveCount(24)
  const first = await cards.first().getAttribute('data-site-id')
  await page.getByRole('button', { name: 'Load more' }).click()
  await expect.poll(() => cards.count()).toBeGreaterThan(24)
  const ids = await cards.evaluateAll(nodes => nodes.map(node => node.getAttribute('data-site-id')))
  expect(ids[0]).toBe(first)
  expect(new Set(ids).size).toBe(ids.length)
  const filter = page.getByRole('button', { name: 'Filters', exact: true })
  await filter.click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Communities', { exact: true }).check()
  await dialog.getByLabel('Art & Galleries', { exact: true }).check()
  await dialog.getByLabel('Portfolio', { exact: true }).check()
  await dialog.getByLabel('English', { exact: true }).check()
  await expect(page).toHaveURL(/\/en\/sites\/directory$/)
  await dialog.getByRole('button', { name: 'Apply filters' }).click()
  await expect(dialog).toBeHidden()
  await expect(page).toHaveURL(/\?category=community%2Cgallery&tags=portfolio&language=en$/)
  const shared = page.url()
  await expect(page.getByRole('button', { name: 'Remove Portfolio' })).toBeVisible()
  await expect(cards.first()).toBeVisible()
  await expect.poll(() => cards.count()).toBeLessThan(24)
  await page.reload()
  await expect(page.getByRole('button', { name: 'Remove Portfolio' })).toBeVisible()
  await page.getByRole('button', { name: 'Filters (4)' }).click()
  await dialog.getByLabel('Portfolio', { exact: true }).uncheck()
  await dialog.press('Escape')
  await expect(page.getByRole('button', { name: 'Filters (4)' })).toBeFocused()
  await expect(page).toHaveURL(shared)
  await page.getByRole('button', { name: 'Clear filters' }).click()
  await expect(page).toHaveURL(/\/en\/sites\/directory$/)
  await page.goBack()
  await expect(page).toHaveURL(shared)
  await expect(page.getByRole('button', { name: 'Remove Portfolio' })).toBeVisible()
  await page.goForward()
  await expect(cards).toHaveCount(24)
})

test('IME debounce, page failure, stale refresh and typing cancellation', async ({ page, context }) => {
  await assets(context)
  await page.goto(edge + '/en/sites/directory')
  const field = page.getByRole('searchbox', { name: 'Search sites' })
  const cards = page.locator('[data-site-id]')
  await field.focus()
  await field.dispatchEvent('compositionstart', { data: '' })
  await field.fill('原野')
  await field.press('Enter')
  await page.waitForTimeout(380)
  await expect(page).toHaveURL(/\/en\/sites\/directory$/)
  await field.dispatchEvent('compositionend', { data: '原野' })
  await expect(page).toHaveURL(/\?q=%E5%8E%9F%E9%87%8E$/)
  await field.fill('Field Guide')
  await field.press('Enter')
  await expect(page).toHaveURL(/\?q=Field\+Guide$/)
  await expect(cards).toHaveCount(24)
  const paging = '**/api/v1/sites/directory?**cursor=*'
  await page.route(paging, route => route.fulfill({ status: 503 }))
  await page.getByRole('button', { name: 'Load more' }).click()
  await expect(page.getByText('More sites could not be loaded.')).toBeVisible()
  await expect(cards).toHaveCount(24)
  await page.unroute(paging)
  await page.route(paging, route => route.fulfill({ status: 400 }))
  await page.getByRole('button', { name: 'Retry', exact: true }).click()
  await expect(page.getByText('The directory has changed.')).toBeVisible()
  await page.getByRole('button', { name: 'Refresh results' }).click()
  await expect(page).toHaveURL(/\?q=Field\+Guide$/)
  await expect(cards).toHaveCount(24)
  await page.unroute(paging)

  let finish: (() => void) | undefined
  let started: (() => void) | undefined
  const pending = new Promise<void>(resolve => { started = resolve })
  await page.route(paging, async route => {
    started?.()
    await new Promise<void>(resolve => { finish = resolve })
    await route.continue().catch(() => undefined)
  })
  await page.getByRole('button', { name: 'Load more' }).click()
  await pending
  await field.fill('no such fixture')
  await field.press('Enter')
  finish?.()
  await expect(page).toHaveURL(/\?q=no\+such\+fixture$/)
  await expect(page.getByText('No sites match these filters.')).toBeVisible()
  await expect(cards).toHaveCount(0)
})

test('responsive surfaces, failed images and measured first load', async ({ page, context }) => {
  const folder = process.env.SITES_SCREENSHOTS
  if (folder) await mkdir(folder, { recursive: true })
  await assets(context)
  const overflow = async () => page.evaluate(() => document.documentElement.scrollWidth > innerWidth)
  await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
  await page.setViewportSize({ width: 1440, height: 960 })
  const response = await page.goto(edge + '/en/sites')
  expect(response?.status()).toBe(200)
  await expect(page.locator('img[fetchpriority="high"]')).toBeVisible()
  await page.locator('img[fetchpriority="high"]').evaluate((img: HTMLImageElement) => img.decode())
  const metrics = await page.evaluate(() => {
    const navigation = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming
    return { document: navigation.decodedBodySize, firstByteMS: Math.round(navigation.responseStart),
      domContentLoadedMS: Math.round(navigation.domContentLoadedEventEnd), coverImages: document.querySelectorAll('img[src$=".avif"]').length }
  })
  expect(metrics.coverImages).toBe(1)
  console.log('Sites browser first load ' + JSON.stringify(metrics) + ', logo ' + (await asset('site-logo.webp')).length + ' B, cover ' + (await asset('site-cover.avif')).length + ' B')
  if (folder) await page.screenshot({ path: join(folder, 'home-desktop.jpg'), fullPage: true, type: 'jpeg', quality: 79 })
  for (const width of [1440, 1280, 1024, 768, 390, 320]) {
    await page.setViewportSize({ width, height: 850 })
    expect(await overflow()).toBeFalsy()
    await expect(page.getByRole('region', { name: 'Explore by category' }).getByRole('listitem')).toHaveCount(8)
  }
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.setViewportSize({ width: 390, height: 850 })
  await page.goto(edge + '/en/sites/directory?category=community')
  expect(await overflow()).toBeFalsy()
  await expect(page.locator('img[src$=".avif"]')).toHaveCount(0)
  expect(await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)
  if (folder) await page.screenshot({ path: join(folder, 'directory-mobile-dark.jpg'), type: 'jpeg', quality: 80 })
  await page.getByRole('button', { name: 'Filters (1)' }).click()
  const sheet = page.getByRole('dialog')
  await expect(sheet).toBeVisible()
  const bounds = await sheet.boundingBox()
  expect(bounds && Math.abs(bounds.y + bounds.height - 850)).toBeLessThan(2)
  await expect(sheet.getByLabel('Communities', { exact: true })).toBeChecked()
  await sheet.getByRole('combobox', { name: 'Sort' }).selectOption('heat_7d')
  if (folder) await page.screenshot({ path: join(folder, 'directory-filter-sheet.jpg'), type: 'jpeg', quality: 80 })
  await sheet.getByRole('button', { name: 'Cancel' }).click()
  await expect(page.getByRole('button', { name: 'Filters (1)' })).toBeFocused()
  await expect(page).toHaveURL(/\?category=community$/)
  await context.unroute(assetHost + '/sites/**')
  await assets(context, true)
  await page.goto(edge + '/en/sites')
  await expect(page.locator('img[src$=".avif"]')).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Pinewood Commons', exact: true })).toBeVisible()
  await page.setViewportSize({ width: 640, height: 480 })
  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  expect(await overflow()).toBeFalsy()
})
