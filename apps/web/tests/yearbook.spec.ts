import { expect, test } from '@playwright/test'

const cn = process.env.CATALOG_CN_WEB!
const intl = process.env.CATALOG_INTL_WEB!
const api = process.env.CATALOG_CN_API!
const id = '000000000000000a'
const site = `/sites/${id}/example-community`
const yearbook = site + '/yearbook/2025'
const collection = `${api}/api/v1/sites/${id}/yearbooks`

test('published yearbook is indexed in its own locale and linked from the Site', async ({ request, browser }) => {
  for (const [base, origin, prefix, locale, title] of [
    [cn, 'https://cn.catalog.example', '', 'zh', '示例社区 · 2025 年站点年鉴'],
    [cn, 'https://cn.catalog.example', '/en', 'en', 'Example Community · 2025 Site Yearbook'],
    [intl, 'https://intl.catalog.example', '', 'en', 'Example Community · 2025 Site Yearbook'],
    [intl, 'https://intl.catalog.example', '/zh', 'zh', '示例社区 · 2025 年站点年鉴'],
  ]) {
    const response = await request.get(base + prefix + yearbook, {
      headers: { Cookie: 'gf-locale=en; gf-theme=dark', 'Accept-Language': 'fr', 'X-Forwarded-Host': 'untrusted.example' },
    })
    expect(response.status()).toBe(200)
    expect(response.headers()['cache-control']).toBe('public, max-age=0, must-revalidate')
    expect(response.headers()['vary'] ?? '').not.toMatch(/cookie/i)
    const html = await response.text()
    expect(html).toContain(`<html lang="${locale}"`)
    expect(html).toContain(`<title>${title}</title>`)
    expect(html).toContain(`rel="canonical" href="${origin}${prefix}${yearbook}"`)
    expect(html).not.toContain('untrusted.example')
    expect(html).not.toContain('PRIVATE-FIXTURE')
  }
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 360, height: 740 }, colorScheme: 'dark', reducedMotion: 'reduce' })
  try {
    const page = await context.newPage()
    await page.goto(cn + site)
    await page.getByRole('link', { name: '2025 年站点年鉴', exact: true }).click()
    await expect(page.getByRole('heading', { name: '2025 年站点年鉴' })).toBeVisible()
    await expect(page.getByText('14 / 365 天', { exact: true })).toBeVisible()
    await expect(page.getByText('未知表示该日缺少可确认的证据。', { exact: false })).toBeVisible()
    await expect(page.locator('link[rel=alternate][hreflang=zh]')).toHaveAttribute('href', 'https://cn.catalog.example' + yearbook)
    await expect(page.locator('link[rel=alternate][hreflang=en]')).toHaveAttribute('href', 'https://cn.catalog.example/en' + yearbook)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
    expect(overflow).toBe(false)
    await page.keyboard.press('Tab')
    await expect(page.getByRole('link', { name: '站点详情' })).toBeFocused()
    const darkText = await page.locator('main').evaluate(el => getComputedStyle(el).color)
    await page.emulateMedia({ colorScheme: 'light' })
    const lightText = await page.locator('main').evaluate(el => getComputedStyle(el).color)
    expect(lightText).not.toBe(darkText)
    await page.getByRole('link', { name: 'English' }).click()
    await expect(page.getByRole('heading', { name: '2025 Site Yearbook' })).toBeVisible()
    await expect(page.getByText('14 of 365', { exact: true })).toBeVisible()
    await page.getByText('Unknown date ranges', { exact: false }).click()
    await expect(page.getByText('2025-01-01 – 2025-11-10', { exact: true })).toBeVisible()
  } finally {
    await context.close()
  }
})

test('yearbook routing, revision visibility and public data allowlist', async ({ request }) => {
  const redirect = await request.get(cn + `/en/sites/${id}/old-slug/yearbook/2025?next=https://untrusted.example`, { maxRedirects: 0 })
  expect(redirect.status()).toBe(301)
  expect(new URL(redirect.headers().location, cn).href).toBe(cn + '/en' + yearbook)
  expect(redirect.headers()['cache-control']).toContain('no-store')
  for (const path of [site + '/yearbook/25', site + '/yearbook/2026', yearbook + '/extra',
    `/sites/${id}/yearbook/2025`, '/sites/000000000000000c/private-draft/yearbook/2025',
    '/games/000000000000000b/orbit-game/yearbook/2025']) {
    const response = await request.get(cn + path, { maxRedirects: 0 })
    expect(response.status(), path).toBe(404)
  }
  const list = await request.get(collection)
  expect(list.status()).toBe(200)
  expect(await list.json()).toEqual({ years: [2025] })
  const response = await request.get(collection + '/2025')
  expect(response.status()).toBe(200)
  const data = await response.json()
  expect(Object.keys(data).sort()).toEqual(['current_names', 'document', 'public_id', 'published_at', 'revision', 'site_available', 'slug', 'year'])
  expect(data.revision).toBe(1)
  expect(data.document.coverage).toMatchObject({ days: 14, partial_year: true, recorded_from: '2025-11-11' })
  const json = JSON.stringify(data)
  expect(json).not.toMatch(/PRIVATE-FIXTURE|<script>|target_id|observation_id|binding_id|source_id|api_key|authorization/i)
  expect(json).not.toMatch(/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}/i)
  const write = await request.post(collection + '/2025', { data: { revision: 100 } })
  expect([404, 405]).toContain(write.status())
  expect((await (await request.get(collection + '/2025')).json()).revision).toBe(1)
  expect((await request.get(collection + '/0000')).status()).toBe(404)
  expect((await request.get(api + '/api/v1/games/000000000000000b/yearbooks')).status()).toBe(404)
  const draft = await request.get(api + '/api/v1/sites/000000000000000c/yearbooks/2025')
  expect(draft.status()).toBe(404)
  expect(draft.headers()['cache-control']).toContain('no-store')
  expect((await request.get(cn + '/start')).status()).toBe(200)
})
