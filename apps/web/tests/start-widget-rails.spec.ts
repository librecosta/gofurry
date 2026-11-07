import { expect, test } from '@playwright/test'
import { calculate, displayNumber } from '../src/start/tools/calculate'
import { parseCandidates, parseInteger, randomInteger } from '../src/start/tools/random-values'
import { convertUnit, parseQuantity } from '../src/start/tools/unit-values'
import { moveWidget, readWidgetOrder, sameWidgetSet, toggleWidgetInOrder } from '../src/start/widget-order'

const region = process.env.WEB_REGION === 'INTL' ? 'INTL' : 'CN'
const english = region === 'CN' ? '/en/start' : '/start'
const chinese = region === 'CN' ? '/start' : '/zh/start'
const key = `gf-widget-rails:${region}`
const tools = '[data-start-tools]'
const widgets = '[data-start-widgets]'
const manager = '[data-widget-manager]'
const managerTrigger = '[data-widget-manager-trigger]'
const panel = '[data-rail-panel]'
const trigger = (id: string) => `[data-rail-trigger="${id}"]`

test('widget order rejects unregistered and duplicate IDs, enforces capacity and keeps a stable reorder', () => {
  const registered = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h']
  const order = readWidgetOrder(['b', 'unknown', 'b', null, 'a', 'c', 'd', 'e', 'f', 'g', 'h'], registered)
  expect(order).toEqual(['b', 'a', 'c', 'd', 'e', 'f', 'g'])
  expect(toggleWidgetInOrder(order, 'h', registered)).toBe(order)
  expect(toggleWidgetInOrder(toggleWidgetInOrder(order, 'a', registered), 'h', registered)).toEqual(['b', 'c', 'd', 'e', 'f', 'g', 'h'])
  expect(toggleWidgetInOrder([], 'unknown', registered)).toEqual([])
  expect(moveWidget(order, 'b', -1)).toBe(order)
  const moved = moveWidget(order, 'b', 1)
  expect(moved.slice(0, 3)).toEqual(['a', 'b', 'c'])
  expect(sameWidgetSet(order, moved)).toBe(true)
  expect(sameWidgetSet(order, ['b', 'b', 'c', 'd', 'e', 'f', 'g'])).toBe(false)
  expect(sameWidgetSet(order, [...order.slice(0, -1), 'h'])).toBe(false)
})

test('official tool arithmetic, range and unit boundaries', () => {
  for (const [expression, expected] of [['3 + 2 × (4.5 − 1)', 10], ['-.5 * -4 + 12 / 3 / 2', 4], ['0.1 + 0.2', 0.3]] as const) {
    const result = calculate(expression)
    expect('value' in result && displayNumber(result.value)).toBe(String(expected))
  }
  expect(calculate('1/(3-3)')).toEqual({ error: 'zero' })
  expect(calculate('1e308 * 2')).toEqual({ error: 'overflow' })
  for (const input of ['1+)', '1 2', 'Math.random()', '2**3', '('.repeat(161)]) expect(calculate(input)).toEqual({ error: 'invalid' })
  expect(parseInteger('1.5', -10, 10)).toBeNull()
  expect(parseInteger('1e2', -100, 100)).toBeNull()
  expect(() => randomInteger(2, 1)).toThrow(RangeError)
  expect(randomInteger(-5, -5)).toBe(-5)
  for (let i = 0; i < 16; i++) expect([-2, -1, 0, 1]).toContain(randomInteger(-2, 1))
  expect(parseCandidates(' A \r\n\nB ')).toEqual(['A', 'B'])
  expect(parseCandidates('   ')).toBeNull()
  expect(parseCandidates('x'.repeat(4001))).toBeNull()
  expect(parseCandidates('a\n'.repeat(101))).toBeNull()
  expect(convertUnit('length', 1, 'mi', 'm')).toBe(1609.344)
  expect(convertUnit('area', 1, 'acre', 'm²')).toBe(4046.8564224)
  expect(convertUnit('mass', 1, 'lb', 'kg')).toBe(0.45359237)
  expect(convertUnit('volume', 1, 'US gal', 'L')).toBe(3.785411784)
  expect(convertUnit('temperature', 212, '°F', '°C')).toBe(100)
  expect(convertUnit('temperature', -40, '°C', '°F')).toBe(-40)
  expect(convertUnit('temperature', 0, 'K', '°C')).toBe(-273.15)
  expect(convertUnit('length', 1e308, 'km', 'mm')).toBeNull()
  expect(parseQuantity('0x10')).toBeNull()
})

test('desktop rails and manager keep search in place, manage AI Hub and synchronize regional state', async ({ page, context, baseURL }, info) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => {
    if (message.type() === 'error' && /hydrat|react error #(418|423)/i.test(message.text())) errors.push(message.text())
  })
  await page.goto(english, { waitUntil: 'networkidle' })
  const search = page.getByRole('search')
  const initial = await search.boundingBox()
  await expect(page.locator(tools).getByRole('button')).toHaveCount(3)
  expect(await page.locator(tools).getByRole('button').evaluateAll(buttons => buttons.map(button => button.getAttribute('aria-label'))))
    .toEqual(['Calculator', 'Random tools', 'Unit converter'])
  await expect(page.locator(widgets).getByRole('button')).toHaveCount(1)
  const left = (await page.locator(trigger('calculator')).boundingBox())!
  const right = (await page.locator(managerTrigger).boundingBox())!
  expect(left.x).toBeLessThan(30)
  expect(right.x + right.width).toBeGreaterThan(1410)
  expect(await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight])).toEqual([1440, 900])

  const tab = await context.newPage()
  await tab.goto(english, { waitUntil: 'networkidle' })
  await page.locator(managerTrigger).click()
  const dialog = page.locator(manager)
  await expect(dialog.getByRole('tab', { name: 'Library' })).toHaveAttribute('aria-selected', 'true')
  const input = dialog.getByRole('searchbox', { name: 'Search widgets' })
  const card = dialog.locator('[data-widget-card="ai-hub"]')
  expect((await input.boundingBox())!.x).toBeLessThan((await card.boundingBox())!.x)
  await expect(dialog.locator('[data-widget-card]')).toHaveCount(1)
  await input.fill('unmatched')
  await expect(dialog.getByText('No matching widgets.')).toBeVisible()
  await input.fill('aI hUb')
  await dialog.getByRole('tab', { name: 'Added' }).click()
  await expect(dialog.getByText('No widgets added yet.')).toBeVisible()
  await dialog.getByRole('button', { name: 'Navigation', exact: true }).click()
  await expect(dialog.getByRole('tab', { name: 'Library' })).toHaveAttribute('aria-selected', 'true')
  await card.getByRole('button', { name: 'Add AI Hub' }).click()
  await expect(card.getByRole('img', { name: 'Added' })).toBeVisible()
  await expect(tab.locator(trigger('ai-hub'))).toBeVisible()
  await page.screenshot({ path: info.outputPath('desktop-widget-manager.png') })
  await dialog.getByRole('tab', { name: 'Added' }).click()
  await expect(dialog.getByRole('tabpanel')).toHaveCount(1)
  await expect(dialog.getByRole('tabpanel').getByRole('button', { name: 'Remove AI Hub' })).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Reorder AI Hub' })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(page.locator(managerTrigger)).toBeFocused()
  await expect(page.locator(widgets).getByRole('button').nth(0)).toHaveAccessibleName('AI Hub')
  await expect(page.locator(widgets).getByRole('button').nth(1)).toHaveAccessibleName('Manage widgets')
  expect(await search.boundingBox()).toEqual(initial)
  await page.screenshot({ path: info.outputPath('desktop-rails.png') })

  await page.locator('[data-start-settings]').click()
  await page.getByRole('tab', { name: 'Advanced' }).click()
  const leftSwitch = page.getByRole('switch', { name: 'Show official tools' })
  const rightSwitch = page.getByRole('switch', { name: 'Show widget rail' })
  await expect(page.getByRole('switch', { name: 'Show Set as homepage shortcut' })).toBeChecked()
  await leftSwitch.click()
  await expect(tab.locator(tools)).toHaveCount(0)
  await expect(tab.locator(trigger('ai-hub'))).toBeVisible()
  await rightSwitch.click()
  await expect(tab.locator(widgets)).toHaveCount(0)
  await leftSwitch.click()
  await expect(tab.locator(tools)).toBeVisible()
  await rightSwitch.click()
  await expect(tab.locator(trigger('ai-hub'))).toBeVisible()
  await page.keyboard.press('Escape')
  await page.reload({ waitUntil: 'networkidle' })
  await expect(page.locator(trigger('ai-hub'))).toBeVisible()
  const other = region === 'CN' ? 'INTL' : 'CN'
  expect(await page.evaluate(storageKey => localStorage.getItem(storageKey), `gf-widget-rails:${other}`)).toBeNull()
  await page.locator(managerTrigger).click()
  await dialog.getByRole('tab', { name: 'Added' }).click()
  await dialog.getByRole('tabpanel').getByRole('button', { name: 'Remove AI Hub' }).click()
  await expect(tab.locator(trigger('ai-hub'))).toHaveCount(0)
  expect(errors).toEqual([])
  await tab.close()
  expect(baseURL).toBeTruthy()
})

test('mobile rails stack below quick access and grow the whole page on short screens', async ({ page, context, baseURL }, info) => {
  await context.addCookies([{ name: `gf-quick-access-${region}`, value: '1', url: baseURL! }])
  await page.addInitScript(storageKey => localStorage.setItem(storageKey, JSON.stringify({
    version: 1, leftEnabled: true, rightEnabled: true, widgets: ['fake', 'ai-hub', 'ai-hub'],
  })), key)
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.goto(chinese, { waitUntil: 'networkidle' })
  for (const [width, height] of [[320, 280], [375, 667], [390, 620], [768, 640], [1000, 360]]) {
    await page.setViewportSize({ width, height })
    const official = (await page.locator(tools).boundingBox())!
    const self = (await page.locator(widgets).boundingBox())!
    const search = (await page.locator('[data-start-search]').boundingBox())!
    expect(official.y).toBeGreaterThanOrEqual(search.y + search.height)
    expect(self.y).toBeGreaterThan(official.y + official.height)
    const buttons = await page.locator(widgets).getByRole('button').all()
    expect(buttons).toHaveLength(2)
    for (const button of buttons) {
      const bounds = (await button.boundingBox())!
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
      expect(bounds.y).toBe(self.y)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width)
    if (region === 'CN') expect((await page.locator('[data-start-filing]').boundingBox())!.y).toBeGreaterThan(self.y + self.height)
    if (height === 280) {
      expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBeGreaterThan(height)
      await page.locator(managerTrigger).scrollIntoViewIfNeeded()
      expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
      expect(await page.locator('[data-start-background]').evaluate(element => {
        const { x, y, width, height } = element.getBoundingClientRect()
        return [x, y, width, height]
      })).toEqual([0, 0, width, height])
    }
  }
  await page.setViewportSize({ width: 375, height: 667 })
  await page.locator(trigger('ai-hub')).click()
  const hub = page.locator(panel)
  await expect(hub.getByRole('link')).toHaveCount(10)
  const bounds = (await hub.boundingBox())!
  expect(bounds.x).toBeGreaterThanOrEqual(0)
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(375)
  expect(bounds.y + bounds.height).toBeCloseTo(659, 0)
  await page.screenshot({ path: info.outputPath('mobile-ai-hub.png') })
  await page.keyboard.press('Escape')
  await expect(page.locator(trigger('ai-hub'))).toBeFocused()
  await page.locator(managerTrigger).click()
  const dialog = page.locator(manager)
  const search = (await dialog.getByRole('searchbox', { name: '搜索组件' }).boundingBox())!
  const tab = (await dialog.getByRole('tab', { name: '组件库' }).boundingBox())!
  expect(tab.y).toBeGreaterThan(search.y + search.height)
  await page.screenshot({ path: info.outputPath('mobile-widget-manager.png') })
})

test('calculator, random tools and unit converter work by keyboard and pointer with one panel open', async ({ page, context }, info) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto(english, { waitUntil: 'networkidle' })
  const calculator = page.locator(trigger('calculator'))
  await calculator.press('Enter')
  const calc = page.locator('[data-rail-panel="calculator"]')
  const expression = calc.getByLabel('Expression')
  const result = calc.locator('output')
  expect((await calc.boundingBox())!.x).toBeGreaterThan((await calculator.boundingBox())!.x + (await calculator.boundingBox())!.width)
  await expression.fill('3 + (4.5 - .5) × 2')
  await expression.press('Enter')
  await expect(result).toHaveText('11')
  await calc.getByRole('button', { name: 'Copy result' }).click()
  await expect(calc.getByText('Copied', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('11')
  await expression.fill('5/0')
  await expression.press('Enter')
  await expect(result).toHaveText('Cannot divide by zero.')
  await calc.getByRole('button', { name: 'Backspace' }).click()
  await expect(expression).toHaveValue('5/')
  await calc.getByRole('button', { name: 'Clear' }).click()
  await expect(expression).toHaveValue('')
  await page.screenshot({ path: info.outputPath('desktop-calculator.png') })

  await page.locator(trigger('random')).click()
  await expect(page.locator(panel)).toHaveCount(1)
  const random = page.locator('[data-rail-panel="random"]')
  await random.getByLabel('Minimum').fill('9')
  await random.getByLabel('Maximum').fill('9')
  await random.getByRole('button', { name: 'Generate' }).click()
  await expect(random.locator('output')).toHaveText('9')
  await random.getByLabel('Minimum').fill('9.5')
  await random.getByRole('button', { name: 'Generate' }).click()
  await expect(random.locator('output')).toContainText('Use integers')
  await random.getByRole('tab', { name: 'Dice', exact: true }).click()
  await random.getByLabel('Dice count').fill('2')
  await random.getByLabel('Sides per die').selectOption('4')
  await random.getByRole('button', { name: 'Generate' }).click()
  await expect(random.locator('output')).toHaveText(/^[1-4] \+ [1-4] · Total [2-8]$/)
  await random.getByRole('tab', { name: 'Pick an item' }).click()
  await random.getByRole('button', { name: 'Generate' }).click()
  await expect(random.locator('output')).toContainText('Enter 1 to 100 candidates')
  await random.getByLabel('Candidates (one per line)').fill('Tea\nCoffee')
  await random.getByRole('button', { name: 'Generate' }).click()
  await expect(random.locator('output')).toHaveText(/^(Tea|Coffee)$/)

  const unitTrigger = page.locator(trigger('units'))
  await unitTrigger.click()
  await expect(page.locator(panel)).toHaveCount(1)
  const units = page.locator('[data-rail-panel="units"]')
  await units.getByLabel('Category').selectOption('temperature')
  await units.getByLabel('Value').fill('100')
  await expect(units.locator('output')).toHaveText('212 °F')
  await units.getByRole('button', { name: 'Copy result' }).click()
  await expect(units.getByText('Copied', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('212 °F')
  await units.getByRole('button', { name: 'Swap units' }).click()
  await units.getByLabel('Value').fill('32')
  await expect(units.locator('output')).toHaveText('0 °C')
  await page.keyboard.press('Escape')
  await expect(page.locator(panel)).toHaveCount(0)
  await expect(unitTrigger).toBeFocused()
})

test('AI Hub loads on demand and uses ten direct official links without feeding quick access', async ({ page, context, baseURL }) => {
  const hosts = ['chatgpt.com', 'claude.ai', 'gemini.google.com', 'chat.deepseek.com', 'www.perplexity.ai', 'grok.com',
    'www.doubao.com', 'www.kimi.com', 'chat.qwen.ai', 'yuanbao.tencent.com']
  const outbound: string[] = []
  const assets: string[] = []
  page.on('request', request => {
    const url = new URL(request.url())
    if (hosts.includes(url.hostname)) outbound.push(url.hostname)
    if (url.pathname.endsWith('.js')) assets.push(url.pathname)
  })
  await context.addCookies([{ name: `gf-quick-access-${region}`, value: '1', url: baseURL! }])
  await page.addInitScript(storageKey => localStorage.setItem(storageKey, JSON.stringify({
    version: 1, leftEnabled: true, rightEnabled: true, widgets: ['ai-hub'],
  })), key)
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto(english, { waitUntil: 'networkidle' })
  expect(assets.filter(url => /\/(calculator|units|random|ai-hub)-/.test(url))).toEqual([])
  const hubTrigger = page.locator(trigger('ai-hub'))
  await hubTrigger.click()
  const hub = page.locator('[data-rail-panel="ai-hub"]')
  await expect(hub.getByRole('link')).toHaveCount(10)
  expect(assets.some(url => /\/ai-hub-/.test(url))).toBe(true)
  const bounds = (await hub.boundingBox())!
  expect(bounds.x + bounds.width).toBeLessThan((await hubTrigger.boundingBox())!.x)
  expect(outbound).toEqual([])
  expect(await hub.getByRole('link').evaluateAll(links => links.map(link => [link.getAttribute('href'), link.getAttribute('target'), link.getAttribute('rel')])))
    .toEqual(hosts.map(host => [`https://${host}/`, '_blank', 'noopener noreferrer']))
  await context.route('https://chatgpt.com/**', route => route.fulfill({ body: 'Official destination test' }))
  const [opened] = await Promise.all([page.waitForEvent('popup'), hub.getByRole('link', { name: 'ChatGPT' }).click()])
  await opened.waitForLoadState()
  expect(opened.url()).toBe('https://chatgpt.com/')
  expect(await opened.evaluate(() => [document.referrer, window.opener])).toEqual(['', null])
  expect(await page.evaluate(storageKey => localStorage.getItem(storageKey), `gf-quick-access:${region}`)).toBeNull()
  await opened.close()
})

test('invalid storage restores defaults and denied storage still supports widgets in memory', async ({ page, browser, baseURL }) => {
  await page.addInitScript(storageKey => localStorage.setItem(storageKey, '{bad'), key)
  await page.goto(english, { waitUntil: 'networkidle' })
  await expect(page.locator(tools).getByRole('button')).toHaveCount(3)
  await expect(page.locator(widgets).getByRole('button')).toHaveCount(1)

  const context = await browser.newContext()
  try {
    const blocked = await context.newPage()
    await blocked.addInitScript(() => Object.defineProperty(window, 'localStorage', {
      get() { throw new DOMException('Storage denied', 'SecurityError') },
    }))
    await blocked.goto(baseURL + english, { waitUntil: 'networkidle' })
    await blocked.locator(managerTrigger).click()
    await blocked.getByRole('button', { name: 'Add AI Hub' }).click()
    await blocked.keyboard.press('Escape')
    await expect(blocked.locator(trigger('ai-hub'))).toBeVisible()
  } finally { await context.close() }
})
