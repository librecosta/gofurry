import { test, expect, type Page } from '@playwright/test'
import { sampleMotion } from './motion'

const region = process.env.WEB_REGION ?? 'CN'
const origin = 'http://127.0.0.1:17320'
const englishPath = region === 'CN' ? '/en/start' : '/start'
const chinesePath = region === 'CN' ? '/start' : '/zh/start'
const input = (page: Page) => page.locator('input[name="q"]')
async function english(page: Page) { await page.goto(englishPath); await page.waitForLoadState('networkidle') }
async function settings(page: Page) { await page.locator('[data-start-settings]').click(); await expect(page.locator('[data-start-preferences]')).toHaveCSS('opacity', '1') }

test.use({ viewport: { width: 1440, height: 900 } })

test('start: explicit language URLs own SSR metadata and ignore conflicting cookies', async ({ browser }) => {
  for (const [path, locale] of [[englishPath, 'en'], [chinesePath, 'zh']]) {
    const context = await browser.newContext({ javaScriptEnabled: false, locale: locale === 'en' ? 'zh-CN' : 'en-US' })
    await context.addCookies([{ name: 'gf-locale', value: locale === 'en' ? 'zh' : 'en', url: origin }, { name: 'gf-region', value: region === 'CN' ? 'INTL' : 'CN', url: origin }])
    const page = await context.newPage(), response = await page.goto(path)
    expect(response!.status()).toBe(200)
    expect(new URL(page.url()).pathname).toBe(path)
    await expect(page.locator('html')).toHaveAttribute('lang', locale)
    await expect(page.locator('html')).toHaveAttribute('data-region', region)
    await expect(page.locator('html')).toHaveAttribute('data-content-mode', 'SFW')
    expect(response!.headers()['x-robots-tag']).toBe('noindex')
    await expect(page.locator('meta[name="robots"]')).toHaveAttribute('content', 'noindex')
    await expect(page).toHaveTitle(locale === 'en' ? 'GoFurry · Start' : 'GoFurry · 起始页')
    await expect(page.locator('[data-start-actions] > *')).toHaveCount(2)
    await expect(page.locator('[data-start-logo]')).toHaveCSS('background-color', 'rgb(215, 220, 226)')
    await expect(page.locator('[data-start-filing]')).toHaveCount(region === 'CN' ? 1 : 0)
    await expect(page.getByRole('search')).toHaveAttribute('action', region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search')
    await context.close()
  }
})

test('start: system theme is painted by SSR CSS before JS and stays synchronized', async ({ browser, page }) => {
  for (const colorScheme of ['light', 'dark'] as const) {
    const context = await browser.newContext({ javaScriptEnabled: false, colorScheme })
    const ssr = await context.newPage(); await ssr.goto('/start')
    await expect(ssr.locator('html')).toHaveAttribute('data-theme-preference', 'system')
    await expect(ssr.locator('html')).toHaveCSS('background-color', colorScheme === 'dark' ? 'rgb(29, 31, 34)' : 'rgb(242, 227, 208)')
    await expect(ssr.getByRole('search')).toHaveCSS('background-color', colorScheme === 'dark' ? 'rgba(25, 30, 38, 0.36)' : 'rgba(250, 251, 253, 0.4)')
    await context.close()
  }
  const errors: string[] = []
  page.on('pageerror', e => errors.push(e.message)); page.on('console', e => { if (e.type() === 'error') errors.push(e.text()) })
  await page.emulateMedia({ colorScheme: 'dark' }); await english(page)
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.emulateMedia({ colorScheme: 'light' }); await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await settings(page); await page.getByRole('radio', { name: 'Dark', exact: true }).click()
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.emulateMedia({ colorScheme: 'light' }); await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.reload(); await expect(page.locator('html')).toHaveAttribute('data-theme-preference', 'dark')
  await settings(page); await page.getByRole('radio', { name: 'System', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme-preference', 'system')
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  await page.emulateMedia({ colorScheme: 'dark' }); await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  expect(errors).toEqual([])
})

test('start: settings trap focus, retain their shell across language URLs and fully animate out', async ({ page }) => {
  await english(page); const shell = await page.locator('[data-start-page]').elementHandle()
  await settings(page)
  const dialog = page.locator('[data-start-preferences]')
  await expect(dialog).toHaveCSS('border-top-width', '0px'); await expect(dialog).toHaveCSS('box-shadow', 'none')
  await expect(dialog).toHaveCSS('transition-duration', '0s') // Motion owns opacity/transform.
  for (let i = 0; i < 12; i++) { await page.keyboard.press('Tab'); expect(await dialog.evaluate(el => el.contains(document.activeElement))).toBe(true) }
  await expect(dialog.getByRole('link', { name: /GitHub/ })).toHaveAttribute('href', 'https://github.com/gofurry')
  await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toHaveCount(region === 'CN' ? 0 : 1)
  if (region === 'INTL') {
    await dialog.getByRole('radio', { name: 'NSFW', exact: true }).click()
    expect((await page.context().cookies()).find(c => c.name === 'gf-content-preference')?.value).toBe('NSFW')
    await expect(dialog).toContainText('Backend content is currently SFW')
    await expect(page.locator('html')).toHaveAttribute('data-content-mode', 'SFW')
  }
  await dialog.getByRole('radio', { name: '简体中文', exact: true }).click()
  await expect(page).toHaveURL(chinesePath)
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh')
  await expect(dialog).toBeVisible()
  expect(await page.locator('[data-start-page]').evaluate((el, first) => el === first, shell)).toBe(true)
  await dialog.getByRole('radio', { name: 'English', exact: true }).click(); await expect(page).toHaveURL(englishPath)
  await page.keyboard.press('Escape'); await expect(dialog).toHaveAttribute('data-ending-style')
  const exit = await sampleMotion(dialog)
  expect(exit.some(frame => frame.opacity > 0 && frame.opacity < 1)).toBe(true)
  expect(exit.at(-1)!.t).toBeGreaterThan(250); await expect(dialog).toHaveCount(0)
  await expect(page.locator('[data-start-settings]')).toBeFocused()
  await settings(page); await page.mouse.click(4, 4); await expect(dialog).toHaveCount(0)
  await page.reload(); await settings(page)
  if (region === 'INTL') await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toBeChecked()
  else await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toHaveCount(0)
  await page.emulateMedia({ reducedMotion: 'reduce' }); await expect(dialog).toHaveCSS('transition-duration', '0s')
  await dialog.getByRole('button', { name: 'Close settings' }).click(); await expect(dialog).toHaveCount(0)
})

test('start: appearance retains the native circular reveal centered on the selected option', async ({ page }) => {
  await page.addInitScript(() => {
    (window as unknown as { reveal: unknown[] }).reveal = []
    const animate = Element.prototype.animate
    Element.prototype.animate = function(frames, options) { if (typeof options === 'object' && options.pseudoElement) (window as unknown as { reveal: unknown[] }).reveal.push({ frames, options }); return animate.call(this, frames, options) }
  })
  await page.emulateMedia({ colorScheme: 'light' }); await english(page); await settings(page)
  const dark = page.getByRole('radio', { name: 'Dark', exact: true }), bounds = (await dark.boundingBox())!
  await dark.click(); await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark'); await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal')
  const reveal = await page.evaluate(() => (window as unknown as { reveal: { frames: { clipPath: string[] }; options: { duration: number; easing: string } }[] }).reveal)
  expect(reveal).toHaveLength(1)
  expect(reveal[0].options).toMatchObject({ duration: 420, easing: 'cubic-bezier(0.22, 1, 0.36, 1)' })
  expect(reveal[0].frames.clipPath[0]).toBe(`circle(0px at ${bounds.x + bounds.width / 2}px ${bounds.y + bounds.height / 2}px)`)
  await page.emulateMedia({ reducedMotion: 'reduce' }); await page.getByRole('radio', { name: 'Light', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  expect(await page.evaluate(() => (window as unknown as { reveal: unknown[] }).reveal.length)).toBe(1)
})

test('start: settings text tabs move their underline without recentering; language radios stay unfilled', async ({ page }) => {
  await english(page); await settings(page)
  const dialog = page.locator('[data-start-preferences]'), initial = (await dialog.boundingBox())!
  const basic = dialog.getByRole('tab', { name: 'Basic', exact: true }), advanced = dialog.getByRole('tab', { name: 'Advanced', exact: true })
  const underlineStart = (await dialog.locator('[data-settings-tab-indicator]').boundingBox())!.x
  await basic.focus(); await basic.press('ArrowRight'); await expect(advanced).toBeFocused(); await expect(advanced).toHaveAttribute('aria-selected', 'true')
  await expect(advanced).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)'); await expect(advanced).toHaveCSS('outline-style', 'solid')
  const underline = dialog.locator('[data-settings-tab-indicator]')
  const progress = await sampleMotion(underline)
  expect(progress.some(frame => frame.x > underlineStart + 0.1 && frame.x < progress.at(-1)!.x - 0.1)).toBe(true)
  await expect(dialog.getByRole('tabpanel')).toBeEmpty(); expect(await dialog.boundingBox()).toEqual(initial)
  await basic.click(); await expect(basic).toHaveAttribute('aria-selected', 'true')
  const language = dialog.getByRole('radiogroup', { name: 'Language', exact: true }), englishRadio = language.getByRole('radio', { name: 'English', exact: true })
  await expect(englishRadio).toBeChecked(); await englishRadio.hover()
  await expect(englishRadio.locator('[aria-hidden="true"] > span')).toHaveCSS('opacity', '1')
  await expect(englishRadio).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)'); await expect(englishRadio).toHaveCSS('border-top-width', '0px')
  await language.getByRole('radio', { name: '简体中文', exact: true }).click(); await expect(page).toHaveURL(chinesePath)
  await expect(dialog.getByRole('radio', { name: '简体中文', exact: true }).locator('[aria-hidden="true"] > span')).toHaveCSS('opacity', '1')
  const footer = dialog.locator('footer')
  await expect(footer).toHaveText('用心打造，开源于 GitHub。'); await expect(footer).toHaveCSS('border-top-style', 'dashed'); await expect(footer).toHaveCSS('border-top-width', '2px')
  for (const width of [320, 375, 390]) {
    await page.setViewportSize({ width, height: 667 })
    for (const radio of await dialog.getByRole('radio').all()) { const r = (await radio.boundingBox())!; expect(r.x).toBeGreaterThanOrEqual(16); expect(r.x + r.width).toBeLessThanOrEqual(width - 16) }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width)
  }
  await dialog.getByRole('radio', { name: 'English', exact: true }).click(); await expect(footer).toHaveText('Made with care. Open source on GitHub.')
  await expect(footer.getByRole('link')).toHaveAttribute('href', 'https://github.com/gofurry'); await expect(footer.getByRole('link')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await page.emulateMedia({ reducedMotion: 'reduce' }); await expect(underline).toHaveCSS('transition-duration', '0s')
})

test('start: settings segments slide, retain flat theme-aware surfaces and preserve preferences', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' }); await english(page); await settings(page)
  const dialog = page.locator('[data-start-preferences]'), track = dialog.getByRole('radiogroup', { name: 'Appearance', exact: true }), selection = dialog.locator('[data-settings-selection="appearance"]')
  await expect(track.getByRole('radio', { name: 'System', exact: true })).toBeChecked()
  const lightSurface = await track.evaluate(el => getComputedStyle(el).backgroundColor)
  await expect(track).toHaveCSS('border-radius', '10px'); await expect(selection).toHaveCSS('border-radius', '8px'); await expect(selection).toHaveCSS('box-shadow', 'none')
  const selectionStart = (await selection.boundingBox())!.x
  await track.getByRole('radio', { name: 'Dark', exact: true }).click()
  const progress = await sampleMotion(selection)
  expect(progress.some(frame => frame.x > selectionStart + 0.1 && frame.x < progress.at(-1)!.x - 0.1)).toBe(true)
  await expect(page.locator('html')).not.toHaveAttribute('data-gf-theme-reveal'); expect(await track.evaluate(el => getComputedStyle(el).backgroundColor)).not.toBe(lightSurface)
  if (region === 'INTL') {
    const content = dialog.getByRole('radiogroup', { name: 'Content preference', exact: true })
    await expect(content.getByRole('radio', { name: 'SFW', exact: true })).toBeChecked()
    await content.getByRole('radio', { name: 'NSFW', exact: true }).click()
    await expect(page.locator('html')).toHaveAttribute('data-content-mode', 'SFW')
  } else await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toHaveCount(0)
  await page.reload(); await settings(page); await expect(track.getByRole('radio', { name: 'Dark', exact: true })).toBeChecked()
  if (region === 'INTL') await expect(dialog.getByRole('radio', { name: 'NSFW', exact: true })).toBeChecked()
  await page.emulateMedia({ reducedMotion: 'reduce' }); await expect(selection).toHaveCSS('transition-duration', '0s')
  await track.getByRole('radio', { name: 'Light', exact: true }).hover(); await expect(track.getByRole('radio', { name: 'Light', exact: true })).not.toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await track.getByRole('radio', { name: 'Dark', exact: true }).focus(); await page.keyboard.press('ArrowLeft')
  await expect(track.getByRole('radio', { name: 'Light', exact: true })).toBeFocused(); await expect(track.getByRole('radio', { name: 'Light', exact: true })).toHaveCSS('outline-style', 'solid')
})

test('start: logo navigation has stable first-frame placement and unavailable destinations never navigate', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 667 }); await english(page)
  await page.evaluate(() => {
    const samples: { x: number; y: number; w: number; h: number; opacity: number }[] = []
    ;(window as unknown as { navigationFrames: typeof samples }).navigationFrames = samples
    document.addEventListener('pointerdown', () => { let count = 0; const sample = () => { const el = document.querySelector('[data-start-brand]'); if (el) { const r = el.getBoundingClientRect(), parent = getComputedStyle(el.parentElement!); samples.push({ x: r.x, y: r.y, w: r.width, h: r.height, opacity: Number(getComputedStyle(el).opacity) * Number(parent.opacity) }) } if (count++ < 40) requestAnimationFrame(sample) }; requestAnimationFrame(sample) }, { once: true })
  })
  const trigger = page.getByRole('button', { name: 'GoFurry brand', exact: true }), popup = page.locator('[data-start-brand]')
  await trigger.click(); await expect(popup).toHaveCSS('opacity', '1')
  const buttons = popup.locator('[aria-disabled="true"]')
  for (const button of await buttons.all()) {
    const bounds = (await button.boundingBox())!
    await page.mouse.click(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2)
    await expect(page).toHaveURL(englishPath); await expect(button).toHaveAttribute('title', 'Not available yet')
  }
  const final = (await popup.boundingBox())!, samples = await page.evaluate(() => (window as unknown as { navigationFrames: { x: number; y: number; w: number; h: number; opacity: number }[] }).navigationFrames)
  const visible = samples.filter(s => s.opacity > 0.01)
  expect(visible.length).toBeGreaterThan(5)
  for (const frame of visible) { expect(frame.x).toBeCloseTo(final.x, 0); expect(frame.w).toBe(final.width); expect(frame.h).toBeCloseTo(final.height, 0); expect(Math.abs(frame.y - final.y)).toBeLessThanOrEqual(4.1) }
  await page.keyboard.press('Escape'); await expect(popup).toHaveCount(0); await expect(trigger).toBeFocused()
  await trigger.press('Enter'); await expect(popup).toHaveCSS('opacity', '1')
  await page.mouse.click(370, 600); await expect(popup).toHaveCount(0); await expect(trigger).toBeFocused()
})

test('start: custom clear cancels suggestions, preserves focus and never submits', async ({ page }) => {
  let calls = 0
  await page.route('**/api/v1/start/search/suggestions?*', route => { calls++; return route.fulfill({ headers: { 'X-GoFurry-Region': region, 'X-GoFurry-Content-Mode': 'SFW' }, json: { suggestions: ['golang tutorial'] } }) })
  await english(page); await input(page).fill('golang')
  await expect(page.locator('[data-search-panel][data-open="true"]')).toHaveCSS('opacity', '1')
  const clear = page.getByRole('button', { name: 'Clear search', exact: true }), submit = page.getByRole('button', { name: 'Search (new tab)', exact: true })
  const a = (await clear.boundingBox())!, b = (await submit.boundingBox())!; expect(a.x + a.width).toBeLessThan(b.x)
  await input(page).press('Tab'); await expect(clear).toBeFocused(); await clear.press('Enter')
  await expect(input(page)).toHaveValue(''); await expect(input(page)).toBeFocused(); await expect(clear).toHaveCount(0)
  await expect(page.getByRole('listbox')).toHaveCount(0); expect(page.context().pages()).toHaveLength(1)
  const before = calls; await page.waitForTimeout(700); expect(calls).toBe(before)
  await input(page).fill('next'); await clear.click(); await expect(input(page)).toBeFocused(); await expect(input(page)).toHaveValue('')
})

test('start: all categories remain reachable at 320/375/390/768px with no document overflow', async ({ page }) => {
  await english(page)
  for (const path of [englishPath, chinesePath]) {
    await page.goto(path); await page.waitForLoadState('networkidle')
    for (const width of [320, 375, 390, 768]) {
      await page.setViewportSize({ width, height: 667 })
      const track = page.locator('[data-category-track]'), group = page.locator('[data-categories]')
      const first = group.getByRole('button').first(), last = group.getByRole('button').last()
      await first.focus(); await first.press('End'); await expect(last).toBeFocused()
      const r = (await last.boundingBox())!, t = (await track.boundingBox())!
      expect(r.x).toBeGreaterThanOrEqual(t.x - 1); expect(r.x + r.width).toBeLessThanOrEqual(t.x + t.width + 1)
      await last.press('Home'); await expect(first).toBeFocused()
      const dimensions = await page.evaluate(() => ({ viewport: [innerWidth, innerHeight], root: [document.documentElement.scrollWidth, document.documentElement.scrollHeight], body: [document.body.scrollWidth, document.body.scrollHeight] }))
      expect(dimensions.root).toEqual(dimensions.viewport); expect(dimensions.body).toEqual(dimensions.viewport)
      for (const element of await page.locator('[data-start-actions] > *').all()) await expect(element).toBeVisible()
    }
  }
  // Stress the documented scroll fallback, rather than only fitting default text.
  await page.setViewportSize({ width: 320, height: 667 })
  await page.locator('[data-categories] button').evaluateAll(elements => elements.forEach(el => { (el as HTMLElement).style.fontSize = '24px' }))
  await page.locator('[data-categories] button').first().focus()
  await expect(page.locator('[data-category-edge="right"]')).toBeVisible()
  await page.keyboard.press('End'); await expect(page.locator('[data-category-edge="left"]')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320)
})

test('start: the user menu contains only an inert sign-in placeholder with full exit', async ({ page }) => {
  await english(page)
  const trigger = page.getByRole('button', { name: 'User menu', exact: true }), menu = page.getByRole('menu')
  await trigger.click(); await expect(menu).toHaveCSS('opacity', '1')
  await expect(menu.getByRole('menuitem')).toHaveCount(1); await expect(menu.getByRole('menuitem')).toBeDisabled(); await expect(menu).toContainText('Sign in')
  await page.keyboard.press('Escape'); await expect(menu).toHaveAttribute('data-ending-style'); await expect(menu).toHaveCount(0); await expect(trigger).toBeFocused()
})

test('start: first and repeated popup frames have CSS, stable geometry and complete Motion exits', async ({ page }) => {
  test.setTimeout(45000)
  await page.emulateMedia({ colorScheme: 'light' }); await english(page)
  for (const viewport of [{ width: 960, height: 600 }, { width: 375, height: 667 }]) {
    await page.setViewportSize(viewport)
    for (const [trigger, selector, travel] of [
      ['[data-start-logo]', '[data-start-brand]', 4], ['[data-start-actions] > button:last-child', '[data-start-user-menu]', 4], ['[data-start-settings]', '[data-start-preferences]', 6],
    ] as const) {
      for (let repeat = 0; repeat < 2; repeat++) {
        await page.evaluate(selector => {
          type Frame = { opacity: number; x: number; y: number; w: number; h: number; background: string }
          const frames: Frame[] = []; (window as unknown as { popupFrames: Frame[] }).popupFrames = frames
          let count = 0
          const sample = () => {
            const el = document.querySelector(selector)
            if (el) {
              const r = el.getBoundingClientRect(), s = getComputedStyle(el), parent = getComputedStyle(el.parentElement!)
              frames.push({ opacity: Number(s.opacity) * Number(parent.opacity), x: r.x, y: r.y, w: r.width, h: r.height, background: s.backgroundColor })
            }
            if (count++ < 80) requestAnimationFrame(sample)
          }
          requestAnimationFrame(sample)
        }, selector)
        await page.locator(trigger).click()
        const popup = page.locator(selector); await expect(popup).toHaveCSS('opacity', '1')
        const final = (await popup.boundingBox())!
        const frames = await page.evaluate(() => (window as unknown as { popupFrames: { opacity: number; x: number; y: number; w: number; h: number; background: string }[] }).popupFrames)
        const visible = frames.filter(frame => frame.opacity > 0.01)
        expect(visible.length).toBeGreaterThan(3)
        for (const frame of visible) {
          expect(frame.x).toBeCloseTo(final.x, 0); expect(frame.w).toBeCloseTo(final.width, 0); expect(frame.h).toBeCloseTo(final.height, 0)
          expect(Math.abs(frame.y - final.y)).toBeLessThanOrEqual(travel + 0.1)
          expect(frame.background).toBe('rgba(245, 247, 250, 0.33)')
        }
        await page.keyboard.press('Escape')
        const exit = await sampleMotion(popup)
        expect(exit.some(frame => frame.opacity > 0 && frame.opacity < 1)).toBe(true); expect(exit.at(-1)!.t).toBeGreaterThan(250)
        await expect(popup).toHaveCount(0)
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(viewport.width)
      }
    }
  }
})
