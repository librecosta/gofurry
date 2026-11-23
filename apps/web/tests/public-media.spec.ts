import { expect, test, type Page } from '@playwright/test'
import { publishedMediaURL, selectPublishedMedia } from '../src/games/published-media'

const host = 'https://video.akamai.steamstatic.com/'
const imgHost = 'https://shared.steamstatic.com/'
const pixel = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jtyQAAAAASUVORK5CYII=', 'base64')
const fixture = '/games/p00000000000000'
const path = (index: number) => '/en' + fixture + index + '/gallery'

async function images(page: Page) {
  await page.route(/^https:\/\/[^/]+\/.+\.(?:jpg|png)$/, route => route.request().url().includes('/bad-')
    ? route.fulfill({ status: 404, body: '' }) : route.fulfill({ contentType: 'image/png', body: pixel }))
}

async function gallery(page: Page, index = 1) {
  await page.goto(path(index))
  await page.getByRole('button', { name: 'Open gallery' }).click()
  return page.locator('#game-gallery')
}

test('published URLs match the F1 asset rules and keep a separate byte limit', () => {
  expect(selectPublishedMedia({ version: 2, locale: 'en', fallback: false, images: [], screenshots: [],
    movies: [{ movie_id: '1', name: '', sources: {} }] })).toBeUndefined()
  const base = imgHost + 'steam/apps/1/'
  const sizeLimit = base + 'a'.repeat(2048 - base.length - 4) + '.png'
  expect(publishedMediaURL(sizeLimit)).toBe(sizeLimit)
  expect(publishedMediaURL(sizeLimit + 'a.png')).toBeUndefined()
  expect(publishedMediaURL(base + '猫'.repeat(700) + '.png')).toBeUndefined()
  expect(publishedMediaURL(base + 'hello%20world.png')).toBeTruthy()
  expect(publishedMediaURL(base + 'art.gif')).toBeTruthy()
  expect(publishedMediaURL('https://shared.st.dl.eccdnx.com/steam/apps/1/art.png')).toBeTruthy()
  expect(publishedMediaURL('https://shared.cdn.steamchina.queniuam.com/store_item_assets/steam/apps/1/art.png')).toBeTruthy()
  expect(publishedMediaURL('https://shared.st.dl.eccdnx.com/other/art.png')).toBeUndefined()
  expect(publishedMediaURL(host + 'clip.mp4', '.m3u8')).toBeUndefined()
  expect(publishedMediaURL(host + 'clip.MPD', '.mpd')).toBeTruthy()
  expect(publishedMediaURL(host + 'fragment.m4s', 'stream')).toBeTruthy()
  for (const raw of [
    'http://shared.steamstatic.com/test.png', 'https://steamstatic.com.evil.test/test.png',
    'https://xsteamstatic.com/test.png', 'https://.steamstatic.com/test.png',
    'https://name@shared.steamstatic.com/test.png', 'https://shared.steamstatic.com:444/test.png',
    base + 'img.png?', base + 'img.png#', base + '../img.png', base + '%2e%2e/img.png',
    base + '%252e/img.png', base + 'a%2Fb.png', base + 'a%5Cb.png', base + 'a%00b.png',
    base + 'a%ZZ.png', base + 'a%FF.png', base + 'a\u200fb.png',
  ]) expect(publishedMediaURL(raw), raw).toBeUndefined()
})

test('SSR/no-JS shows the published cover and direct links with locale fallback', async ({ browser }) => {
  const page = await browser.newPage({ javaScriptEnabled: false })
  try {
    await images(page)
    const response = await page.goto(fixture + '1/gallery')
    expect(response?.status()).toBe(200)
    const csp = response?.headers()['content-security-policy']
    expect(csp).toContain("media-src 'self' blob: https://*.steamstatic.com")
    expect(csp).toContain("connect-src 'self' https://*.steamstatic.com")
    expect(csp?.split('connect-src')[1]).not.toContain('blob:')
    await expect(page.getByRole('heading', { name: '正式游戏', level: 1 })).toBeVisible()
    await expect(page.locator('[data-game-profile] img')).toHaveAttribute('src', /\/cover\.jpg$/)
    await expect(page.locator('#media').getByText('English')).toBeVisible()
    await expect(page.getByRole('link', { name: '截图 1' })).toHaveAttribute('href', /z-full\.jpg$/)
    await expect(page.getByRole('link', { name: 'Direct trailer' })).toHaveAttribute('href', /direct\.mp4$/)
    await expect(page.locator('video')).toHaveCount(0)
    await expect(page.locator('link[rel=canonical]')).toHaveAttribute('href', /\/games\/p000000000000001\/gallery$/)
  } finally { await page.close() }
})

for (const [width, colorScheme] of [[390, 'dark'], [1280, 'light']] as const) {
  test('published images have role shape, editorial order and keyboard preview at ' + width, async ({ page }) => {
    await page.setViewportSize({ width, height: 820 })
    await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' })
    await images(page)
    const requested: string[] = []
    page.on('request', request => requested.push(request.url()))
    const media = await gallery(page)
    const shot = media.getByRole('button', { name: 'Screenshot 1', exact: true })
    const close = page.getByRole('button', { name: 'Close', exact: true })
    const dialog = page.getByRole('dialog')
    await expect(media.locator('button[data-media-id]').first()).toHaveAttribute('data-media-id', '9007199254740995')
    await expect(shot.locator('img')).toHaveAttribute('src', /z-thumb\.jpg$/)
    await expect(media.getByRole('button', { name: 'Screenshot 2' }).locator('img')).toHaveAttribute('src', /a-full\.jpg$/)
    await expect(media.getByRole('button', { name: 'Library capsule' }).locator('[data-shape]')).toHaveAttribute('data-shape', 'portrait')
    await expect(media.getByRole('button', { name: 'Library hero', exact: true }).locator('[data-shape]')).toHaveAttribute('data-shape', 'wide')
    await expect(media.getByRole('button', { name: 'Library hero · 2×' }).locator('img')).toHaveAttribute('src', /hero\.png$/)
    await expect(media.getByRole('button', { name: 'Library logo' }).locator('[data-shape]')).toHaveAttribute('data-shape', 'logo')
    await expect(media.getByRole('button', { name: 'Community icon' }).locator('img')).toHaveAttribute('src', /shared\.st\.dl\.eccdnx\.com/)
    expect(requested.some(url => /z-full\.jpg|legacy\.jpg|hls\.light|\.m3u8/.test(url))).toBe(false)
    await shot.focus()
    await page.keyboard.press('Enter')
    await expect(dialog.getByRole('img', { name: 'Screenshot 1' })).toHaveAttribute('src', /z-full\.jpg$/)
    await close.focus()
    await page.keyboard.press('ArrowRight')
    await expect(dialog.getByRole('img', { name: 'Screenshot 2' })).toHaveAttribute('src', /a-full\.jpg$/)
    await page.keyboard.press('ArrowRight')
    await expect(dialog.getByRole('status')).toHaveText('This media could not be loaded.')
    expect(await dialog.evaluate(node => node.getBoundingClientRect().right <= innerWidth)).toBe(true)
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(shot).toBeFocused()
    await media.getByRole('button', { name: 'Screenshot 4' }).click()
    await expect(dialog.getByRole('img', { name: 'Screenshot 4' })).toHaveAttribute('src', /fallback-thumb\.jpg$/)
    await page.keyboard.press('ArrowLeft')
    await expect(dialog.getByRole('status')).toHaveText('This media could not be loaded.')
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width)
    await expect(media.getByRole('button', { name: 'Screenshot 3' })).toHaveAttribute('aria-current', 'true')
  })
}

test('catalog entry and exit apply the media policy only to the profile document', async ({ page }) => {
  await images(page)
  const directory = await page.goto('/en/games/directory')
  expect(directory?.status()).toBe(200)
  expect(directory?.headers()['content-security-policy']).toBeUndefined()
  const link = page.getByRole('link', { name: 'Published game' })
  await link.scrollIntoViewIfNeeded()
  const scrollY = await page.evaluate(() => window.scrollY)
  expect(scrollY).toBeGreaterThan(500)
  const entered = page.waitForResponse(response => response.request().isNavigationRequest() &&
    new URL(response.url()).pathname === path(1))
  await link.click()
  expect((await entered).headers()['content-security-policy']).toContain("media-src 'self' blob:")
  await page.goBack()
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThanOrEqual(scrollY - 16)
  await link.click()
  const exited = page.waitForResponse(response => response.request().isNavigationRequest() &&
    new URL(response.url()).pathname === '/en/games/directory')
  await page.getByRole('main').getByRole('link', { name: 'Game directory' }).click()
  expect((await exited).headers()['content-security-policy']).toBeUndefined()
})

test('empty or missing structured media uses legacy and single-group profiles have a gallery', async ({ page }) => {
  await images(page)
  for (const index of [2, 3]) {
    const media = await gallery(page, index)
    await expect(media.getByRole('button', { name: 'Screenshot 1' }).locator('img')).toHaveAttribute('src', /legacy\.jpg$/)
    await media.getByRole('button', {name: 'Trailer 1'}).click()
    await expect(media.locator('video')).toHaveAttribute('preload', 'none')
  }
  for (const [index, heading] of [[4, 'Trailers'], [5, 'Official artwork'], [6, 'Screenshots']] as const) {
    const media = await gallery(page, index)
    await expect(media.getByRole('heading')).toHaveText(heading)
  }
})

test('direct formats play only after preview and release the old video on switch', async ({ page }) => {
  await images(page)
  await page.route(host + '**/direct.*', route => {
    const format = route.request().url().split('.').at(-1)
    return route.fulfill({ path: 'tests/fixtures/public-media/direct.' + format, contentType: 'video/' + format,
      headers: { 'Access-Control-Allow-Origin': '*' } })
  })
  const urls: string[] = []
  page.on('request', req => urls.push(req.url()))
  const media = await gallery(page)
  expect(urls.some(url => /direct\.|hls\.light|\.m3u8/.test(url))).toBe(false)
  const movie = media.getByRole('button', { name: 'Watch Direct trailer' })
  const dialog = page.getByRole('dialog')
  await expect(movie).toHaveAttribute('data-media-id', '9007199254740993')
  await movie.click()
  const video = dialog.locator('video')
  async function play(ext: string) {
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState)).toBeGreaterThan(1)
    expect(await video.evaluate((v: HTMLVideoElement) => v.paused && !v.autoplay && v.currentTime === 0)).toBe(true)
    await expect(video).toHaveAttribute('src', new RegExp('direct\\.' + ext + '$'))
    await video.evaluate((v: HTMLVideoElement) => v.play())
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime)).toBeGreaterThan(.15)
  }
  await play('mp4')
  const first = await video.elementHandle()
  await dialog.getByRole('button', { name: 'Next' }).click()
  await expect(dialog).toHaveAccessibleName('WebM trailer')
  await play('webm')
  expect(await first?.evaluate((v: HTMLVideoElement) => !v.isConnected && v.paused && !v.hasAttribute('src'))).toBe(true)
  const second = await video.elementHandle()
  await dialog.getByRole('button', { name: 'Next' }).click()
  await expect(dialog.getByRole('status')).toContainText('uses DASH')
  expect(await second?.evaluate((v: HTMLVideoElement) => !v.isConnected && v.paused && !v.hasAttribute('src'))).toBe(true)
  await expect(dialog.getByRole('img', { name: 'DASH trailer' })).toBeVisible()
  await dialog.getByRole('button', { name: 'Next' }).click()
  await expect(dialog.getByRole('status')).toContainText('No playable source')
  await expect(dialog).toHaveAccessibleName('<em>Poster only</em>')
  await expect(dialog.locator('em')).toHaveCount(0)
  expect(urls.some(url => /only\.mpd|wrong\.mp4|hls\.light|\.m3u8/.test(url))).toBe(false)
})

test('decode error tries HLS and CORS failure is contained in the dialog', async ({ page }) => {
  await images(page)
  await page.addInitScript(() => {
    const canPlay = HTMLMediaElement.prototype.canPlayType
    HTMLMediaElement.prototype.canPlayType = function(type) {
      return type === 'application/vnd.apple.mpegurl' ? '' : canPlay.call(this, type)
    }
  })
  await page.route(host + '**/direct.mp4', route => route.fulfill({
    contentType: 'video/mp4', body: 'not a video', headers: { 'Access-Control-Allow-Origin': '*' },
  }))
  await page.route(host + '**/fallback.m3u8', route => route.fulfill({
    contentType: 'application/vnd.apple.mpegurl', body: '#EXTM3U\n',
  }))
  const media = await gallery(page)
  const manifest = page.waitForRequest(host + '**/fallback.m3u8')
  await media.getByRole('button', { name: 'Watch Direct trailer' }).click()
  await manifest
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('status')).toHaveText('The Steam stream could not be loaded.')
  await expect(dialog.getByRole('img', { name: 'Direct trailer' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('heading', { name: 'Published game', level: 1 })).toBeVisible()
})

test('HLS rejects an external resource before fetch and closing cancels a delayed import', async ({ page }) => {
  await images(page)
  await page.addInitScript(() => {
    const canPlay = HTMLMediaElement.prototype.canPlayType
    HTMLMediaElement.prototype.canPlayType = function(type) {
      return type === 'application/vnd.apple.mpegurl' ? '' : canPlay.call(this, type)
    }
  })
  let allowChunk = () => {}
  let chunkStarted = () => {}
  const delayed = new Promise<void>(resolve => { allowChunk = resolve })
  const started = new Promise<void>(resolve => { chunkStarted = resolve })
  await page.route('**/hls.light-*.js', async route => {
    chunkStarted()
    await delayed
    await route.continue()
  })
  const urls: string[] = []
  page.on('request', req => urls.push(req.url()))
  await page.route(host + '**/only.m3u8', route => route.fulfill({
    contentType: 'application/vnd.apple.mpegurl', headers: { 'Access-Control-Allow-Origin': '*' },
    body: '#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=80000\nhttps://media-outside.invalid/child.m3u8\n',
  }))
  const media = await gallery(page, 4)
  const card = media.getByRole('button', { name: 'Watch HLS trailer' })
  await card.click()
  await started
  await page.keyboard.press('Escape')
  await expect(page.locator('video')).toHaveCount(0)
  const script = page.waitForResponse(/\/hls\.light-.*\.js$/)
  allowChunk()
  expect((await script).ok()).toBe(true)
  expect(urls.some(url => url.endsWith('only.m3u8'))).toBe(false)
  await card.click()
  await expect(page.getByRole('dialog').getByRole('status')).toHaveText('The Steam stream could not be loaded.')
  expect(urls.some(url => url.endsWith('only.m3u8'))).toBe(true)
  expect(urls.some(url => url.includes('media-outside.invalid'))).toBe(false)
})
