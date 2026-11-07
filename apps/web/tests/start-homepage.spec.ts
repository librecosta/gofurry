import { expect, test } from '@playwright/test'

const region = process.env.WEB_REGION === 'INTL' ? 'INTL' : 'CN'
const englishPath = region === 'CN' ? '/en/start' : '/start'
const chinesePath = region === 'CN' ? '/start' : '/zh/start'
const guide = '[data-start-homepage-guide]'
const entry = '[data-start-homepage]'
const cookie = `gf-homepage-entry-${region}`

for (const variant of [
  {
    path: chinesePath, scheme: 'light', entry: '设为主页', title: '使用 GoFurry 浏览器扩展',
    description: '安装适用于 Microsoft Edge 或 Google Chrome 的浏览器扩展，即可将 GoFurry 设为新标签页。',
    notice: '扩展暂未发布', download: '前往下载', close: '关闭', surface: 'rgba(245, 247, 250, 0.33)',
  },
  {
    path: englishPath, scheme: 'dark', entry: 'Set as homepage', title: 'Use the GoFurry browser extension',
    description: 'Install the browser extension for Microsoft Edge or Google Chrome to make GoFurry your new tab page.',
    notice: 'Extension not yet released', download: 'Download', close: 'Close', surface: 'rgba(27, 31, 38, 0.32)',
  },
] as const) {
  test(`homepage extension prompt shows localized copy and actions in ${variant.scheme}`, async ({ page, context }) => {
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.emulateMedia({ colorScheme: variant.scheme, reducedMotion: variant.scheme === 'dark' ? 'reduce' : 'no-preference' })
    await page.goto(variant.path, { waitUntil: 'networkidle' })
    const shortcut = page.getByRole('button', { name: variant.entry, exact: true })
    const weather = page.locator('[data-start-weather]')
    const search = page.getByRole('search')
    await expect(page.locator('[data-start-actions] > button').nth(0)).toHaveText(variant.entry)
    await expect(page.locator('[data-start-actions] > button').nth(1)).toHaveAttribute('data-start-weather')
    await expect(shortcut.locator('svg')).toHaveCount(0)
    await expect(shortcut).toHaveCSS('height', '40px')
    await expect(shortcut).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await shortcut.hover()
    await expect(shortcut).toHaveCSS('background-color', 'rgba(224, 230, 238, 0.14)')
    const trigger = (await shortcut.boundingBox())!
    const weatherButton = (await weather.boundingBox())!
    expect(trigger.x + trigger.width).toBeLessThan(weatherButton.x)
    expect(trigger.y).toBe(weatherButton.y)

    await shortcut.click()
    const panel = page.getByRole('dialog', { name: variant.title, exact: true })
    await expect(panel).toHaveAccessibleDescription(variant.description)
    await expect(panel).toContainText(variant.notice)
    await expect(panel).toHaveCSS('background-color', variant.surface)
    await expect(panel).toHaveCSS('width', '300px')
    const position = (await panel.boundingBox())!
    const input = (await search.boundingBox())!
    expect(position.y - (trigger.y + trigger.height)).toBe(6)
    expect(position.x).toBeLessThan(trigger.x + trigger.width)
    expect(position.x + position.width).toBeGreaterThan(trigger.x)
    expect(position.x >= input.x + input.width || position.y + position.height <= input.y).toBe(true)

    const download = panel.getByRole('button', { name: variant.download })
    await expect(panel.getByRole('button')).toHaveCount(2)
    await expect(panel.getByRole('link')).toHaveCount(0)
    await expect(download).toBeDisabled()
    await download.dispatchEvent('click')
    await expect(page).toHaveURL(variant.path)
    expect(context.pages()).toHaveLength(1)
    await panel.getByRole('button', { name: variant.close, exact: true }).click()
    await expect(shortcut).toHaveAttribute('aria-expanded', 'false')
    await expect(page.locator(guide)).toHaveCount(0)
    await expect(shortcut).toBeFocused()
  })
}

test('homepage prompt restores keyboard focus and does not block search or other header panels', async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 600 })
  await page.goto(englishPath, { waitUntil: 'networkidle' })
  const shortcut = page.locator(entry)
  const panel = page.locator(guide)
  const search = page.getByRole('combobox')
  await shortcut.press('Enter')
  await expect(panel.getByRole('button', { name: 'Close', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(panel).toHaveCount(0)
  await expect(shortcut).toBeFocused()

  await shortcut.click()
  const promptBounds = (await panel.boundingBox())!
  const inputBounds = (await page.getByRole('search').boundingBox())!
  expect(promptBounds.y + promptBounds.height).toBeLessThan(inputBounds.y)
  await search.click()
  await expect(panel).toHaveCount(0)
  await expect(search).toBeFocused()
  await search.fill('fox')
  await expect(search).toHaveValue('fox')

  await shortcut.click()
  await page.locator('[data-start-weather]').click()
  await expect(page.locator('[data-start-weather-details]')).toBeVisible()
  await expect(panel).toHaveCount(0)
  await shortcut.click()
  await expect(panel).toBeVisible()
  await expect(page.locator('[data-start-weather-details]')).toHaveCount(0)
  await page.locator('[data-start-settings]').click()
  await expect(page.locator('[data-start-preferences]')).toBeVisible()
  await expect(panel).toHaveCount(0)
})

test('homepage preference is regional, persisted and already applied on the SSR page', async ({ page, browser, context, baseURL }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => {
    if (message.type() === 'error' && /hydrat|react error #(418|423)/i.test(message.text())) errors.push(message.text())
  })
  const other = region === 'CN' ? 'INTL' : 'CN'
  await context.addCookies([{ name: `gf-homepage-entry-${other}`, value: '0', url: baseURL! }])
  await page.goto(chinesePath, { waitUntil: 'networkidle' })
  const toggle = page.getByRole('switch', { name: '显示设为主页入口' })
  const openAdvanced = async () => {
    await page.locator('[data-start-settings]').click()
    await page.getByRole('tab', { name: '高级' }).click()
  }
  await expect(page.locator(entry)).toBeVisible()
  await openAdvanced()
  await expect(toggle).toBeChecked()
  await toggle.click()
  await expect(toggle).not.toBeChecked()
  await page.keyboard.press('Escape')
  await expect(page.locator(entry)).toHaveCount(0)
  const saved = (await context.cookies()).find(item => item.name === cookie)!
  expect(saved.value).toBe('0')
  expect(saved.expires).toBeGreaterThan(Date.now() / 1000)
  const ssr = await browser.newContext({ javaScriptEnabled: false })
  try {
    await ssr.addCookies([saved])
    const document = await ssr.newPage()
    await document.goto(baseURL + englishPath)
    await expect(document.locator(entry)).toHaveCount(0)
  } finally { await ssr.close() }
  await page.reload({ waitUntil: 'networkidle' })
  await expect(page.locator(entry)).toHaveCount(0)
  await openAdvanced()
  await expect(toggle).not.toBeChecked()
  await toggle.click()
  await expect(toggle).toBeChecked()
  await page.keyboard.press('Escape')
  await page.reload({ waitUntil: 'networkidle' })
  await expect(page.locator(entry)).toBeVisible()
  await page.locator(entry).click()
  await expect(page.locator(guide)).toBeVisible()
  await page.setViewportSize({ width: 375, height: 667 })
  await expect(page.locator(entry)).toBeHidden()
  await expect(page.locator(guide)).toHaveCount(0)
  expect(errors).toEqual([])
})
