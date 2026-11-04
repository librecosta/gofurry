import { test, expect, type Page } from '@playwright/test'
import { sampleMotion } from './motion'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const region = process.env.WEB_REGION ?? 'CN'
const path = region === 'CN' ? '/en/start' : '/start'
const chinese = region === 'CN' ? '/start' : '/zh/start'
const origin = 'http://127.0.0.1:17320'
const key = `gf-quick-access:${region}`
const cookie = `gf-quick-access-${region}`
const pin = (name: string, order: number, host = `${name.toLowerCase()}.example`) => ({ id: `site-${order}`, name, url: `https://${host}/`, order })
const stored = (page: Page) => page.evaluate(k => JSON.parse(localStorage.getItem(k)!), key)
const manager = (page: Page) => page.locator('[data-quick-manager]')
const tiles = (page: Page) => page.locator('[data-quick-site]:not([inert])')
async function seed(page: Page, pinned = [pin('GitHub', 0, 'github.com')], recent: object[] = []) {
  await page.context().addCookies([{ name: cookie, value: '1', url: origin }])
  await page.addInitScript(({ key, data }) => { if (!localStorage.getItem(key)) localStorage.setItem(key, JSON.stringify(data)) }, { key, data: { version: 1, pinned, recent } })
}
async function load(page: Page) {
  await page.route('https://**/favicon.ico', route => route.fulfill({ status: 404, body: '' }))
  await page.goto(path); await expect(page.locator('[data-quick-manage]')).toBeEnabled()
}
async function manage(page: Page) { await page.locator('[data-quick-manage]').click(); await expect(manager(page)).toHaveCSS('opacity', '1') }
async function close(page: Page) { await page.keyboard.press('Escape'); await expect(manager(page)).toHaveCount(0) }
test('interaction contract: React sources never add native title tooltips', () => {
  const root = fileURLToPath(new URL('../src/', import.meta.url))
  const offenders = readdirSync(root, { recursive: true }).filter(file => typeof file === 'string' && /\.tsx?$/.test(file) && /\btitle\s*=/.test(readFileSync(join(root, file), 'utf8')))
  expect(offenders).toEqual([])
})

test('quick access: default and remembered SSR layout, smooth toggle and search retain one implementation', async ({ page, browser }) => {
  for (const enabled of [false, true]) {
    const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 1280, height: 720 } })
    if (enabled) await context.addCookies([{ name: cookie, value: '1', url: origin }])
    const ssr = await context.newPage(); await ssr.goto(path)
    await expect(ssr.locator('[data-start-search]')).toHaveAttribute('data-quick-enabled', String(enabled))
    await expect(ssr.locator('[data-mobile-platform-trigger]')).toBeVisible({ visible: enabled })
    await expect(ssr.locator('[data-desktop-platforms]')).toHaveCount(enabled ? 0 : 1)
    await expect(ssr.locator('[data-quick-access]')).toHaveCount(enabled ? 1 : 0)
    await expect(ssr.getByRole('search')).toHaveCSS('width', '500px')
    await context.close()
  }
  const errors: string[] = []; page.on('pageerror', e => errors.push(e.message))
  page.on('console', e => { if (e.type() === 'error' && /hydrat|did not match/i.test(e.text())) errors.push(e.text()) })
  await page.goto(path); await page.waitForLoadState('networkidle'); await expect(page.locator('[data-desktop-platforms]')).toBeVisible()
  const platformBounds = (await page.locator('[data-desktop-platforms]').boundingBox())!
  expect(platformBounds.x + platformBounds.width / 2).toBeCloseTo((await page.evaluate(() => innerWidth)) / 2, 1)
  await page.locator('[data-start-settings]').click()
  await expect(page.locator('[data-start-preferences]')).toHaveCSS('opacity', '1')
  const toggle = page.getByRole('switch', { name: 'Show quick access' })
  await expect(toggle).not.toBeChecked()
  const [frames] = await Promise.all([
    page.locator('[data-start-search]').evaluate(el => new Promise<{ t: number; tail: number; picker: number; form: number }[]>(resolve => {
      const frames: { t: number; tail: number; picker: number; form: number }[] = []
      let start: number | undefined
      document.addEventListener('click', () => { start = performance.now() }, { once: true, capture: true })
      const sample = () => {
        if (start !== undefined) frames.push({ t: performance.now() - start, tail: el.querySelector('[data-search-tail]')!.getBoundingClientRect().height, picker: el.querySelector('[data-mobile-platform-trigger]')!.getBoundingClientRect().width, form: el.querySelector('form')!.getBoundingClientRect().width })
        if (start === undefined || performance.now() - start < 650) requestAnimationFrame(sample); else resolve(frames)
      }
      requestAnimationFrame(sample)
    })), toggle.click(),
  ])
  expect(frames.every(f => f.form === 500)).toBe(true)
  const finalPicker = frames.at(-1)!.picker
  expect(frames.every((frame, index) => index === 0 || frame.picker >= frames[index - 1].picker - 0.1)).toBe(true)
  expect(frames.filter(f => f.picker > 1 && f.picker < finalPicker - 1).length).toBeGreaterThan(3)
  const firstTail = frames[0].tail, lastTail = frames.at(-1)!.tail
  expect(frames.filter(f => f.tail > firstTail + 1 && f.tail < lastTail - 1).length).toBeGreaterThan(3)
  await expect(toggle).toBeChecked()
  await expect(page.locator('[data-desktop-platforms]')).toHaveCount(0)
  await toggle.click()
  await expect(page.locator('[data-desktop-platforms]')).toBeVisible()
  await expect(page.locator('[data-mobile-platform-trigger]')).toBeHidden()
  await expect(page.locator('[data-quick-access]')).toHaveCount(0)
  await toggle.click()
  await closeSettings(page)
  await expect(page.locator('[data-mobile-platform-trigger]')).toBeVisible()
  await page.locator('[data-mobile-platform-trigger]').click(); await page.getByRole('option', { name: 'DuckDuckGo', exact: true }).click()
  await page.route('**/api/v1/start/search/suggestions?*', route => route.fulfill({ headers: { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' }, json: { suggestions: ['furry art'] } }))
  await page.evaluate(() => { (window as unknown as { targets: string[] }).targets = []; window.open = url => { (window as unknown as { targets: string[] }).targets.push(String(url)); return null } })
  await page.locator('input[name=q]').fill('furry'); await expect(page.getByRole('option', { name: 'furry art' })).toBeVisible()
  await page.locator('input[name=q]').press('Enter')
  expect(await page.evaluate(() => (window as unknown as { targets: string[] }).targets[0])).toContain('duckduckgo.com')
  expect((await stored(page))?.recent ?? []).toEqual([])
  await page.reload(); await expect(page.locator('[data-start-search]')).toHaveAttribute('data-quick-enabled', 'true')
  await page.goto(chinese); await expect(page.locator('[data-quick-manage]')).toBeEnabled()
  expect(errors).toEqual([])
})
async function closeSettings(page: Page) { await page.keyboard.press('Escape'); await expect(page.locator('[data-start-preferences]')).toHaveCount(0) }

test('quick access: add/edit, pointer and keyboard reorder, validation and complete manager exit', async ({ page }) => {
  await seed(page, [pin('GitHub', 0, 'github.com'), pin('Mozilla', 1, 'mozilla.org')]); await load(page); await manage(page)
  await manager(page).getByLabel('Name', { exact: true }).fill('Docs')
  await manager(page).getByLabel('URL', { exact: true }).fill('javascript:alert(1)'); await expect(manager(page).getByRole('button', { name: 'Add site', exact: true })).toBeDisabled()
  await manager(page).getByLabel('URL', { exact: true }).press('Enter'); expect((await stored(page)).pinned).toHaveLength(2)
  await manager(page).getByLabel('URL', { exact: true }).fill('https://user:password@example.com'); await expect(manager(page).getByRole('button', { name: 'Add site', exact: true })).toBeDisabled()
  await manager(page).getByLabel('URL', { exact: true }).fill('https://developer.mozilla.org/path?private=1#section'); await manager(page).getByRole('button', { name: 'Add site', exact: true }).click()
  await expect(manager(page).locator('[data-quick-pin]')).toHaveCount(3)
  const id = (await stored(page)).pinned[2].id
  await manager(page).getByRole('button', { name: 'Edit: Docs', exact: true }).click()
  await manager(page).getByLabel('Name', { exact: true }).fill('MDN Docs'); await manager(page).getByRole('button', { name: 'Save', exact: true }).click()
  expect((await stored(page)).pinned[2].id).toBe(id)
  const handle = manager(page).getByRole('button', { name: 'Drag to reorder: MDN Docs', exact: true })
  const from = (await handle.boundingBox())!, to = (await manager(page).locator('[data-quick-pin]').first().boundingBox())!
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2); await page.mouse.down()
  await page.mouse.move(from.x + from.width / 2, to.y + 4, { steps: 20 }); await page.mouse.up()
  await expect.poll(async () => (await stored(page)).pinned[0].name).toBe('MDN Docs')
  await handle.focus(); await handle.press('ArrowDown')
  await expect.poll(async () => (await stored(page)).pinned[1].name).toBe('MDN Docs')
  await expect(handle).toBeFocused()
  await page.keyboard.press('Escape'); const frames = await sampleMotion(manager(page))
  expect(frames.some(f => f.opacity > 0 && f.opacity < 1)).toBe(true); expect(frames.at(-1)!.t).toBeGreaterThan(250)
  await expect(manager(page)).toHaveCount(0); await expect(page.locator('[data-quick-manage]')).toBeFocused()
  await page.reload(); await expect(tiles(page).getByRole('link', { name: 'MDN Docs', exact: true })).toBeVisible()
  expect((await stored(page)).pinned.map((p: { order: number }) => p.order)).toEqual([0, 1, 2])
  await manage(page); await manager(page).getByRole('button', { name: 'Edit: MDN Docs', exact: true }).click()
  await expect(manager(page).getByLabel('Name', { exact: true })).toHaveValue('MDN Docs')
  expect(await manager(page).evaluate(el => el.contains(document.activeElement))).toBe(true)
  await close(page)
})

test('quick access: recent visits are local and sanitized; delete, unpin and active revisits differ', async ({ page }) => {
  await seed(page, [{ ...pin('GitHub', 0, 'github.com'), url: 'https://github.com/features?private=1#fragment' }]); await load(page)
  await page.context().route('https://**/*', route => route.request().isNavigationRequest() ? route.fulfill({ contentType: 'text/html', body: '<title>Visited site</title>' }) : route.fulfill({ status: 404, body: '' }))
  const opened = page.waitForEvent('popup'); await tiles(page).getByRole('link', { name: 'GitHub', exact: true }).click(); await (await opened).close()
  await expect.poll(async () => (await stored(page)).recent.length).toBe(1)
  expect((await stored(page)).recent[0].url).toBe('https://github.com/')
  await page.evaluate(() => document.addEventListener('contextmenu', event => { (window as unknown as { contextPrevented: boolean }).contextPrevented = event.defaultPrevented }, { once: true }))
  await tiles(page).getByRole('link', { name: 'GitHub', exact: true }).click({ button: 'right' }); await page.keyboard.press('Escape')
  expect(await page.evaluate(() => (window as unknown as { contextPrevented: boolean }).contextPrevented)).toBe(false)
  await expect(tiles(page).getByRole('button')).toHaveCount(0); await expect(page.locator('[data-quick-menu]')).toHaveCount(0)
  await manage(page); await manager(page).getByRole('button', { name: 'Unpin: GitHub', exact: true }).click()
  await expect(tiles(page)).toHaveAttribute('data-pinned', 'false')
  await manager(page).getByText('Recent visits (1)', { exact: true }).click()
  await manager(page).getByRole('button', { name: 'Pin to quick access: GitHub', exact: true }).click(); await expect(tiles(page)).toHaveAttribute('data-pinned', 'true')
  await manager(page).getByRole('button', { name: 'Delete: GitHub', exact: true }).click(); await close(page)
  await expect(tiles(page)).toHaveCount(0)
  await expect(page.locator('[data-quick-manage]')).toBeFocused()
  expect((await stored(page)).pinned).toEqual([]); expect((await stored(page)).recent).toEqual([])
  await page.reload(); await expect(page.locator('[data-quick-manage]')).toBeEnabled(); await expect(tiles(page)).toHaveCount(0)
  await page.locator('[data-start-settings]').click(); const revisit = page.waitForEvent('popup')
  await page.locator('[data-start-preferences]').getByRole('link', { name: /GoFurry GitHub/ }).click(); await (await revisit).close(); await closeSettings(page)
  await expect(tiles(page).getByRole('link', { name: 'GitHub', exact: true })).toBeVisible()
  expect((await stored(page)).recent[0].count).toBe(1)
  expect(await page.evaluate(() => document.cookie)).not.toContain('github.com')
  await manage(page); await manager(page).getByText('Recent visits (1)', { exact: true }).click()
  await manager(page).getByRole('button', { name: 'Pin to quick access: GitHub', exact: true }).click()
  expect((await stored(page)).pinned).toHaveLength(1)
  await manager(page).getByRole('button', { name: 'Remove from recent visits: GitHub', exact: true }).click()
  expect((await stored(page)).recent).toEqual([])
})

test('quick access: responsive 12/10/8/4 slots preserve data, deduplicate, theme and narrow viewport boundaries', async ({ page }, testInfo) => {
  const pinned = Array.from({ length: 14 }, (_, i) => pin(`Site${i}`, i))
  await seed(page, pinned, [{ url: pinned[0].url, name: 'Duplicate', count: 1, lastVisited: Date.now() }]); await load(page)
  for (const [width, height, count] of [[1920, 1080, 12], [1600, 900, 12], [1599, 900, 10], [1280, 720, 10], [1200, 720, 10], [1199, 720, 8], [768, 360, 8], [767, 600, 4], [390, 844, 4], [375, 667, 4], [320, 480, 4]]) {
    await page.setViewportSize({ width, height })
    await expect(tiles(page).filter({ visible: true })).toHaveCount(count)
    await expect(page.locator('[data-quick-manage]')).toBeVisible()
    const bounds = await page.locator('[data-quick-access]').boundingBox()
    expect(bounds!.x).toBeGreaterThanOrEqual(16); expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width - 16)
    expect((await stored(page)).pinned).toHaveLength(14)
    expect(await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight])).toEqual([width, height])
    await expect(page.getByRole('search')).toHaveCSS('width', width < 548 ? `${width - 32}px` : '500px')
  }
  await page.setViewportSize({ width: 1280, height: 720 }); await page.emulateMedia({ colorScheme: 'dark' })
  await page.screenshot({ path: testInfo.outputPath('quick-desktop-dark.png') })
  await page.emulateMedia({ colorScheme: 'light' }); await page.screenshot({ path: testInfo.outputPath('quick-desktop-light.png') })
  await page.setViewportSize({ width: 390, height: 844 }); await page.screenshot({ path: testInfo.outputPath('quick-mobile.png') })
  await manage(page)
  for (const [width, height] of [[768, 360], [390, 844], [375, 667], [320, 480]]) {
    await page.setViewportSize({ width, height })
    const bounds = (await manager(page).boundingBox())!
    const fields = manager(page).locator('form input, form button')
    for (const box of await fields.evaluateAll(els => els.map(el => { const r = el.getBoundingClientRect(); return { x: r.x, right: r.right, width: r.width } }))) {
      expect(box.x).toBeGreaterThanOrEqual(bounds.x); expect(box.right).toBeLessThanOrEqual(bounds.x + bounds.width)
    }
    for (const input of await manager(page).locator('form input').all()) expect((await input.boundingBox())!.width).toBeGreaterThan(150)
    expect(await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight])).toEqual([width, height])
  }
  await page.setViewportSize({ width: 390, height: 844 }); await page.screenshot({ path: testInfo.outputPath('quick-mobile-manager.png') }); await close(page)
  await page.emulateMedia({ reducedMotion: 'reduce' }); await page.reload(); await manage(page); await close(page)
})

test('quick access: touch management retains pin/edit/delete without per-site menus', async ({ browser }) => {
  const context = await browser.newContext({ baseURL: origin, viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })
  const page = await context.newPage()
  await seed(page, [], [{ url: 'https://github.com/', name: 'GitHub', count: 1, lastVisited: Date.now() }]); await load(page)
  await expect(tiles(page).getByRole('button')).toHaveCount(0)
  await page.locator('[data-quick-manage]').tap(); await expect(manager(page)).toHaveCSS('opacity', '1')
  await manager(page).getByText('Recent visits (1)', { exact: true }).tap()
  await manager(page).getByRole('button', { name: 'Pin to quick access: GitHub', exact: true }).tap()
  await expect(tiles(page)).toHaveAttribute('data-pinned', 'true')
  await manager(page).getByRole('button', { name: 'Edit: GitHub', exact: true }).tap()
  await manager(page).getByLabel('Name', { exact: true }).fill('GitHub Mobile'); await manager(page).getByRole('button', { name: 'Save', exact: true }).tap()
  await manager(page).getByRole('button', { name: 'Delete: GitHub Mobile', exact: true }).tap()
  await expect(tiles(page)).toHaveCount(0)
  await manager(page).getByRole('button', { name: 'Close quick access manager' }).tap(); await expect(manager(page)).toHaveCount(0)
  await context.close()
})

test('quick access: blocked storage degrades to memory', async ({ page }) => {
  await page.context().addCookies([{ name: cookie, value: '1', url: origin }])
  await page.addInitScript(() => { Object.defineProperty(Storage.prototype, 'getItem', { value() { throw new Error('blocked') } }); Object.defineProperty(Storage.prototype, 'setItem', { value() { throw new Error('blocked') } }) })
  await load(page); await manage(page)
  await manager(page).getByLabel('Name', { exact: true }).fill('Memory'); await manager(page).getByLabel('URL', { exact: true }).fill('https://memory.example/')
  await manager(page).getByRole('button', { name: 'Add site', exact: true }).click(); await close(page)
  await expect(tiles(page).getByRole('link', { name: 'Memory', exact: true })).toBeVisible()
})

test('quick access: pinned priority, recent fill, origin deduplication and regional isolation', async ({ page }) => {
  const now = Date.now(), otherKey = `gf-quick-access:${region === 'CN' ? 'INTL' : 'CN'}`
  await seed(page, [pin('GitHub', 0, 'github.com'), pin('Docs', 1, 'docs.example')], [
    { url: 'http://www.github.com/private?q=secret#fragment', name: 'Duplicate', lastVisited: now, count: 3 },
    { url: 'https://older.example/path', name: 'Older', lastVisited: now - 1000, count: 1 },
    { url: 'https://newer.example/path', name: 'Newer', lastVisited: now - 100, count: 1 },
    { url: 'javascript:alert(1)', name: 'Invalid', lastVisited: now, count: 1 },
  ])
  await page.addInitScript(otherKey => localStorage.setItem(otherKey, JSON.stringify({ version: 1, pinned: [{ id: 'foreign', name: 'Foreign', url: 'https://foreign.example/', order: 0 }], recent: [] })), otherKey)
  await load(page)
  await expect(tiles(page).getByRole('link')).toHaveText(['GitHub', 'Docs', 'Newer', 'Older'])
  expect(await page.evaluate(otherKey => JSON.parse(localStorage.getItem(otherKey)!).pinned[0].name, otherKey)).toBe('Foreign')
  await expect(page.getByRole('link', { name: 'Foreign', exact: true })).toHaveCount(0)
})

test('quick access: icon-only hover, compact stable form and shared 420ms input focus in both system themes', async ({ page }, testInfo) => {
  test.setTimeout(60000)
  await seed(page); await load(page)
  await expect(page.locator('[data-quick-access]')).toHaveCSS('opacity', '1')
  for (const scheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme: scheme }); await page.mouse.move(4, 500)
    const link = tiles(page).getByRole('link'), card = link.locator(':scope > span').first(), name = link.locator(':scope > span').last()
    await expect(link).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)'); await expect(name).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    const idleCard = await card.evaluate(el => getComputedStyle(el).backgroundColor)
    expect(idleCard).not.toBe('rgba(0, 0, 0, 0)')
    await expect(card).toHaveCSS('box-shadow', 'none'); await expect(card).toHaveCSS('border-top-width', '0px'); await expect(card).toHaveCSS('transition-duration', '0.3s')
    await name.hover(); await expect(card).not.toHaveCSS('background-color', idleCard)
    await expect(link).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)'); await expect(name).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await page.keyboard.press('Tab'); await link.focus(); await expect(link).toHaveCSS('outline-style', 'none')
    expect(await card.evaluate(el => getComputedStyle(el).outlineColor)).not.toBe('rgba(0, 0, 0, 0)')
    await page.locator('[data-quick-manage]').click()
    const frames = (await sampleMotion(manager(page))).filter(f => f.opacity > 0)
    expect(frames.some(f => f.opacity < 1)).toBe(true)
    expect(Math.max(...frames.map(f => f.w)) - Math.min(...frames.map(f => f.w))).toBeLessThan(0.2)
    expect(Math.max(...frames.map(f => f.h)) - Math.min(...frames.map(f => f.h))).toBeLessThan(0.2)
    expect(frames.every(f => Math.abs(f.x + f.w / 2 - 640) < 0.2)).toBe(true)
    const field = manager(page).getByLabel('Name', { exact: true }), url = manager(page).getByLabel('URL', { exact: true })
    const confirm = manager(page).getByRole('button', { name: 'Add site', exact: true }), cancel = manager(page).getByRole('button', { name: 'Cancel edit', exact: true })
    const boxes = await Promise.all([field, url, confirm, cancel].map(el => el.boundingBox()))
    expect(boxes.every(r => Math.abs(r!.y - boxes[0]!.y) < 0.2)).toBe(true)
    await expect(confirm).toBeDisabled(); await expect(cancel).toBeDisabled(); await expect(confirm).toHaveText(''); await expect(cancel).toHaveText('')
    const disabledColor = await cancel.evaluate(el => getComputedStyle(el).backgroundColor)
    await cancel.hover(); await page.waitForTimeout(350); await expect(cancel).toHaveCSS('background-color', disabledColor)
    await confirm.evaluate((el: HTMLButtonElement) => el.click()); expect((await stored(page)).pinned).toHaveLength(1)
    const dismiss = manager(page).getByRole('button', { name: 'Close quick access manager' })
    const nameSurface = await inputSurface(page, field, dismiss), urlSurface = await inputSurface(page, url, dismiss)
    expect(nameSurface).toEqual(urlSurface)
    await manager(page).getByRole('button', { name: 'Edit: GitHub', exact: true }).click()
    await expect(cancel).toBeEnabled(); await expect(manager(page).getByRole('button', { name: 'Save', exact: true })).toBeEnabled()
    const editingBoxes = await Promise.all([field, url, manager(page).getByRole('button', { name: 'Save', exact: true }), cancel].map(el => el.boundingBox()))
    expect(editingBoxes.map(r => [r!.x, r!.y, r!.width, r!.height])).toEqual(boxes.map(r => [r!.x, r!.y, r!.width, r!.height]))
    await cancel.click(); await expect(field).toHaveValue(''); await expect(url).toHaveValue(''); await expect(cancel).toBeDisabled()
    await expect(page.locator('[title]')).toHaveCount(0)
    await page.screenshot({ path: testInfo.outputPath(`manager-${scheme}.png`) }); await close(page)
    await page.locator('[data-start-weather]').click(); const weather = page.locator('[data-start-weather-details]')
    await expect(weather).toHaveCSS('opacity', '1'); await weather.getByRole('button', { name: 'Choose city', exact: true }).click()
    const citySurface = await inputSurface(page, weather.getByRole('combobox'), weather.getByRole('button', { name: 'Back to weather', exact: true }))
    expect(citySurface).toEqual(nameSurface)
    await page.keyboard.press('Escape'); await expect(weather.getByRole('button', { name: 'Choose city', exact: true })).toBeFocused()
    await page.keyboard.press('Escape'); await expect(weather).toHaveCount(0)
  }
})

async function inputSurface(page: Page, field: import('@playwright/test').Locator, blur: import('@playwright/test').Locator) {
  await blur.focus(); await page.mouse.move(4, 500); await page.waitForTimeout(450)
  await expect(field).toHaveCSS('outline-style', 'none'); await expect(field).toHaveCSS('box-shadow', 'none')
  await expect(field).toHaveCSS('border-top-width', '0px'); await expect(field).toHaveCSS('transition-duration', '0.42s')
  await expect(field).toHaveCSS('transition-timing-function', 'cubic-bezier(0.22, 1, 0.36, 1)')
  const idle = await field.evaluate(el => getComputedStyle(el).backgroundColor)
  await field.hover(); await page.waitForTimeout(450); await expect(field).toHaveCSS('background-color', idle)
  const [focusFrames] = await Promise.all([sampleSurface(field), field.focus()])
  expect(new Set(focusFrames).size).toBeGreaterThan(3)
  const focused = focusFrames.at(-1)!
  expect(focused).not.toBe(idle)
  const [blurFrames] = await Promise.all([sampleSurface(field), blur.focus()])
  expect(new Set(blurFrames).size).toBeGreaterThan(3); await expect(field).toHaveCSS('background-color', idle)
  return { idle, focused }
}
async function sampleSurface(field: import('@playwright/test').Locator) {
  return field.evaluate(el => new Promise<string[]>(resolve => {
    const values: string[] = [], end = performance.now() + 480
    const sample = () => { values.push(getComputedStyle(el).backgroundColor); if (performance.now() < end) requestAnimationFrame(sample); else resolve(values) }
    requestAnimationFrame(sample)
  }))
}
