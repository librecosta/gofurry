import { expect, test } from '@playwright/test'
import { mkdir, readFile } from 'node:fs/promises'
import { join } from 'node:path'

const cn = process.env.CATALOG_CN_WEB!
const intl = process.env.CATALOG_INTL_WEB!
const edge = process.env.CATALOG_CN_EDGE!
const api = process.env.CATALOG_CN_API!
const intlAPI = process.env.CATALOG_INTL_API!
const id = '0000000000001001'
const path = '/sites/' + id + '/field-guide-1'
const lantern = '/sites/0000000000001002/field-guide-2'
const legacy = '/sites/0000000000001027/field-guide-39'
const asset = 'https://wallpapers.catalog.example/sites/logos/**'

test('public Site API preserves identity and isolates facts, visibility and ranked matches', async ({ request }) => {
  const key = '/api/v1/sites/' + id
  const response = await request.get(api + key)
  expect(response.status()).toBe(200)
  expect(response.headers()['x-gofurry-interactions']).toBe('enabled')
  const text = await response.text()
  const profile = JSON.parse(text)
  expect(profile).toMatchObject({ reviewed: true, primary_category: 'community', secondary_categories: ['knowledge'], supported_languages: ['en', 'zh'] })
  expect(profile.addresses.map((address: { kind: string }) => address.kind)).toEqual(['primary', 'alternate'])
  expect(profile.logo_url).toMatch(/\/sites\/logos\/[a-f0-9]{64}\.webp$/)
  expect(profile.technology).toEqual(expect.arrayContaining([
    expect.objectContaining({ kind: 'http', http_status: 200, source: 'field-guide.example', latest_confirmed_at: expect.any(String) }),
    expect.objectContaining({ kind: 'fingerprint', technology: 'astro', first_observed_at: expect.any(String), ruleset: 'web-passive-1' }),
    expect.objectContaining({ kind: 'tls', ruleset: 'gofurry-evidence-1' }),
  ]))
  expect(text).not.toMatch(/PRIVATE-FIXTURE|rdap|certificate_sha|asset_key|target_id|other\.example/)
  const related = async (base: string) => {
    const response = await request.get(base + key + '/related')
    expect(response.status()).toBe(200)
    return (await response.json()).items.map((item: { public_id: string; primary_category: string }) => item)
  }
  const matches = await related(api)
  expect(matches).toHaveLength(6)
  expect(matches.slice(0, 4).map((item: { primary_category: string }) => item.primary_category)).toEqual(Array(4).fill('community'))
  const ids = matches.map((item: { public_id: string }) => item.public_id)
  expect(new Set(ids).size).toBe(6)
  expect(ids).not.toContain(id)
  expect(ids).not.toContain('0000000000001027')
  expect(ids).not.toContain('0000000000001028')
  expect(await related(api)).toEqual(matches)
  const independent = await (await request.get(intlAPI + key)).json()
  expect(independent.localizations[0].name).toBe('Cedar Commons')
  expect(independent.technology).toBeUndefined()
  const stats = await (await request.get(api + key + '/stats')).json()
  expect(stats.detail_views).toBeGreaterThanOrEqual(78)
  expect(stats.outbound_clicks).toBeGreaterThanOrEqual(4)
  const low = await (await request.get(api + '/api/v1/sites/0000000000001002/stats')).json()
  expect(low.detail_views).toBeUndefined()
  expect(low.outbound_clicks).toBeUndefined()
  const legacyAPI = '/api/v1/sites/0000000000001027'
  const old = await request.get(api + legacyAPI)
  expect(old.status()).toBe(200)
  expect(await old.json()).toMatchObject({ reviewed: false, addresses: [] })
  for (const suffix of ['/related', '/stats']) expect((await request.get(api + legacyAPI + suffix)).status()).toBe(404)
  expect((await request.get(intlAPI + legacyAPI)).status()).toBe(404)
  expect((await request.post(api + key, { data: { body: 'No public editing' } })).status()).toBe(405)
})

test('SSR uses real translations, canonical status and the legacy noindex gate', async ({ request }) => {
  for (const [base, prefix, lang, name, origin] of [
    [cn, '', 'zh', '松林兽圈', 'cn'], [cn, '/en', 'en', 'Pinewood Commons', 'cn'],
    [intl, '', 'en', 'Cedar Commons', 'intl'], [intl, '/zh', 'zh', '松林兽圈', 'intl'],
  ]) {
    const response = await request.get(base + prefix + path)
    const html = await response.text()
    expect(response.status()).toBe(200)
    expect(response.headers()['x-robots-tag']).toBeUndefined()
    expect(html).toContain('<title>' + name + '</title>')
    expect(html).toContain('<html lang="' + lang + '"')
    expect(html).toContain('rel="canonical" href="https://' + origin + '.catalog.example' + prefix + path + '"')
    expect(html).not.toMatch(/PRIVATE-FIXTURE|Runtime skeleton|Restricted specimen|Archived specimen/)
  }
  const missing = await request.get(cn + lantern)
  expect(missing.status()).toBe(200)
  expect(missing.headers()['x-robots-tag']).toBe('noindex')
  expect(await missing.text()).toMatch(/<h1 lang="en">Lantern Sketchbook<\/h1>/)
  expect(await missing.text()).not.toMatch(/name="description"|hreflang="zh"|<h2[^>]*>近期关注/)
  const old = await request.get(cn + '/en' + legacy)
  expect(old.status()).toBe(200)
  expect(old.headers()['x-robots-tag']).toBe('noindex')
  expect(await old.text()).not.toMatch(/PRIVATE-LEGACY-BODY|Related sites|Technology overview|Recent activity|Visit website/)
  expect((await request.get(intl + legacy)).status()).toBe(404)
  const alias = await request.get(cn + '/en/sites/' + id + '/wrong-slug?next=ignore', { maxRedirects: 0 })
  expect(alias.status()).toBe(301)
  expect(alias.headers().location).toBe('/en' + path)
  expect((await request.get(cn + '/fr' + path)).status()).toBe(404)
})

test('directory history and Start menu preserve the navigation path, with native outbound on failure', async ({ page, context }) => {
  await context.route(asset, route => route.fulfill({ status: 503 }))
  await context.route('https://mirror.field-guide.example/**', route => route.fulfill({ body: 'Alternate destination fixture' }))
  const events: string[] = []
  await page.route('**/interactions', route => {
    events.push(route.request().postDataJSON().type)
    return route.fulfill({ status: 503 })
  })
  const filtered = '/en/sites/directory?tags=forum'
  await page.goto(edge + filtered)
  const source = page.locator('article[data-site-id="0000000000001019"]').getByRole('link', { name: 'Field Guide 25', exact: true })
  await source.scrollIntoViewIfNeeded()
  const scroll = await page.evaluate(() => scrollY)
  expect(scroll).toBeGreaterThan(50)
  await page.evaluate(() => { document.body.dataset.navigationFixture = 'retained' })
  await source.click()
  await expect(page.getByRole('heading', { level: 1, name: 'Field Guide 25' })).toBeVisible()
  await expect(page.locator('body')).toHaveAttribute('data-navigation-fixture', 'retained')
  await page.goBack()
  await expect(page).toHaveURL(edge + filtered)
  await expect(page.getByRole('button', { name: 'Remove Forum' })).toBeVisible()
  await expect.poll(() => page.evaluate(() => scrollY)).toBeCloseTo(scroll, 0)
  await page.goto(edge + '/en/start')
  await page.getByRole('button', { name: 'GoFurry brand' }).press('Enter')
  const menu = page.locator('[data-start-brand]')
  const sites = menu.getByRole('link', { name: 'Sites', exact: true })
  await expect(sites).toHaveAttribute('href', '/en/sites')
  await expect(menu.getByRole('button', { name: 'Games' })).toHaveAttribute('aria-disabled', 'true')
  await sites.focus()
  await sites.press('Enter')
  await expect(page).toHaveURL(edge + '/en/sites')
  await expect(menu).toHaveCount(0)
  await page.getByRole('link', { name: 'Pinewood Commons', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Pinewood Commons' })).toBeVisible()
  await page.getByText('Alternate addresses (1)', { exact: true }).click()
  const alternate = page.getByRole('link', { name: 'mirror.field-guide.example', exact: true })
  await expect(alternate).toHaveAttribute('href', 'https://mirror.field-guide.example/browse?view=community')
  await expect(alternate).toHaveAttribute('rel', 'noopener noreferrer')
  const opened = context.waitForEvent('page')
  await alternate.press('Enter')
  const tab = await opened
  await expect(tab.locator('body')).toHaveText('Alternate destination fixture')
  await tab.close()
  await expect.poll(() => events.filter(event => event === 'outbound_click').length).toBe(1)
  const info = page.getByRole('region', { name: 'Site information' })
  await expect(info.getByRole('link', { name: 'News & Knowledge' })).toHaveAttribute('href', '/en/sites/directory?category=knowledge')
  await expect(info.getByRole('link', { name: 'Wiki' })).toHaveAttribute('href', '/en/sites/directory?tags=wiki')
})

test('profile renders readable columns, localized text and last-confirmed evidence in both themes', async ({ page, context }) => {
  const folder = process.env.SITE_PROFILE_SCREENSHOTS
  if (folder) await mkdir(folder, { recursive: true })
  await context.route(asset, async route => route.fulfill({
    contentType: 'image/webp',
    body: await readFile(new URL('../../../services/api/internal/storage/testdata/site-logo.webp', import.meta.url)),
  }))
  await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
  await page.setViewportSize({ width: 1440, height: 920 })
  await page.goto(edge + '/en' + path)
  await expect(page.getByRole('region', { name: 'About this site' }).locator('p')).toHaveCount(2)
  await expect(page.getByRole('region', { name: 'Recent activity' })).toContainText('Qualified detail views')
  const tech = page.getByRole('region', { name: 'Technology overview' })
  await expect(tech).toContainText('Last confirmed by Harvest')
  await expect(tech).toContainText('First recorded')
  await expect(tech.getByText('Astro', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Visit website', exact: true })).toHaveAttribute('href', 'https://field-guide.example/entry/1?view=guide')
  await expect(page.locator('article > header img')).toBeVisible()
  await page.locator('article > header img').evaluate((img: HTMLImageElement) => img.decode())
  const about = page.getByRole('region', { name: 'About this site' })
  const related = page.getByRole('region', { name: 'Related sites' })
  expect((await related.boundingBox())!.x).toBeGreaterThan((await about.boundingBox())!.x + (await about.boundingBox())!.width)
  if (folder) await page.screenshot({ path: join(folder, 'profile-desktop.jpg'), fullPage: true, type: 'jpeg', quality: 82 })
  const overflow = () => page.evaluate(() => document.documentElement.scrollWidth > innerWidth)
  for (const width of [1440, 1280, 1024, 768, 390, 320]) {
    await page.setViewportSize({ width, height: 850 })
    expect(await overflow()).toBe(false)
  }
  expect((await related.boundingBox())!.y).toBeGreaterThan((await about.boundingBox())!.y + (await about.boundingBox())!.height)
  await page.setViewportSize({ width: 390, height: 850 })
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.getByRole('link', { name: '简体中文', exact: true }).click()
  await expect(page.getByRole('heading', { name: '松林兽圈', level: 1 })).toBeVisible()
  await expect(page.getByRole('region', { name: '关于网站' })).toContainText('成员会分享活动计划')
  expect(await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)
  if (folder) await page.screenshot({ path: join(folder, 'profile-mobile-dark.jpg'), fullPage: true, type: 'jpeg', quality: 82 })
  await page.setViewportSize({ width: 640, height: 480 })
  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  expect(await overflow()).toBe(false)
})
