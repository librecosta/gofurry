import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { copyFile, mkdir } from 'node:fs/promises'
import { sampleMotion } from './motion'
const region = process.env.WEB_REGION ?? 'CN'
const path = region === 'CN' ? '/en/start' : '/start'
async function open(page: import('@playwright/test').Page) {
  await page.locator('[data-start-settings]').click()
  await page.locator('[data-open-wallpapers]').click()
  await expect(page.locator('[data-start-preferences]')).toHaveCount(0)
  await expect(page.locator('[data-wallpaper-library]')).toHaveCSS('opacity', '1')
  await expect(page.getByRole('dialog')).toHaveCount(1)
}
test('library: real regional PostgreSQL/CDN, independent preview, preview pin and remembered Off', async ({ page }, info) => {
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
  await expect(page.getByRole('button', { name:/^(View HD|Turn off wallpaper|Use Featured random)$/ })).toHaveCount(0)
  await page.evaluate(r=>localStorage.setItem(`gf-wallpaper:${r}`,JSON.stringify({version:1,source:'official',mode:'off'})),region)
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.locator('[data-start-settings]')).toBeFocused()
  requests.length=0; await page.reload(); await expect(page.locator('[data-start-settings]')).toBeVisible(); await page.waitForTimeout(800)
  expect(requests.filter(url=>/backgrounds|\.(avif|webp)(\?|$)/.test(url))).toEqual([])
  expect(errors).toEqual([])
})

test('library: local directory fallback, selected metadata, fixed/random/source changes and URL cleanup', async ({ page }) => {
  await page.addInitScript(()=>{const live=new Set<string>(),create=URL.createObjectURL.bind(URL),revoke=URL.revokeObjectURL.bind(URL);(window as unknown as {wallpaperURLs:Set<string>}).wallpaperURLs=live;URL.createObjectURL=b=>{const u=create(b);live.add(u);return u};URL.revokeObjectURL=u=>{live.delete(u);revoke(u)}})
  const requests: string[]=[];page.on('request',r=>{if(r.method()!=='GET')requests.push(r.url())})
  // Exercise the genuine File input fallback, not a fake image response.
  await page.addInitScript(()=>Object.defineProperty(window,'showDirectoryPicker',{value:undefined,configurable:true}))
  await page.goto(path);await open(page);await page.getByRole('tab',{name:'Local',exact:true}).click()
  await page.locator('[data-local-folder]').setInputFiles(resolve('../../services/api/internal/wallpaper/testdata'))
  const first=page.getByRole('button',{name:'Preview image.avif',exact:true})
  await expect(first).toBeVisible();await first.click()
  await expect(page.locator('[data-local-metadata]')).toContainText('image.avif')
  await expect(page.getByText('Compatibility mode: select the folder again after reloading.')).toBeVisible()
  await page.getByRole('button',{name:'Pin wallpaper',exact:true}).click()
  await expect(page.locator('[data-start-preview]').last()).toHaveAttribute('src',/^blob:/)
  await expect(page.locator('[data-start-preview]').last()).toHaveCSS('opacity','1')
  await expect(page.locator('[data-start-preview]')).toHaveCount(1)
  await expect.poll(()=>page.evaluate(r=>JSON.parse(localStorage.getItem(`gf-wallpaper:${r}`)!).fixedPath,region)).toBe('image.avif')
  await page.getByRole('button',{name:'Unpin',exact:true}).click()
  await expect.poll(()=>page.evaluate(r=>JSON.parse(localStorage.getItem(`gf-wallpaper:${r}`)!).mode,region)).toBe('random')
  await page.getByRole('tab',{name:'Featured',exact:true}).click()
  await page.getByRole('button',{name:'Use Featured wallpapers',exact:true}).click()
  await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity','1')
  await page.keyboard.press('Escape');await expect.poll(()=>page.evaluate(()=>(window as unknown as {wallpaperURLs:Set<string>}).wallpaperURLs.size)).toBe(0)
  await page.reload();await open(page)
  await page.getByRole('tab',{name:'Local',exact:true}).click()
  await expect(page.getByText('No local images to browse.')).toBeVisible()
  expect(requests).toEqual([])
})

test('library: bounded pages, skeletons, debounce, stale requests, image failure and browser-only preview', async ({ page,request }) => {
  const base=(await (await request.get(`${process.env.START_API_URL ?? 'http://127.0.0.1:17321'}/api/v1/start/backgrounds?page_size=9`)).json()).backgrounds[0]
  const rows=Array.from({length:21},(_,i)=>({...base,id:`01a1ab75-f106-7088-b9a6-${String(i).padStart(12,'0')}`,title:`Wallpaper ${i+1}`}))
  rows[9].preview={...base.preview,url:'https://wallpaper-test.invalid/missing.webp'}
  await page.route('https://wallpaper-test.invalid/missing.webp',r=>r.abort())
  const headers={'X-GoFurry-Region':region,'X-GoFurry-Content-Mode':'SFW'},calls:string[]=[]
  await page.route('**/api/v1/start/backgrounds?**',async route=>{
    const url=new URL(route.request().url()),p=Number(url.searchParams.get('page')??1),q=url.searchParams.get('q')??'';calls.push(url.search)
    expect(url.searchParams.get('page_size')).toBe('9')
    await new Promise(r=>setTimeout(r,q==='slow'?700:180))
    try{await route.fulfill({json:{backgrounds:q?rows.filter(x=>x.title.includes(q)):rows.slice((p-1)*9,p*9),total:q?rows.filter(x=>x.title.includes(q)).length:21,page:p,page_size:9},headers})}catch{/* canceled request */}
  })
  await page.goto(path);await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity','1');await open(page)
  await expect(page.getByRole('button',{name:/^Preview Wallpaper /})).toHaveCount(9)
  await page.getByRole('button',{name:'Next',exact:true}).click()
  await expect(page.locator('[aria-busy=true]')).toBeVisible()
  await expect(page.getByRole('button',{name:'Preview Wallpaper 10',exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'Preview Wallpaper 10',exact:true}).getByRole('status')).toBeVisible()
  await page.getByRole('button',{name:'Next',exact:true}).click();await expect(page.getByRole('button',{name:/^Preview Wallpaper /})).toHaveCount(3)
  const search=page.getByRole('textbox',{name:'Search Featured wallpapers'})
  await search.fill('slow');await page.waitForTimeout(350);await search.fill('Wallpaper 2')
  await expect(page.getByRole('button',{name:'Preview Wallpaper 2',exact:true})).toBeVisible();await page.waitForTimeout(800)
  await expect(page.getByRole('button',{name:/^Preview Wallpaper /})).toHaveCount(3)
  expect(calls.every(q=>q.includes('page_size=9'))).toBe(true)
  await search.fill('no-match');await expect(page.getByText('No matching wallpapers.')).toBeVisible()
  // Applying a source is independent of the currently filtered catalog page.
  await expect(page.getByRole('button',{name:'Use Featured wallpapers',exact:true})).toBeEnabled()
  await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('library: fixed 503 is retained, only 404 resets, Local denial never falls back to CDN', async ({ browser,request })=>{
  const base=(await (await request.get(`${process.env.START_API_URL ?? 'http://127.0.0.1:17321'}/api/v1/start/backgrounds`)).json()).backgrounds[0],headers={'X-GoFurry-Region':region,'X-GoFurry-Content-Mode':'SFW'}
  for(const status of [503,404]){
    const context=await browser.newContext(),page=await context.newPage();let random=0
    await page.addInitScript(({region,id})=>localStorage.setItem(`gf-wallpaper:${region}`,JSON.stringify({version:1,source:'official',mode:'fixed',fixedId:id})),{region,id:base.id})
    await page.route(`**/api/v1/start/backgrounds/${base.id}`,r=>r.fulfill({status,headers,json:{error:{code:status}}}))
    await page.route('**/api/v1/start/backgrounds/random**',r=>{random++;return r.fulfill({headers,json:base})})
    await page.goto(`http://127.0.0.1:17322${path}`)
    if(status===404){await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity','1');expect(random).toBe(1)}
    else{await expect(page.getByRole('search')).toBeVisible();await open(page);await expect(page.getByText('Wallpaper service unavailable. Try again later.')).toBeVisible();expect(random).toBe(0)}
    expect(await page.evaluate(r=>JSON.parse(localStorage.getItem(`gf-wallpaper:${r}`)!).mode,region)).toBe(status===404?'random':'fixed')
    await context.close()
  }
  const context=await browser.newContext(),page=await context.newPage(),calls:string[]=[]
  await page.addInitScript(r=>localStorage.setItem(`gf-wallpaper:${r}`,JSON.stringify({version:1,source:'local',mode:'fixed',fixedPath:'gone.jpg'})),region)
  page.on('request',r=>{if(/backgrounds|\.(avif|webp)(\?|$)/.test(r.url()))calls.push(r.url())})
  await page.goto(`http://127.0.0.1:17322${path}`);await page.waitForLoadState('networkidle');await open(page)
  await expect(page.getByText('Choose a folder in Local.')).toBeVisible()
  expect(calls).toEqual([]);expect(await page.evaluate(r=>JSON.parse(localStorage.getItem(`gf-wallpaper:${r}`)!).fixedPath,region)).toBe('gone.jpg')
  await context.close()
})

test('library: corrupt preferences recover and never inherit the other deployment',async({browser,request})=>{
  const base=(await(await request.get(`${process.env.START_API_URL??'http://127.0.0.1:17321'}/api/v1/start/backgrounds`)).json()).backgrounds[0]
  for(const raw of ['{broken',JSON.stringify({version:2,source:'official',mode:'off'}),JSON.stringify({version:1,source:'official',mode:'fixed',fixedId:'-'.repeat(36)}),JSON.stringify({version:1,source:'local',mode:'fixed',fixedPath:'../private.jpg'})]){
    const context=await browser.newContext(),page=await context.newPage();let count=0
    await page.addInitScript(({region,raw})=>{localStorage.setItem(`gf-wallpaper:${region}`,raw);localStorage.setItem(`gf-wallpaper:${region==='CN'?'INTL':'CN'}`,JSON.stringify({version:1,source:'official',mode:'off'}))},{region,raw})
    await page.route('**/api/v1/start/backgrounds/random**',route=>{count++;return route.fulfill({json:base,headers:{'X-GoFurry-Region':region,'X-GoFurry-Content-Mode':'SFW'}})})
    await page.goto(`http://127.0.0.1:17322${path}`);await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity','1');expect(count).toBe(1)
    await context.close()
  }
})

test('library: reselecting an inaccessible Local folder resumes its fixed path; cancel keeps the image',async({page})=>{
  const calls:string[]=[]
  await page.addInitScript(r=>{localStorage.setItem(`gf-wallpaper:${r}`,JSON.stringify({version:1,source:'local',mode:'fixed',fixedPath:'image.avif'}));Object.defineProperty(window,'showDirectoryPicker',{value:undefined,configurable:true})},region)
  page.on('request',r=>{if(/backgrounds|https:.*\.(avif|webp)(\?|$)/.test(r.url()))calls.push(r.url())})
  await page.goto(path);await open(page)
  await expect(page.getByText('Choose a folder in Local.')).toBeVisible()
  await page.locator('[data-local-folder]').setInputFiles(resolve('../../services/api/internal/wallpaper/testdata'))
  await expect(page.locator('[data-start-background]')).toHaveAttribute('data-stage','image')
  const before=await page.locator('[data-start-preview]').getAttribute('src')
  await page.evaluate(()=>Object.defineProperty(window,'showDirectoryPicker',{configurable:true,value:async()=>{throw new DOMException('Canceled','AbortError')}}))
  await page.getByRole('button',{name:'Choose folder',exact:true}).click();await page.waitForTimeout(550)
  expect(await page.locator('[data-start-preview]').getAttribute('src')).toBe(before)
  expect(calls).toEqual([])
})

test('library refinement: compact entry, text tabs, preview pin and source toolbar remain accessible', async ({ page }, info) => {
  await page.goto(path); await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity','1')
  await page.locator('[data-start-settings]').click()
  const entry=page.locator('[data-open-wallpapers]')
  await expect(entry).toHaveCSS('font-size','13px'); await expect(entry).toHaveCSS('padding-left','12px')
  await expect(entry).toHaveCSS('margin-bottom','16px'); await expect(entry).toHaveCSS('transition-duration','0.34s, 0.34s')
  await entry.click(); await expect(page.locator('[data-wallpaper-library]')).toHaveCSS('opacity','1')
  const preview=page.getByRole('region',{name:'Wallpaper preview'})
  await expect(preview.getByRole('button',{name:'Pin wallpaper',exact:true})).toBeVisible()
  await expect(preview.getByText(/^(Preview only|Pinned)$/)).toHaveCount(0)
  await expect(page.getByRole('button',{name:/^(View HD|Turn off wallpaper|Use Featured random|Use Local random|Authorize)$/})).toHaveCount(0)
  const pin=preview.getByRole('button',{name:'Pin wallpaper',exact:true})
  await pin.click()
  const frames=await sampleMotion(preview.getByRole('button',{name:'Unpin',exact:true}).locator('span').last(),420)
  expect(frames.some(frame=>frame.opacity>0 && frame.opacity<1)).toBe(true)
  expect(frames.at(-1)!.opacity).toBe(1)
  await expect(page.getByRole('button',{name:'Use Featured wallpapers',exact:true})).toBeVisible()
  for(const colorScheme of ['light','dark'] as const){
    await page.emulateMedia({colorScheme})
    await expect(page.getByRole('tab',{name:'Featured',exact:true})).toHaveCSS('border-bottom-width','0px')
    await expect(page.locator('[data-selected=true]').first()).toHaveCSS('outline-style','none')
    await page.screenshot({path:info.outputPath(colorScheme+'.png')})
  }
  await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0);await expect(page.locator('[data-start-settings]')).toBeFocused()
})

test('library refinement: tabs/search reuse bounded local previews and metadata changes atomically', async ({ page }, info) => {
  const folder=info.outputPath('folder');await mkdir(folder,{recursive:true})
  await Promise.all(Array.from({length:21},(_,i)=>copyFile(resolve('../../services/api/internal/wallpaper/testdata',i===1?'nonwide.webp':'preview.webp'),resolve(folder,String(i+1).padStart(2,'0')+(i===0?'-wide.webp':i===1?'-square.webp':'-sample.webp')))))
  await page.addInitScript(()=>{
    const urls=new Set<string>(),create=URL.createObjectURL.bind(URL),revoke=URL.revokeObjectURL.bind(URL),bitmap=createImageBitmap
    const stats={decodes:0,urls};(window as unknown as {localPreviewStats:typeof stats}).localPreviewStats=stats
    URL.createObjectURL=blob=>{const url=create(blob);urls.add(url);return url};URL.revokeObjectURL=url=>{urls.delete(url);revoke(url)}
    window.createImageBitmap=((...args:Parameters<typeof createImageBitmap>)=>{stats.decodes++;return bitmap(...args)}) as typeof createImageBitmap
    Object.defineProperty(window,'showDirectoryPicker',{value:undefined,configurable:true})
  })
  await page.goto(path);await expect(page.locator('[data-start-hd]')).toHaveCSS('opacity','1');await open(page)
  await page.getByRole('tab',{name:'Local',exact:true}).click();await page.locator('[data-local-folder]').setInputFiles(folder)
  const cards=page.getByRole('button',{name:/^Preview \d/})
  await expect(cards).toHaveCount(9)
  await expect.poll(()=>cards.locator('img').count()).toBe(9)
  const initial=await cards.locator('img').evaluateAll(images=>images.map(image=>image.getAttribute('src')))
  const decodes=()=>page.evaluate(()=>(window as unknown as {localPreviewStats:{decodes:number}}).localPreviewStats.decodes)
  const before=await decodes()
  for(let i=0;i<2;i++){
    await page.getByRole('tab',{name:'Featured',exact:true}).click();await page.getByRole('tab',{name:'Local',exact:true}).click()
    expect(await cards.locator('img').evaluateAll(images=>images.map(image=>image.getAttribute('src')))).toEqual(initial)
    expect(await decodes()).toBe(before)
  }
  await page.getByRole('button',{name:'Preview 01-wide.webp',exact:true}).click()
  await expect(page.locator('[data-local-metadata]')).toContainText('01-wide.webp')
  await page.locator('[data-local-metadata]').evaluate(table=>{
    const snapshots:string[]=[];(window as unknown as {metadataSnapshots:string[]}).metadataSnapshots=snapshots
    new MutationObserver(()=>snapshots.push(table.textContent??'')).observe(table,{subtree:true,childList:true,characterData:true})
  })
  await page.getByRole('button',{name:'Preview 02-square.webp',exact:true}).click()
  await expect(page.locator('[data-local-metadata]')).toContainText('02-square.webp')
  const snapshots=await page.evaluate(()=>(window as unknown as {metadataSnapshots:string[]}).metadataSnapshots)
  expect(snapshots.length).toBeGreaterThan(0)
  expect(snapshots.every(text=>text.includes('Resolution') && text.includes('Modified') && text.includes('×'))).toBe(true)
  const search=page.getByRole('textbox',{name:'Search Local wallpapers'})
  await search.fill('wide');await expect(cards).toHaveCount(1);await search.clear();await expect(cards).toHaveCount(9)
  await expect.poll(()=>cards.locator('img').count()).toBe(9)
  expect(await decodes()).toBe(before)
  await page.getByRole('button',{name:'Next',exact:true}).click();await expect(page.getByRole('button',{name:'Preview 10-sample.webp',exact:true})).toBeVisible()
  await expect.poll(()=>cards.locator('img').count()).toBe(9)
  await page.getByRole('button',{name:'Previous',exact:true}).click();await expect(page.getByRole('button',{name:'Preview 01-wide.webp',exact:true})).toBeVisible()
  await expect(page.locator('[data-selected=true]').last()).toHaveCSS('outline-style','none')
  for(const width of [375,390]){await page.setViewportSize({width,height:667});expect(await page.evaluate(()=>[document.documentElement.scrollWidth,document.documentElement.scrollHeight])).toEqual([width,667])}
  await page.screenshot({path:info.outputPath('mobile.png')})
  await page.keyboard.press('Escape');await expect.poll(()=>page.evaluate(()=>(window as unknown as {localPreviewStats:{urls:Set<string>}}).localPreviewStats.urls.size)).toBe(0)
})
