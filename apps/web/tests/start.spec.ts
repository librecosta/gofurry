import { test, expect, type Page } from '@playwright/test'

const region = process.env.WEB_REGION ?? 'CN'
const defaultLocale = region === 'CN' ? 'zh' : 'en'
const action = region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'
const origin = 'http://127.0.0.1:17320'
const placeholder = { zh: '搜索站点或内容…', en: 'Search sites or content…' }
test.use({ viewport: { width: 1440, height: 900 } })

async function english(page: Page) {
  await page.context().addCookies([{ name: 'gf-locale', value: 'en', url: origin }])
  await page.goto('/start')
}
async function interceptNavigation(page: Page) {
  await page.context().route('https://**/*', (route) => route.request().isNavigationRequest()
    ? route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Search target</title>' }) : route.continue())
}
async function submitted(page: Page, submit: () => Promise<unknown>) {
  const pending = page.waitForEvent('popup')
  await submit()
  const popup = await pending
  await popup.waitForLoadState('domcontentloaded')
  const url = new URL(popup.url())
  await popup.close()
  return url
}

test('start: SSR uses the local PNG, regional defaults and honest CN-only filing placeholders', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false })
  const page = await context.newPage()
  const response = await page.goto('/start')
  expect(response!.status()).toBe(200)
  expect(response!.headers()['cache-control']).toContain('no-store')
  await expect(page.locator('html')).toHaveAttribute('lang', defaultLocale)
  await expect(page.locator('html')).toHaveAttribute('data-region', region)
  await expect(page.getByRole('search')).toHaveAttribute('action', action)
  await expect(page.getByRole('searchbox')).toHaveAttribute('placeholder', placeholder[defaultLocale])
  await expect(page.locator('header img')).toHaveAttribute('src', '/logo-mini.png')
  expect(await page.locator('header img').evaluate((img) => (img as HTMLImageElement).naturalWidth)).toBe(64)
  expect(await page.locator('header').innerText()).not.toContain('GoFurry')
  await expect(page.locator('[data-start-actions] > *')).toHaveCount(5)
  await expect(page.locator('[data-start-actions] > button:disabled')).toHaveCount(2)
  await expect(page.locator('[data-start-filing]')).toHaveCount(region === 'CN' ? 1 : 0)
  await expect(page.locator('[data-start-filing] a')).toHaveCount(0)
  if (region === 'CN') await expect(page.locator('footer')).toContainText('待配置')
  // The default native GET search still works before hydration/without JS.
  await interceptNavigation(page)
  await page.getByRole('searchbox').fill('GoFurry 兽人 & 雪')
  const url = await submitted(page, () => page.getByRole('searchbox').press('Enter'))
  expect(url.origin).toBe(new URL(action).origin)
  expect(url.searchParams.get('q')).toBe('GoFurry 兽人 & 雪')
  await context.close()
})

test('start: top actions persist theme/locale, keep region and open the actual GitHub target', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (entry) => { if (entry.type() === 'error') errors.push(entry.text()) })
  await english(page)
  const actions = page.locator('[data-start-actions]')
  await expect(actions.locator(':scope > *').nth(0)).toHaveText('EN')
  await actions.getByRole('button', { name: 'Switch to 中文' }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh')
  await expect(page.getByRole('searchbox')).toHaveAttribute('placeholder', placeholder.zh)
  await actions.getByRole('button', { name: '切换为深色主题' }).click()
  const response = await page.reload()
  expect(await response!.text()).toContain('data-theme="dark"')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh')
  await expect(page.locator('html')).toHaveAttribute('data-region', region)
  await actions.getByRole('button', { name: '切换为 English' }).click()
  await actions.getByRole('button', { name: 'Switch to light theme' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  if (region === 'CN') await expect(page.locator('footer')).toContainText('Chengdu Yuanan Kaiwu Technology Co., Ltd.')
  await expect(actions.getByRole('button', { name: 'Settings (not available yet)', exact: true })).toBeDisabled()
  await expect(actions.getByRole('button', { name: 'User (not available yet)', exact: true })).toBeDisabled()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  const github = actions.getByRole('link')
  await expect(github).toHaveAttribute('href', 'https://github.com/gofurry')
  await expect(github).toHaveAttribute('rel', 'noopener noreferrer')
  await interceptNavigation(page)
  const url = await submitted(page, () => github.click())
  expect(url.href).toBe('https://github.com/gofurry')
  expect(errors).toEqual([])
})

test('start: Light/Dark pointer, active, keyboard and disabled states use one focus frame', async ({ page }) => {
  await english(page)
  for (const theme of ['light', 'dark']) {
    await page.context().addCookies([{ name: 'gf-theme', value: theme, url: origin }])
    await page.reload()
    const form = page.getByRole('search'), input = page.getByRole('searchbox')
    const bounds = await form.boundingBox()
    const background = await form.evaluate((el) => getComputedStyle(el).backgroundColor)
    await input.hover()
    await expect.poll(() => form.evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe(background)
    await expect(form).toHaveCSS('outline-style', 'none')
    await page.mouse.down()
    await expect(form).toHaveCSS('border-top-width', '1px')
    await page.mouse.up()
    await expect(input).toBeFocused()
    await expect(form).toHaveCSS('outline-style', 'none')
    await expect(input).toHaveCSS('outline-style', 'none')
    await page.getByRole('group', { name: 'Search categories' }).getByRole('button').last().focus()
    await page.keyboard.press('Tab')
    await expect(input).toBeFocused()
    await expect(form).toHaveCSS('outline-style', 'solid')
    await expect(form).toHaveCSS('outline-offset', '-2px')
    await expect(form).toHaveCSS('border-top-color', 'rgba(0, 0, 0, 0)')
    expect(await form.boundingBox()).toEqual(bounds)
    await input.click()
    await expect(form).toHaveCSS('outline-style', 'none')
    const toggle = page.locator('[data-start-actions] > button').nth(1)
    await toggle.hover()
    await expect(toggle).toHaveCSS('outline-style', 'none')
    await page.mouse.down()
    await expect(toggle).toHaveCSS('border-top-width', '0px')
    await page.mouse.up()
    await page.locator('[data-start-actions] > button').first().focus()
    await page.keyboard.press('Tab')
    await expect(toggle).toBeFocused()
    await expect(toggle).toHaveCSS('outline-style', 'solid')
    await expect(toggle).toHaveCSS('outline-width', '2px')
    await expect(toggle).toHaveCSS('outline-offset', '-2px')
    await expect(toggle).toHaveCSS('background-color', 'rgba(255, 248, 241, 0.92)')
    await toggle.hover()
    await expect(toggle).toHaveCSS('background-color', 'rgba(255, 248, 241, 0.92)')
    expect(await toggle.evaluate((el) => getComputedStyle(el).outlineColor)).not.toBe('rgb(0, 0, 0)')
    await page.keyboard.press('Tab')
    const github = page.locator('[data-start-actions] > a')
    await expect(github).toBeFocused()
    await expect(github).toHaveCSS('outline-style', 'solid')
    await expect(github).toHaveCSS('border-top-width', '0px')
    await expect(github).toHaveCSS('background-color', 'rgba(255, 248, 241, 0.92)')
    await expect(page.locator('[data-start-actions] > button:disabled').first()).toHaveCSS('cursor', 'not-allowed')
  }
})

test('start: categories, platform URL encoding and composition-safe submission work without legacy suggestions', async ({ page }) => {
  const suggestionRequests: string[] = []
  page.on('request', (request) => { if (/suggest|go-furry.com/.test(request.url())) suggestionRequests.push(request.url()) })
  await english(page)
  await interceptNavigation(page)
  const input = page.getByRole('searchbox')
  const query = 'wolf & 兽人 +?/#'
  const cat = page.getByRole('group', { name: 'Search categories' })
  const providers = page.locator('[data-desktop-platforms]')
  for (const [category, provider, host, parameter] of [
    ['Search', region === 'CN' ? 'Bing' : 'Google', new URL(action).hostname, 'q'],
    ['Search', 'DuckDuckGo', 'duckduckgo.com', 'q'],
    ['Search', 'Bilibili', 'search.bilibili.com', 'keyword'],
    ['Games', 'Epic', 'store.epicgames.com', 'q'],
    ['Art', 'ZCool', 'www.zcool.com.cn', 'word'],
    ['Novel', 'Project Gutenberg', 'www.gutenberg.org', 'query'],
  ]) {
    await cat.getByRole('button', { name: category, exact: true }).click()
    await providers.getByRole('button', { name: provider, exact: true }).click()
    await input.fill(`  ${query}  `)
    const url = await submitted(page, () => input.press('Enter'))
    expect(url.hostname).toBe(host)
    expect(url.searchParams.get(parameter)).toBe(query)
    if (provider === 'Bing') expect(url.searchParams.get('adlt')).toBe('strict')
    if (provider === 'Google') expect(url.searchParams.get('safe')).toBe('active')
    if (provider === 'DuckDuckGo') expect(url.searchParams.get('kp')).toBe('1')
  }
  await cat.getByRole('button', { name: 'Furry', exact: true }).click()
  await input.fill(query)
  let url = await submitted(page, () => input.press('Enter'))
  expect(url.hostname).toBe('en.wikifur.com')
  expect(decodeURIComponent(url.pathname)).toBe(`/wiki/${query}`)
  await page.locator('[data-start-actions] > button').first().click()
  url = await submitted(page, () => input.press('Enter'))
  expect(url.hostname).toBe('zh.wikifur.com')
  await input.dispatchEvent('compositionstart')
  await input.fill('中文输入法')
  const pages = page.context().pages().length
  await input.press('Enter')
  await input.dispatchEvent('keydown', { key: 'Enter', isComposing: true, keyCode: 229, bubbles: true, cancelable: true })
  expect(page.context().pages()).toHaveLength(pages)
  await input.dispatchEvent('compositionend', { data: '中文输入法' })
  await input.press('ArrowDown'); await input.press('ArrowUp'); await input.press('Escape')
  await expect(input).toHaveValue('中文输入法')
  await expect(page.getByRole('listbox')).toHaveCount(0)
  await expect(page.getByRole('progressbar')).toHaveCount(0)
  url = await submitted(page, () => input.press('Enter'))
  expect(decodeURIComponent(url.pathname)).toBe('/wiki/中文输入法')
  await input.fill('   '); await input.press('Enter')
  expect(page.context().pages()).toHaveLength(pages)
  expect(suggestionRequests).toEqual([])
})

test('start: mobile platform slots, arrows, Escape, outside blur and breakpoint focus remain usable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await english(page)
  const picker = page.locator('[data-mobile-platforms]')
  const trigger = picker.locator('[aria-expanded]')
  await expect(picker.getByRole('button')).toHaveCount(1)
  await trigger.click()
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await expect(picker.getByRole('button')).toHaveCount(7)
  await trigger.press('ArrowDown')
  expect(await picker.evaluate((el) => el.contains(document.activeElement))).toBe(true)
  await page.keyboard.press('Enter')
  await expect(trigger).toBeFocused()
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await trigger.press('Escape')
  await expect(picker.getByRole('button')).toHaveCount(1)
  await expect(trigger).toBeFocused()
  await trigger.click()
  await page.getByRole('searchbox').click()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await trigger.click()
  await page.locator('header img').click()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await trigger.click()
  await page.setViewportSize({ width: 1000, height: 800 })
  await expect(page.locator('[data-categories] [aria-pressed="true"]')).toBeFocused()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('group', { name: 'Search categories' }).getByRole('button', { name: 'Furry' }).click()
  await expect(trigger).toBeDisabled()
  await expect(trigger).toHaveText('WikiFur')
})

test('start: desktop/mobile controls sit above center with five visible actions and unobstructed CN footer', async ({ page }, testInfo) => {
  await english(page)
  for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 320, height: 568 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport)
    const group = (await page.locator('[data-start-search]').boundingBox())!
    const header = (await page.locator('header').boundingBox())!
    expect(Math.abs(group.x + group.width / 2 - viewport.width / 2)).toBeLessThanOrEqual(1)
    const center = group.y + group.height / 2
    expect(center).toBeLessThan(viewport.height * 0.43)
    if (viewport.height > 700) { expect(center / viewport.height).toBeGreaterThan(0.24); expect(center / viewport.height).toBeLessThan(0.31) }
    expect(group.y).toBeGreaterThan(header.y + header.height + 16)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(viewport.width)
    for (const button of await page.locator('[data-start-actions] > *').all()) {
      await expect(button).toBeVisible()
      const bounds = (await button.boundingBox())!
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(viewport.width)
    }
    if (region === 'CN') {
      const footer = (await page.locator('footer').boundingBox())!
      expect(footer.y).toBeGreaterThan(group.y + group.height + 12)
      expect(footer.x + footer.width).toBeLessThanOrEqual(viewport.width)
    } else await expect(page.locator('footer')).toHaveCount(0)
    if (viewport.width === 390) await page.screenshot({ path: testInfo.outputPath(`start-${region}-mobile-layout.png`) })
  }
})
