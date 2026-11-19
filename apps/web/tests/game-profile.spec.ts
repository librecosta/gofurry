import { expect, test } from '@playwright/test'

const id = '2000000000000000'
const app = '3990000000'
const second = '3990000040'
const blocked = '2000000000000029'
const path = '/games/' + id + '/game-0'
const api = process.env.CATALOG_CN_API + '/api/v1/games/'
const cn = process.env.CATALOG_CN_WEB!
const intl = process.env.CATALOG_INTL_WEB!
const pixels = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jtyQAAAAASUVORK5CYII=', 'base64')

test('scoped profile, official news keyset and authenticated observation semantics', async ({ request }) => {
  const response = await request.get(api + id + '?profile=1&locale=en')
  expect(response.status()).toBe(200)
  const profile = await response.json()
  expect(profile.sources).toHaveLength(3)
  expect(profile.sources.find((source: { external_key: string }) => source.external_key === app)).toMatchObject({
    details: { publishers: ['Pine Press'] },
    media: { screenshots: expect.arrayContaining([expect.stringContaining('harbor.jpg')]), movies: [expect.stringContaining('trailer.mp4')] },
  })
  expect(profile.localizations).toContainEqual(expect.objectContaining({ locale: 'en', summary: expect.stringContaining('forgotten coastline') }))
  expect(profile.sources.find((source: { external_key: string }) => source.external_key === app).details.description).toBeUndefined()
  expect(JSON.stringify(profile)).not.toMatch(/untrusted\.example|script\.html|content_hash|binding_id|target_id/)
  expect(profile.related.map((row: { public_id: string }) => row.public_id)).toEqual(['2000000000000001','2000000000000002'])
  expect(profile.news.items).toHaveLength(6)
  const next = await request.get(api + id + '/news?after=' + profile.news.after)
  expect(next.status()).toBe(200)
  const { items } = await next.json()
  expect(items).toHaveLength(2)
  expect(new Set([...profile.news.items, ...items].map(item => item.gid)).size).toBe(8)
  const read = async (country: string, selected = app) => {
    const response = await request.get(api + id + '/insights?app_id=' + selected + '&country=' + country + '&range=30&locale=en')
    expect(response.status()).toBe(200)
    return response.json()
  }
  const us = await read('us')
  expect(us.currency).toBe('USD')
  expect(us.price_days).toHaveLength(3)
  expect(us.price_days[0]).toMatchObject({ state: 'zero_price', final: 0, original: 1500 })
  expect(us.price_changes).toHaveLength(1)
  expect(us.player_days).toHaveLength(3)
  expect(us.player_days[0]).toMatchObject({ maximum: 23, average: 20, samples: 2 })
  expect(us.player_days[2]).toMatchObject({ maximum: 0, average: 0, samples: 2 })
  expect(us.ranges).toEqual([7,30])
  expect((await read('cn')).price_days).toMatchObject([{ state: 'free', final: null }])
  expect((await read('hk')).price_days).toMatchObject([{ state: 'priced', final: 8000 }])
  expect((await read('us', second)).player_days).toMatchObject([{ maximum: 7 }])
  expect((await read('us', second)).price_days).toEqual([])
  for (const query of ['range=31', 'range=30&range=7', 'country=gb', 'app_id=3990000041', 'app_id=0001']) {
    expect((await request.get(api + id + '/insights?' + query)).status(), query).toBe(400)
  }
  for (const suffix of ['?profile=1', '/news', '/insights']) {
    expect((await request.get(api + blocked + suffix)).status(), suffix).toBe(404)
  }
})

test('game profile SSR is bilingual and initial navigation requires no JavaScript', async ({ browser, request }) => {
  for (const [origin, label, lang, width] of [[cn, '狐狸图册', 'zh', 390], [intl, 'Fox Atlas International', 'en', 1280]] as const) {
    const response = await request.get(origin + path)
    expect(response.status()).toBe(200)
    const html = await response.text()
    expect(html).toContain('<title>' + label + '</title>')
    expect(html).toContain('<html lang="' + lang + '"')
    expect(html).toContain('rel="canonical"')
    expect(html).not.toContain('<video')
    const page = await browser.newPage({ javaScriptEnabled: false, viewport: { width, height: 820 }, colorScheme: lang === 'zh' ? 'dark' : 'light' })
    await page.route('https://**.steamstatic.com/**', route => route.fulfill({ contentType: 'image/png', body: pixels }))
    try {
      await page.goto(origin + path)
      await expect(page.getByRole('heading', { name: label, level: 1 })).toBeVisible()
      await expect(page.getByRole('link', { name: lang === 'zh' ? '前往 Steam' : 'View on Steam' }).first()).toHaveAttribute('href', 'https://store.steampowered.com/app/' + app + '/')
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    } finally {
      await page.close()
    }
  }
})

test('keyboard media, CDN failure, lazy insights and gap-aware chart', async ({ page }, info) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.emulateMedia({ reducedMotion: 'reduce', colorScheme: 'dark' })
  const errors: string[] = []
  const requests: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('request', request => requests.push(request.url()))
  await page.route('https://**.steamstatic.com/**', route => {
    const url = route.request().url()
    return url.endsWith('cove.jpg') || url.endsWith('trailer.mp4') ? route.fulfill({ status: 404, body: '' })
      : route.fulfill({ contentType: 'image/png', body: pixels })
  })
  await page.goto(cn + '/en' + path)
  await expect(page.getByRole('heading', { name: 'Fox Atlas', level: 1 })).toBeVisible()
  await expect(page.locator('#overview').getByText('The foxes chart a forgotten coastline.', { exact: false })).toBeVisible()
  await expect(page.locator('video')).toHaveCount(0)
  expect(requests.some(url => url.includes('fixture/harbor.jpg') || url.includes('trailer.mp4'))).toBe(false)

  await page.getByRole('button', { name: 'Open gallery' }).click()
  const fullscreen = page.getByRole('button', { name: 'View full screen' })
  await fullscreen.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.keyboard.press('ArrowRight')
  await expect(page.getByRole('dialog').getByText('This media could not be loaded.')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(fullscreen).toBeFocused()
  await page.getByRole('button', { name: 'Trailer 1' }).click()
  const video = page.locator('video')
  await expect(video).toHaveAttribute('preload', 'none')
  expect(await video.evaluate((node: HTMLVideoElement) => node.paused && !node.autoplay)).toBe(true)
  await page.getByRole('button', { name: 'Screenshot 1' }).click()
  await expect(video).toHaveCount(0)

  await page.getByRole('button', { name: 'Load Steam insights' }).click()
  await page.getByLabel('Steam store').selectOption('us')
  await expect(page.getByRole('heading', { name: 'Price history · United States · USD' })).toBeVisible()
  const scrubber = page.getByRole('slider', { name: 'Price history' })
  await scrubber.focus()
  await page.keyboard.press('End')
  await expect(scrubber).toHaveAttribute('aria-valuetext', /Zero-price offer.*USD\s*0/)
  const chart = page.getByRole('img', { name: 'Price history', exact: true })
  // The two adjacent days form one segment; the missing day before today breaks it.
  await expect(chart.locator('path')).toHaveCount(2)
  await expect(page.getByText('Price change observed', { exact: true })).toHaveCount(1)

  const firstNews = page.locator('#news li')
  await expect(firstNews).toHaveCount(6)
  await page.getByRole('button', { name: 'Load more' }).click()
  await expect(firstNews).toHaveCount(8)
  await page.emulateMedia({ colorScheme: 'light' })
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await page.screenshot({ path: info.outputPath('game-profile-1280-light.png'), fullPage: true })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.setViewportSize({ width: 390, height: 850 })
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  await info.attach('game-profile-390-dark', { body: await page.screenshot({ path: info.outputPath('game-profile-390-dark.png'), fullPage: true }), contentType: 'image/png' })
  expect(errors).toEqual([])
})
