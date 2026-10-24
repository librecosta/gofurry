import { test, expect } from '@playwright/test'
import { readFile, writeFile } from 'node:fs/promises'

test('StyleX updates CSS and Fast Refresh preserves open state', async ({ page, request }) => {
  const source = new URL('../src/routes/index.tsx', import.meta.url)
  const original = await readFile(source, 'utf8')
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  try {
    const html = await (await request.get('/')).text()
    expect(html).toContain('/virtual:stylex.css')
    await page.goto('/')
    await page.waitForLoadState('networkidle')
    expect(errors).toEqual([])
    await expect(page.locator('main')).toHaveCSS('padding-top', '32px')
    await page.getByRole('button', { name: '验证交互' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.evaluate(() => { (window as unknown as { hmrMarker: boolean }).hmrMarker = true })
    await writeFile(source, original.replace('padding: 32', 'padding: 48').replace('V0.1-A · SSR', 'V0.1-A · HMR verified · SSR'))
    await expect(page.locator('main')).toHaveCSS('padding-top', '48px', { timeout: 20_000 })
    await expect(page.getByText('V0.1-A · HMR verified · SSR / StyleX / Base UI')).toBeVisible()
    await expect(page.getByRole('dialog')).toBeVisible()
    expect(await page.evaluate(() => (window as unknown as { hmrMarker: boolean }).hmrMarker)).toBe(true)
    expect(errors).toEqual([])
  } finally {
    await writeFile(source, original)
  }
})
