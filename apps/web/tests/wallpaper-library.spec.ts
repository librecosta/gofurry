import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
const region = process.env.WEB_REGION ?? 'CN'
const path = region === 'CN' ? '/en/start' : '/start'
async function open(page: import('@playwright/test').Page) {
  await page.locator('[data-start-settings]').click()
  await page.locator('[data-open-wallpapers]').click()
  await expect(page.locator('[data-start-preferences]')).toHaveCount(0)
  await expect(page.locator('[data-wallpaper-library]')).toHaveCSS('opacity', '1')
  await expect(page.getByRole('dialog')).toHaveCount(1)
}
test('library: real regional PostgreSQL/CDN, independent preview, pin/off persistence and responsive dialog', async ({ page }, info) => {
  const errors: string[] = [], requests: string[] = []
  page.on('pageerror', e => errors.push(e.message)); page.on('request', r => requests.push(r.url()))
  await page.goto(path); await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity', '1')
  await open(page)
  await expect(page.getByRole('tab', { name: 'Featured', exact: true })).toBeVisible()
  await expect(page.getByRole('tab', { name: 'Local', exact: true })).toBeVisible()
  const cards = page.getByRole('button', { name: /^Preview / })
  await expect(cards.first()).toBeVisible(); expect(await cards.count()).toBeLessThanOrEqual(9)
  const before = await page.locator('[data-start-hd]').getAttribute('src')
  await cards.first().click(); expect(await page.locator('[data-start-hd]').getAttribute('src')).toBe(before)
  await page.getByRole('button', { name: 'Pin wallpaper', exact: true }).click()
  await expect.poll(() => page.evaluate(r => JSON.parse(localStorage.getItem(`gf-wallpaper:${r}`)!).mode, region)).toBe('fixed')
  await expect(page.getByRole('button', { name: 'Unpin', exact: false }).first()).toBeVisible()
  await page.screenshot({ path: info.outputPath('desktop.png') })
  for (const [width,height] of [[375,667],[1280,720]]) {
    await page.setViewportSize({ width,height }); expect(await page.evaluate(() => [document.documentElement.scrollWidth,document.documentElement.scrollHeight])).toEqual([width,height])
    const b = (await page.getByRole('dialog').boundingBox())!; expect(b.x).toBeGreaterThanOrEqual(0); expect(b.x+b.width).toBeLessThanOrEqual(width)
  }
  await page.setViewportSize({ width:375,height:667 }); await page.screenshot({ path:info.outputPath('mobile.png') })
  await page.getByRole('button', { name:'Turn off wallpaper' }).click()
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.locator('[data-start-settings]')).toBeFocused()
  requests.length=0; await page.reload(); await expect(page.locator('[data-start-settings]')).toBeVisible(); await page.waitForTimeout(800)
  expect(requests.filter(url=>/backgrounds|\.(avif|webp)(\?|$)/.test(url))).toEqual([])
  expect(errors).toEqual([])
})

test('library: local directory fallback, selected metadata, fixed/random/off and no local upload', async ({ page }) => {
  const requests: string[]=[];page.on('request',r=>{if(r.method()!=='GET')requests.push(r.url())})
  // Exercise the genuine File input fallback, not a fake image response.
  await page.addInitScript(()=>Object.defineProperty(window,'showDirectoryPicker',{value:undefined,configurable:true}))
  await page.goto(path);await open(page);await page.getByRole('tab',{name:'Local',exact:true}).click()
  await page.locator('[data-local-folder]').setInputFiles(resolve('../../services/api/internal/wallpaper/testdata'))
  const first=page.getByRole('button',{name:'Preview image.avif',exact:true})
  await expect(first).toBeVisible();await first.click()
  await expect(page.getByText('Compatibility mode: select the folder again after reloading.')).toBeVisible()
  await page.getByRole('button',{name:'Pin wallpaper',exact:true}).click()
  await expect(page.locator('[data-start-preview]').last()).toHaveAttribute('src',/^blob:/)
  await expect(page.locator('[data-start-preview]').last()).toHaveCSS('opacity','1')
  await expect(page.locator('[data-start-preview]')).toHaveCount(1)
  await expect.poll(()=>page.evaluate(r=>JSON.parse(localStorage.getItem(`gf-wallpaper:${r}`)!).fixedPath,region)).toBe('image.avif')
  await page.getByRole('button',{name:'Unpin',exact:true}).click()
  await expect.poll(()=>page.evaluate(r=>JSON.parse(localStorage.getItem(`gf-wallpaper:${r}`)!).mode,region)).toBe('random')
  await page.getByRole('button',{name:'Turn off wallpaper'}).click()
  await page.keyboard.press('Escape');await page.reload();await open(page)
  await page.getByRole('tab',{name:'Local',exact:true}).click()
  await expect(page.getByText('No local images to browse.')).toBeVisible()
  expect(requests).toEqual([])
})
