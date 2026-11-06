import { test, expect, type Page } from '@playwright/test'

const region = process.env.WEB_REGION ?? 'CN', other = region === 'CN' ? 'INTL' : 'CN'
const english = region === 'CN' ? '/en/start' : '/start', chinese = region === 'CN' ? '/start' : '/zh/start'
const cookie = `gf-furry-search-${region}`
const field = (page: Page) => page.locator('input[name=q]')
const row = (page: Page) => page.locator('[data-desktop-platforms]')
const category = (page: Page, name: string) => page.locator('[data-categories]').getByRole('button', { name, exact: true, includeHidden: true })
const dialog = (page: Page) => page.locator('[data-start-preferences]')
async function settings(page: Page) { await page.locator('[data-start-settings]').click(); await expect(dialog(page)).toHaveCSS('opacity', '1') }
async function close(page: Page) { await page.keyboard.press('Escape'); await expect(dialog(page)).toHaveCount(0) }
test.use({ viewport: { width: 1280, height: 720 } })

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/start/backgrounds/**', route => route.fulfill({ status: 503, json: {} }))
  await page.route('**/api/v1/start/weather?*', route => route.fulfill({ status: 503, json: {} }))
  await page.route('**/api/v1/start/search/suggestions?*', route => route.fulfill({ headers: { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' }, json: { suggestions: ['furry art', 'furry games'] } }))
  await page.addInitScript(() => {
    const opened: { url: string; target?: string; features?: string }[] = []
    Object.assign(window, { opened })
    window.open = (url, target, features) => { opened.push({ url: String(url), target, features }); return null }
  })
})

test('catalog UI: furry preference resets safely, preserves keyword/NSFW and survives locale/reload', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (/hydrat|did not match/i.test(message.text())) errors.push(message.text()) })
  await page.goto(english)
  await category(page, 'Furry').click()
  await expect(row(page).getByRole('button')).toHaveCount(5)
  await row(page).getByRole('button', { name: 'Linpx', exact: true }).click()
  await field(page).fill('保留 furry'); await settings(page)
  const toggle = page.getByRole('switch', { name: 'Show furry search options', exact: true })
  await expect(toggle).toBeChecked(); await toggle.click()
  await expect(category(page, 'Furry')).toHaveCount(0)
  await expect(field(page)).toHaveValue('保留 furry')
  await expect(row(page).locator('[aria-pressed=true]')).toHaveText('Bing')
  expect(await page.context().cookies().then(c => c.find(c => c.name === cookie)?.value)).toBe('0')
  await page.screenshot({ path: info.outputPath(`${region}-settings.png`) })
  await close(page); await field(page).press('Enter')
  const opened = await page.evaluate(() => (window as unknown as { opened: { url: string; target: string; features: string }[] }).opened)
  expect(opened).toHaveLength(1); expect(new URL(opened[0].url).hostname).toBe('www.bing.com')
  expect(new URL(opened[0].url).searchParams.get('q')).toBe('保留 furry')
  expect(opened[0].target).toBe('_blank'); expect(opened[0].features).toBe('noopener,noreferrer')
  await settings(page); await page.getByRole('radio', { name: '简体中文', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(chinese + '$')); await expect(field(page)).toHaveValue('保留 furry')
  await expect(page.locator('[data-categories]')).not.toContainText('兽圈')
  await expect(page.getByRole('switch', { name: '显示兽圈相关搜索选项' })).not.toBeChecked()
  await close(page); await page.reload(); await expect(page.locator('[data-categories]')).not.toContainText('兽圈')
  await settings(page); await page.getByRole('switch', { name: '显示兽圈相关搜索选项' }).click(); await close(page)
  await expect(category(page, '兽圈')).toBeVisible(); await expect(page.locator('html')).toHaveAttribute('data-region', region)
  expect(errors).toEqual([])
})

test('catalog UI: default SFW, INTL-only adult links and immediate valid fallback', async ({ page }) => {
  await page.context().addCookies([{ name: 'gf-content-preference', value: 'NSFW', url: test.info().project.use.baseURL! }])
  await page.goto(english); await category(page, 'Furry').click()
  await expect(row(page).getByRole('button', { name: 'e621', exact: true })).toHaveCount(region === 'INTL' ? 1 : 0)
  await expect(row(page).getByRole('button', { name: 'YiffParty', exact: true })).toHaveCount(region === 'INTL' ? 1 : 0)
  if (region === 'INTL') {
    await row(page).getByRole('button', { name: 'e621', exact: true }).click(); await field(page).fill('fox')
    await settings(page); await page.getByRole('radio', { name: 'SFW', exact: true }).click()
    await expect(row(page).getByRole('button', { name: 'e621', exact: true, includeHidden: true })).toHaveCount(0)
    await expect(row(page).locator('[aria-pressed=true]')).toHaveText('WikiFur')
    await page.getByRole('radio', { name: 'NSFW', exact: true }).click()
    await page.getByRole('switch', { name: 'Show furry search options' }).click()
    await expect(page.getByRole('radio', { name: 'NSFW', exact: true })).toBeChecked()
    await page.getByRole('switch', { name: 'Show furry search options' }).click(); await close(page)
    await category(page, 'Furry').click(); await row(page).getByRole('button', { name: 'YiffParty', exact: true }).click()
    await field(page).press('Enter')
    expect(await page.evaluate(() => (window as unknown as { opened: { url: string }[] }).opened.map(o => o.url))).toEqual(['https://yiff-party.com/search/?q=fox'])
  } else { await settings(page); await expect(page.getByRole('radio', { name: 'NSFW', exact: true })).toHaveCount(0) }
})

test('catalog UI: locale retains valid selection, suggestions stay destination-independent and IME never submits', async ({ page }) => {
  await page.goto(english)
  const name = region === 'CN' ? 'Baidu' : 'DuckDuckGo', host = region === 'CN' ? 'www.baidu.com' : 'duckduckgo.com', parameter = region === 'CN' ? 'wd' : 'q'
  await row(page).getByRole('button', { name, exact: true }).click()
  await field(page).fill('furry'); await expect(page.getByRole('option', { name: 'furry art', exact: true })).toBeVisible()
  await field(page).press('ArrowDown'); await field(page).press('Enter')
  await settings(page); await page.getByRole('radio', { name: '简体中文', exact: true }).click(); await close(page)
  await expect(row(page).locator('[aria-pressed=true]')).toHaveText(region === 'CN' ? '百度' : 'DuckDuckGo')
  await expect(field(page)).toHaveValue('furry')
  await field(page).dispatchEvent('compositionstart'); await field(page).fill('兽人'); await field(page).press('Enter')
  expect(await page.evaluate(() => (window as unknown as { opened: unknown[] }).opened.length)).toBe(1)
  await field(page).dispatchEvent('compositionend'); await field(page).press('Escape'); await field(page).press('Enter')
  const urls = await page.evaluate(() => (window as unknown as { opened: { url: string }[] }).opened.map(o => o.url))
  expect(urls).toHaveLength(2)
  for (const url of urls) expect(new URL(url).hostname).toBe(host)
  expect(new URL(urls[0]).searchParams.get(parameter)).toBe('furry games'); expect(new URL(urls[1]).searchParams.get(parameter)).toBe('兽人')
})

test('catalog UI: SSR reads only its regional preference, default and corrupt values stay enabled', async ({ browser, baseURL }) => {
  for (const saved of [undefined, '0', 'invalid']) {
    const context = await browser.newContext({ baseURL, javaScriptEnabled: false })
    await context.addCookies([{ name: `gf-furry-search-${other}`, value: '0', url: baseURL! }, ...(saved ? [{ name: cookie, value: saved, url: baseURL! }] : [])])
    const ssr = await context.newPage(); await ssr.goto(english)
    await expect(category(ssr, 'Furry')).toHaveCount(saved === '0' ? 0 : 1)
    await expect(row(ssr).getByRole('button')).toHaveCount(region === 'CN' ? 8 : 7)
    await expect(row(ssr).getByRole('button', { name: 'Bing', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await context.close()
  }
})

test('catalog UI: mobile/compact keyboard selection, desktop keyboard row, responsive categories and no document overflow', async ({ page }, info) => {
  await page.goto(english)
  await category(page, 'Games').click()
  const steam = row(page).getByRole('button', { name: 'Steam', exact: true }); await steam.focus(); await steam.press('End')
  await expect(row(page).getByRole('button', { name: 'Game Jolt', exact: true })).toBeFocused()
  await page.keyboard.press('Enter'); await expect(row(page).locator('[aria-pressed=true]')).toHaveText('Game Jolt')
  await settings(page); await page.getByRole('switch', { name: 'Show quick access' }).click(); await close(page)
  for (const width of [1280, 768, 390, 375, 320]) {
    await page.setViewportSize({ width, height: width < 768 ? 667 : 720 })
    await expect(page.locator('[data-desktop-platforms]')).toHaveCount(0)
    const trigger = page.locator('[data-mobile-platform-trigger]'); await trigger.focus(); await trigger.press('ArrowDown')
    const picker = page.locator('[data-search-panel=platforms][data-open=true]'); await expect(picker).toHaveCSS('opacity', '1')
    await expect(picker.locator('[aria-selected=true]')).toBeFocused(); await page.keyboard.press('Home'); await page.keyboard.press('Enter')
    await expect(trigger).toHaveText('Steam'); await expect(trigger).toBeFocused()
    await trigger.press('ArrowDown'); await page.keyboard.press('Escape'); await expect(picker).toHaveCount(0); await expect(trigger).toBeFocused()
    const last = category(page, 'Novel'); await last.focus(); await last.press('Enter')
    const bounds = (await last.boundingBox())!; expect(bounds.x).toBeGreaterThanOrEqual(0); expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    if (width === 375) { await trigger.click(); await expect(picker).toHaveCSS('opacity', '1'); await page.screenshot({ path: info.outputPath(`${region}-mobile-platforms.png`) }); await page.keyboard.press('Escape') }
    expect(await page.evaluate(() => [document.documentElement, document.body].map(e => [e.scrollWidth, e.scrollHeight]))).toEqual([[width, width < 768 ? 667 : 720], [width, width < 768 ? 667 : 720]])
    await category(page, 'Games').click()
  }
})
