import { test, expect } from '@playwright/test'
import { readFile, writeFile } from 'node:fs/promises'

test('Dev SSR links complete SCSS Modules before JavaScript in both system themes', async ({ browser }) => {
  for (const colorScheme of ['light', 'dark'] as const) {
    const context = await browser.newContext({ javaScriptEnabled: false, colorScheme })
    const page = await context.newPage()
    await page.goto('/start')
    await expect(page.locator('html')).toHaveCSS('background-color', colorScheme === 'dark' ? 'rgb(29, 31, 34)' : 'rgb(242, 227, 208)')
    await expect(page.getByRole('search')).toHaveCSS('height', '48px')
    await expect(page.locator('[data-start-logo]')).toHaveCSS('width', '40px')
    await page.goto('/'); await expect(page.locator('main')).toHaveCSS('padding-top', '32px')
    await context.close()
  }
})

test('SCSS Modules HMR and Fast Refresh preserve open state', async ({ page, request }) => {
  const source = new URL('../src/routes/index.tsx', import.meta.url)
  const stylesheet = new URL('../src/routes/index.module.scss', import.meta.url)
  const original = await readFile(source, 'utf8')
  const originalCSS = await readFile(stylesheet, 'utf8')
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  page.on('console', (e) => { if (e.type() === 'error') errors.push(e.text()) })
  try {
    const html = await (await request.get('/')).text()
    expect(html).toContain('/src/styles/global.scss')
    await page.goto('/')
    await page.waitForLoadState('networkidle')
    expect(errors).toEqual([])
    await expect(page.locator('main')).toHaveCSS('padding-top', '32px')
    await page.getByRole('button', { name: '验证交互' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.evaluate(() => { (window as unknown as { hmrMarker: boolean }).hmrMarker = true })
    await writeFile(stylesheet, originalCSS.replace('padding: 32px', 'padding: 48px'))
    await expect(page.locator('main')).toHaveCSS('padding-top', '48px', { timeout: 20_000 })
    await writeFile(source, original.replace('V0.1-C0 · SSR', 'V0.1-C0 · HMR verified · SSR'))
    await expect(page.getByText('V0.1-C0 · HMR verified · SSR / SCSS Modules / Motion / Base UI')).toBeVisible()
    await expect(page.getByRole('dialog')).toBeVisible()
    expect(await page.evaluate(() => (window as unknown as { hmrMarker: boolean }).hmrMarker)).toBe(true)
    expect(errors).toEqual([])
  } finally {
    await writeFile(source, original)
    await writeFile(stylesheet, originalCSS)
  }
})
