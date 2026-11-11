import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'
import { expect, test } from '@playwright/test'
import { defaultLocale } from '../src/locale-path'
import { entityPath } from '../src/catalog/public-path'
import type { Kind } from '../src/catalog/detail-server'

const cn = process.env.CATALOG_CN_WEB!
const intl = process.env.CATALOG_INTL_WEB!
const cnAPI = process.env.CATALOG_CN_API!
const intlAPI = process.env.CATALOG_INTL_API!
const site = '000000000000000a'
const game = '000000000000000b'
const archivedGame = '000000000000000f'
const sitePath = `/sites/${site}/example-community`
const gamePath = `/games/${game}/orbit-game`

test('regional SSR, published metadata and cookie-independent HTML', async ({ request, browser }) => {
  for (const [base, prefix, origin, locale, name, summary] of [
    [cn, '', 'https://cn.catalog.example', 'zh', '示例社区', '中文编辑简介。'],
    [cn, '/en', 'https://cn.catalog.example', 'en', 'Example Community', 'A curated English summary.'],
    [intl, '', 'https://intl.catalog.example', 'en', 'Example Community', 'A curated English summary.'],
    [intl, '/zh', 'https://intl.catalog.example', 'zh', '示例社区', '中文编辑简介。'],
  ]) {
    const path = prefix + sitePath
    const response = await request.get(base + path, { headers: { Cookie: 'gf-locale=zh; gf-theme=dark', 'Accept-Language': 'fr-FR' } })
    expect(response.status()).toBe(200)
    expect(response.headers()['cache-control']).toBe('public, max-age=60')
    expect(response.headers()['vary'] ?? '').not.toMatch(/cookie/i)
    const html = await response.text()
    expect(html).toContain(`<html lang="${locale}"`)
    expect(html).toContain(`<title>${name}</title>`)
    expect(html).toContain(`content="${summary}"`)
    expect(html).toContain(`rel="canonical" href="${origin}${path}"`)
    expect(html).not.toContain('Runtime skeleton')
  }
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 390, height: 780 } })
  try {
    const page = await context.newPage()
    await page.goto(cn + sitePath)
    await expect(page.getByRole('heading', { name: '示例社区' })).toBeVisible()
    await expect(page.getByRole('link', { name: '访问网站' })).toHaveAttribute('href', 'https://example.com/new/Community?tag=art&tag=games')
    await expect(page.locator('link[rel=alternate][hreflang=en]')).toHaveAttribute('href', 'https://cn.catalog.example/en' + sitePath)
    await expect(page.locator('link[rel=alternate][hreflang=zh]')).toHaveAttribute('href', 'https://cn.catalog.example' + sitePath)
    await page.getByRole('link', { name: 'English', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Example Community' })).toBeVisible()
  } finally {
    await context.close()
  }
})

test('301 resolves by Public ID once, keeps locale, and drops arbitrary query data', async ({ request }) => {
  for (const [base, from, to] of [
    [cn, `/sites/${site}`, sitePath],
    [cn, `/en/sites/${site}/original-slug?next=https://untrusted.example`, '/en' + sitePath],
    [cn, '/zh' + sitePath, sitePath],
    [intl, `/zh/sites/${site}`, '/zh' + sitePath],
    [intl, '/en' + gamePath, gamePath],
    [intl, `/games/${game}/Previous-Slug`, gamePath],
  ]) {
    const response = await request.get(base + from, { maxRedirects: 0 })
    expect(response.status(), from).toBe(301)
    expect(new URL(response.headers().location, base).href).toBe(base + to)
    expect(response.headers()['cache-control']).toContain('no-store')
    expect((await request.get(base + to, { maxRedirects: 0 })).status()).toBe(200)
  }
})

test('missing translation keeps the source name and suppresses translated SEO', async ({ request, page }) => {
  const response = await request.get(cn + gamePath)
  expect(response.status()).toBe(200)
  expect(response.headers()['cache-control']).toContain('no-store')
  expect(response.headers()['x-robots-tag']).toBe('noindex')
  await page.goto(cn + gamePath)
  await expect(page.getByRole('heading', { name: 'Orbit Game' })).toHaveAttribute('lang', 'en')
  await expect(page.getByText('此页面暂无中文译文。')).toBeVisible()
  await expect(page.locator('link[rel=canonical], link[rel=alternate][hreflang=zh], meta[name=description]')).toHaveCount(0)
  await expect(page.locator('link[rel=alternate][hreflang=en]')).toHaveAttribute('href', 'https://cn.catalog.example/en' + gamePath)
  await expect(page.getByRole('link', { name: 'steam' })).toHaveAttribute('href', 'https://store.steampowered.com/app/42/')
})

test('real API exposes bounded published DTOs and preserves Start and read boundaries', async ({ request }) => {
  const rows = await request.get(cnAPI + '/api/v1/sites?limit=1')
  expect(rows.status()).toBe(200)
  const page = await rows.json()
  expect(page.items).toHaveLength(1)
  expect(page.after).toBe(site)
  expect(Object.keys(page.items[0]).sort()).toEqual(['localizations', 'public_id', 'slug'])
  expect((await (await request.get(cnAPI + '/api/v1/sites?limit=1&after=' + site)).json()).items).toHaveLength(1)
  for (const query of ['limit=0', 'limit=51', 'limit=1&limit=2', 'after=17', 'unknown=true']) {
    expect((await request.get(cnAPI + '/api/v1/sites?' + query)).status()).toBe(400)
  }
  const detail = await (await request.get(cnAPI + '/api/v1/games/' + game)).json()
  expect(Object.keys(detail).sort()).toEqual(['localizations', 'public_id', 'slug', 'sources'])
  expect(detail.sources.map((source: { platform_code: string }) => source.platform_code)).toEqual(['itch_io', 'steam'])
  expect((await request.get(intlAPI + '/api/v1/games/000000000000000e')).status()).toBe(404)
  expect((await request.get(cnAPI + '/api/v1/sites/000000000000000c')).status()).toBe(404)
  expect((await request.get(cnAPI + '/api/v1/games/' + archivedGame)).status()).toBe(404)
  expect((await request.get(cnAPI + '/api/v1/sites/000000000000000A')).status()).toBe(404)
  expect((await request.post(cnAPI + '/api/v1/sites', { data: {} })).ok()).toBe(false)
  expect((await request.get(cnAPI + '/internal/v1/sites/' + site)).status()).toBe(404)
  const wallpapers = await (await request.get(cnAPI + '/api/v1/start/backgrounds')).json()
  expect(wallpapers.total).toBe(1)
  const start = await request.get(cn + '/start')
  expect(start.status()).toBe(200)
  expect(start.headers()['cache-control']).toBe('private, no-store')
  expect(start.headers().vary).toMatch(/cookie/i)
  expect(start.headers()['x-robots-tag']).toBe('noindex')
})

test('drafts, archive, malformed IDs, numeric routes and reserved yearbook return real 404', async ({ request }) => {
  expect((await request.get(cn + '/sites/1234567890123456/example-community')).status()).toBe(200)
  for (const path of [
    '/sites/short/no', '/sites/000000000000000A/known', '/sites/000000000000000z/unknown',
    '/en/sites/000000000000000c/private-draft', '/games/' + archivedGame + '/orbit-game',
    '/site/41', '/en/sites/41', '/games/41', '/sites/example-community', '/sites/1234567890123456',
    '/fr' + sitePath, sitePath + '/yearbook/2026',
  ]) {
    const response = await request.get(cn + path, { maxRedirects: 0 })
    expect(response.status(), path).toBe(404)
    expect(response.headers().location).toBeUndefined()
    expect(response.headers()['cache-control'], path).toContain('no-store')
    expect(await response.text()).not.toContain('Hidden Site')
  }
})

test('mock legacy mapping is typed, resolves only published targets, and redirects once', async ({ request }) => {
  const keys = new Map([['sites:41', site], ['games:41', game], ['games:42', archivedGame]])
  const server = createServer(async (incoming, outgoing) => {
    const match = /^\/(?:(en|zh)\/)?(site|sites|games)\/([1-9][0-9]*)$/.exec(incoming.url ?? '')
    if (!match) { outgoing.writeHead(404).end(); return }
    const kind: Kind = match[2] === 'games' ? 'games' : 'sites'
    const key = keys.get(kind + ':' + match[3])
    if (!key) { outgoing.writeHead(404).end(); return }
    try {
      const response = await fetch(`${cnAPI}/api/v1/${kind}/${key}`)
      if (!response.ok) { outgoing.writeHead(response.status).end(); return }
      const row = await response.json()
      const locale = match[1] === 'en' || match[1] === 'zh' ? match[1] : defaultLocale('CN')
      outgoing.writeHead(301, { Location: cn + entityPath(kind, row.public_id, row.slug, locale, 'CN') }).end()
    } catch {
      outgoing.writeHead(503).end()
    }
  })
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
  const origin = 'http://127.0.0.1:' + (server.address() as AddressInfo).port
  try {
    for (const [from, to] of [['/site/41', sitePath], ['/en/sites/41', '/en' + sitePath], ['/games/41', gamePath]]) {
      const response = await request.get(origin + from, { maxRedirects: 0 })
      expect(response.status()).toBe(301)
      expect(response.headers().location).toBe(cn + to)
      expect((await request.get(cn + to, { maxRedirects: 0 })).status()).toBe(200)
    }
    for (const invalid of ['/site/999', '/sites/0', '/games/42']) {
      expect((await request.get(origin + invalid)).status()).toBe(404)
    }
  } finally {
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  }
})
