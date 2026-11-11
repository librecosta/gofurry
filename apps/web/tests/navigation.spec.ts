import { expect, test } from '@playwright/test'
import { categoryCodes } from '../src/navigation/contracts'

const cn = process.env.CATALOG_CN_API!
const intl = process.env.CATALOG_INTL_API!
const edge = process.env.CATALOG_CN_EDGE!
const id = '000000000000000a'
const site = `/sites/${id}/example-community`
const interactions = `/api/v1/sites/${id}/interactions`

test('regional discovery contract, pagination and request boundary', async ({ request }) => {
  for (const [base, region] of [[cn, 'CN'], [intl, 'INTL']]) {
    const detail = await request.get(base + '/api/v1/sites/' + id)
    expect(detail.status()).toBe(200)
    expect(detail.headers()['x-gofurry-interactions']).toBe(region === 'CN' ? 'enabled' : undefined)
    const home = await request.get(base + '/api/v1/sites/discovery')
    expect(home.status()).toBe(200)
    expect(home.headers()['x-gofurry-region']).toBe(region)
    expect(home.headers()['cache-control']).toBe('no-store')
    const data = await home.json()
    expect(Object.keys(data.featured)).toEqual(['primary', 'secondary_1', 'secondary_2'])
    expect(data.categories.map((item: { code: string }) => item.code)).toEqual(categoryCodes)
    expect(data.heat.status).toBe(region === 'CN' ? 'ready' : 'cold')
    expect(data.trending).toEqual([])
    expect(data.featured.primary?.public_id ?? null).toBe(region === 'CN' ? id : null)
    expect(JSON.stringify(data)).not.toMatch(/Hidden Site|layout|welfare|country|actor|score/)
  }
  const first = await request.get(cn + '/api/v1/sites/directory?category=community&page_size=1')
  expect(first.status()).toBe(200)
  const a = await first.json()
  expect(a.total).toBeGreaterThan(1)
  expect(a.items).toHaveLength(1)
  const next = await request.get(cn + '/api/v1/sites/directory?category=community&page_size=1&cursor=' + a.next)
  const b = await next.json()
  expect(b.total).toBe(a.total)
  expect(b.items[0].public_id).not.toBe(a.items[0].public_id)
  for (const query of ['country=CN', 'welfare=public', 'rating=adult', 'page_size=49', 'category=not-a-category', 'cursor=abc', 'sort=relevance']) {
    expect((await request.get(cn + '/api/v1/sites/directory?' + query)).status()).toBe(400)
  }
  expect((await request.get(cn + '/api/v1/sites/000000000000000c/stats')).status()).toBe(404)
  expect((await request.post(cn + '/api/v1/sites/discovery', { data: {} })).status()).toBe(405)
  expect((await request.post(edge + interactions, { data: { type: 'detail_view' }, headers: {
    Origin: 'https://untrusted.example', 'Sec-Fetch-Site': 'same-origin',
  } })).status()).toBe(403)
  const validOrigin = { Origin: edge, 'Sec-Fetch-Site': 'same-origin' }
  for (const data of [{ type: 'outbound_click', url: 'https://untrusted.example' }, { type: 'other' }, { type: 'detail_view', ip: '192.0.2.5' }]) {
    expect((await request.post(edge + interactions, { data, headers: validOrigin })).status()).toBe(400)
  }
  expect((await request.post(edge + interactions, { data: 'x'.repeat(81), headers: validOrigin })).status()).toBe(400)
  expect((await request.post(edge + interactions, { data: 'text', headers: validOrigin })).status()).toBe(415)
})

test('SSR is silent and only visible dwell sends a single real Beacon', async ({ page, request }) => {
  expect((await request.get(edge + site)).status()).toBe(200)
  expect((await request.get(edge + site, { headers: { Purpose: 'prefetch' } })).status()).toBe(200)
  await page.addInitScript(() => {
    let hidden = true
    Object.defineProperty(document, 'visibilityState', { get: () => hidden ? 'hidden' : 'visible' })
    window.addEventListener('navigation-fixture-visibility', () => {
      hidden = !hidden
      document.dispatchEvent(new Event('visibilitychange'))
    })
  })
  await page.clock.install()
  const events: string[] = []
  page.on('request', request => {
    if (new URL(request.url()).pathname === interactions) events.push(request.method())
  })
  await page.goto(edge + site, { waitUntil: 'networkidle' })
  await page.clock.runFor(4000)
  expect(events).toEqual([])
  const flip = () => page.evaluate(() => window.dispatchEvent(new Event('navigation-fixture-visibility')))
  await flip()
  await page.clock.runFor(1700)
  expect(events).toEqual([])
  await flip()
  await page.clock.runFor(4000)
  expect(events).toEqual([])
  await flip()
  const response = page.waitForResponse(response => new URL(response.url()).pathname === interactions)
  await page.clock.runFor(1500)
  expect((await response).status()).toBe(204)
  await page.clock.runFor(4000)
  // Chromium may omit Blob Beacon bodies from request instrumentation. The
  // fixture also verifies that this POST persisted one view and zero clicks.
  expect(events).toEqual(['POST'])
})

test('approved outbound navigation still opens natively when analytics fails', async ({ page, context }) => {
  await context.route('https://example.com/**', route => route.fulfill({ body: 'fixture destination' }))
  await page.route('**/interactions', route => route.fulfill({ status: 503 }))
  await page.goto(edge + site)
  const link = page.getByRole('link', { name: '访问网站' })
  await expect(link).toHaveAttribute('target', '_blank')
  await expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  const event = page.waitForRequest(request => request.url() === edge + interactions && request.postDataJSON().type === 'outbound_click')
  const popup = context.waitForEvent('page')
  await link.click()
  expect((await event).postDataJSON()).toEqual({ type: 'outbound_click' })
  const newPage = await popup
  await expect(newPage.locator('body')).toHaveText('fixture destination')
  expect(await newPage.evaluate(() => window.opener)).toBeNull()
})
