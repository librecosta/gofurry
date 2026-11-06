import { useEffect, useRef, useState, type RefObject } from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { Tabs } from '@base-ui/react/tabs'
import { AnimatePresence, motion } from 'motion/react'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import { PushPinIcon } from '@phosphor-icons/react/dist/ssr/PushPin'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr/WarningCircle'
import { usePreferences } from '../preferences'
import { useMotionTiming } from '../motion'
import { safeLink, wallpaperRequest, WallpaperError, type Wallpaper, type WallpaperPage } from './wallpaper-api'
import { useWallpaper } from './wallpaper-state'
import styles from './wallpaper-library.module.scss'
import settings from './settings.module.scss'

export function WallpaperLibrary({ open, close, finalFocus }: { open: boolean; close: () => void; finalFocus: RefObject<HTMLButtonElement | null> }) {
  const { locale } = usePreferences(), timing = useMotionTiming()
  const title = locale === 'zh' ? '壁纸库' : 'Wallpaper library'
  return <Dialog.Root open={open} onOpenChange={value => { if (!value) close() }}>
    <AnimatePresence>{open && <Dialog.Portal keepMounted>
      <Dialog.Backdrop className={settings.backdrop} render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={timing} />} />
      <Dialog.Popup data-wallpaper-library className={styles.dialog} finalFocus={finalFocus} render={<motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 6 }} transition={timing} />}>
        <header className={styles.header}><Dialog.Title>{title}</Dialog.Title><Dialog.Close className={styles.iconButton} aria-label={locale === 'zh' ? '关闭壁纸库' : 'Close wallpaper library'}><XIcon size={20} /></Dialog.Close></header>
        <Dialog.Description className={styles.srOnly}>{locale === 'zh' ? '浏览不会改变背景。固定或选择随机模式后生效。' : 'Browsing only previews. Pin an image or choose a random mode to apply it.'}</Dialog.Description>
        <LibraryContent />
      </Dialog.Popup>
    </Dialog.Portal>}</AnimatePresence>
  </Dialog.Root>
}

function LibraryContent() {
  const { locale, region } = usePreferences(), zh = locale === 'zh'
  const { preference, choose, shuffle, current, error } = useWallpaper()
  const [tab, setTab] = useState('featured'), [query, setQuery] = useState(''), [debounced, setDebounced] = useState(''), [page, setPage] = useState(1)
  const [result, setResult] = useState<WallpaperPage | null>(null), [status, setStatus] = useState(''), [selected, select] = useState<Wallpaper | null>(current), [hd, setHD] = useState(false)
  const pages = useRef(new Map<string, { value: WallpaperPage; at: number }>())
  useEffect(() => { const timer = setTimeout(() => { setDebounced(query); setPage(1) }, 300); return () => clearTimeout(timer) }, [query])
  useEffect(() => {
    if (tab !== 'featured') return
    const abort = new AbortController(), key = `${region}:${page}:${debounced}`
    setStatus(''); setResult(null)
    const cached = pages.current.get(key)
    if (cached && Date.now() - cached.at < 30000) { setResult(cached.value); return }
    void wallpaperRequest<WallpaperPage>(`?page=${page}&page_size=9&q=${encodeURIComponent(debounced)}`, region, abort.signal).then(value => {
      if (abort.signal.aborted) return
      setResult(value)
      if (pages.current.size >= 3) pages.current.delete(pages.current.keys().next().value!)
      pages.current.set(key, { value, at: Date.now() })
    }).catch(e => { if (!abort.signal.aborted) setStatus(e instanceof WallpaperError ? e.state : 'unavailable') })
    return () => abort.abort()
  }, [region, tab, page, debounced])
  const pinned = (item: Wallpaper) => preference?.source === 'official' && preference.mode === 'fixed' && preference.fixedId === item.id
  const pin = (item: Wallpaper) => choose({ version: 1, source: 'official', mode: pinned(item) ? 'random' : 'fixed', ...(!pinned(item) ? { fixedId: item.id } : {}) })
  const text = (cn: string, en: string) => zh ? cn : en
  const messages: Record<string, string> = { empty: text('精选目录暂无已发布壁纸。', 'No published wallpapers.'), unavailable: text('壁纸服务暂不可用，请稍后重试。', 'Wallpaper service unavailable. Try again later.'), network: text('网络连接失败。', 'Network request failed.'), timeout: text('请求超时，请重试。', 'Request timed out.'), cdn: text('壁纸图片加载失败，保留可用画面。', 'Image failed; keeping the available background.'), permission: text('请在本地页重新选择或授权文件夹。', 'Choose or authorize a folder in Local.'), missing: text('本地文件不存在，请刷新文件列表。', 'Local file missing. Refresh the folder.') }
  return <div className={styles.workspace}>
    <section className={styles.detail} aria-label={text('壁纸预览', 'Wallpaper preview')}>
      <div className={styles.preview}>{selected ? <LibraryImage key={`${selected.id}:${hd}`} src={hd ? selected.image.url : selected.preview.url} /> : <span>{text('选择图片预览', 'Select an image to preview')}</span>}</div>
      {selected && <>
        <h3>{selected.title}</h3><p>{pinned(selected) ? text('已固定', 'Pinned') : text('仅预览', 'Preview only')}</p>
        <dl><dt>{text('分辨率', 'Resolution')}</dt><dd>{selected.image.width} × {selected.image.height}</dd><dt>{text('格式 / 大小', 'Format / Size')}</dt><dd>{selected.image.content_type?.replace('image/', '').toUpperCase()} · {formatBytes(selected.image.bytes)}</dd>
          {selected.created_at && <><dt>{text('创建', 'Created')}</dt><dd>{formatDate(selected.created_at, locale)}</dd></>}{selected.published_at && <><dt>{text('发布', 'Published')}</dt><dd>{formatDate(selected.published_at, locale)}</dd></>}
          {selected.credit && <><dt>{text('署名', 'Credit')}</dt><dd>{selected.credit}</dd></>}
        </dl>
        {safeLink(selected.source_url) && <a href={safeLink(selected.source_url)} target="_blank" rel="noopener noreferrer">{text('来源与声明', 'Source and declaration')}</a>}
        <div className={styles.actions}><button onClick={() => pin(selected)}>{pinned(selected) ? text('取消固定', 'Unpin') : text('固定壁纸', 'Pin wallpaper')}</button><button onClick={() => setHD(true)} disabled={hd}>{text('查看高清', 'View HD')}</button></div>
      </>}
      <div className={styles.actions}><button onClick={() => { choose({ version: 1, source: 'official', mode: 'random' }); shuffle() }}>{text('使用精选随机', 'Use Featured random')}</button><button onClick={() => choose({ version: 1, source: preference?.source ?? 'official', mode: 'off' })}>{text('关闭壁纸', 'Turn off wallpaper')}</button></div>
      {error && <p role="status">{messages[error] ?? messages.unavailable}</p>}
    </section>
    <Tabs.Root value={tab} onValueChange={value => setTab(String(value))} className={styles.browser}>
      <Tabs.List className={styles.tabs} aria-label={text('壁纸来源', 'Wallpaper source')}><Tabs.Tab value="featured">{text('精选', 'Featured')}</Tabs.Tab><Tabs.Tab value="local">{text('本地', 'Local')}</Tabs.Tab></Tabs.List>
      <Tabs.Panel value="featured" className={styles.panel}>
        <input aria-label={text('搜索精选壁纸', 'Search Featured wallpapers')} placeholder={text('搜索标题或署名', 'Search title or credit')} value={query} maxLength={128} onChange={e => setQuery(e.target.value)} className={styles.search} />
        <div className={styles.grid} aria-busy={!result && !status}>
          {!result && !status ? Array.from({ length: 9 }, (_, i) => <div key={i} className={styles.skeleton} />) : result?.backgrounds.map(item => <div key={item.id} className={styles.card} data-selected={selected?.id === item.id}>
            <button className={styles.cardImage} onClick={() => { select(item); setHD(false) }} aria-label={`${text('预览', 'Preview')} ${item.title}`}><LibraryImage src={item.preview.url} /><span className={styles.caption}>{item.title}</span></button>
            <button className={styles.pin} data-pinned={pinned(item)} onClick={() => pin(item)} aria-label={`${pinned(item) ? text('取消固定', 'Unpin') : text('固定', 'Pin')} ${item.title}`} aria-pressed={pinned(item)}><PushPinIcon size={16} weight={pinned(item) ? 'fill' : 'regular'} /></button>
          </div>)}
          {status && <p role="status">{messages[status] ?? messages.unavailable}</p>}{result?.total === 0 && <p role="status">{messages.empty}</p>}
        </div>
        <footer className={styles.pagination}><button disabled={page <= 1} onClick={() => setPage(n => n - 1)}>{text('上一页', 'Previous')}</button><span>{page} / {Math.max(1, Math.min(1000, Math.ceil((result?.total ?? 0) / 9)))}</span><button disabled={!result || page * 9 >= result.total || page >= 1000} onClick={() => setPage(n => n + 1)}>{text('下一页', 'Next')}</button></footer>
      </Tabs.Panel>
      <Tabs.Panel value="local" className={styles.panel}><p>{text('选择本地文件夹，仅在此设备上读取。', 'Choose a local folder. Files stay on this device.')}</p></Tabs.Panel>
    </Tabs.Root>
  </div>
}
export function LibraryImage({ src }: { src: string }) {
  const [state, setState] = useState('loading'), timing = useMotionTiming(0.2)
  return <><motion.img src={src} alt="" loading="lazy" decoding="async" referrerPolicy="no-referrer" initial={{ opacity: 0 }} animate={{ opacity: state === 'ready' ? 1 : 0 }} transition={timing} onLoad={e => { void e.currentTarget.decode().then(() => setState('ready'), () => setState('failed')) }} onError={() => setState('failed')} />{state === 'failed' && <span role="status"><WarningCircleIcon size={20} /></span>}</>
}
export const formatBytes = (n: number) => Number.isFinite(n) ? n >= 1048576 ? `${(n / 1048576).toFixed(1)} MB` : `${Math.ceil(n / 1024)} KB` : ''
export const formatDate = (s: string, locale: string) => new Date(s).toLocaleDateString(locale === 'zh' ? 'zh-CN' : 'en-US')
