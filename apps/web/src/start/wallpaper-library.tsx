import { useEffect, useId, useRef, useState, type RefObject } from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { Tabs } from '@base-ui/react/tabs'
import { AnimatePresence, LayoutGroup, motion } from 'motion/react'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import { CheckIcon } from '@phosphor-icons/react/dist/ssr/Check'
import { PushPinIcon } from '@phosphor-icons/react/dist/ssr/PushPin'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr/WarningCircle'
import { usePreferences } from '../preferences'
import { useMotionTiming } from '../motion'
import { safeLink, wallpaperRequest, WallpaperError, type Wallpaper, type WallpaperPage } from './wallpaper-api'
import { useWallpaper } from './wallpaper-state'
import { LocalPanel, LocalDetail, readLocalPreview, type LocalPreviewCache, type LocalSelection } from './local-wallpaper-panel'
import { localMetadata, type LocalFile } from './local-wallpapers'
import styles from './wallpaper-library.module.scss'
import settings from './settings.module.scss'

export function WallpaperLibrary({ open, close, finalFocus }: { open: boolean; close: () => void; finalFocus: RefObject<HTMLButtonElement | null> }) {
  const { locale } = usePreferences(), timing = useMotionTiming(0.34)
  const heading = locale === 'zh' ? '壁纸库' : 'Wallpaper library'
  return <Dialog.Root open={open} onOpenChange={value => { if (!value) close() }}>
    <AnimatePresence>{open && <Dialog.Portal keepMounted>
      <Dialog.Backdrop className={settings.backdrop} render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={timing} />} />
      <Dialog.Popup data-wallpaper-library className={styles.dialog} finalFocus={finalFocus} render={<motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 6 }} transition={timing} />}>
        <header className={styles.header}><Dialog.Title>{heading}</Dialog.Title><Dialog.Close className={styles.iconButton} aria-label={locale === 'zh' ? '关闭壁纸库' : 'Close wallpaper library'}><XIcon size={20} /></Dialog.Close></header>
        <Dialog.Description className={styles.srOnly}>{locale === 'zh' ? '浏览不会改变背景。固定图片或应用当前来源后生效。' : 'Browsing only previews. Pin an image or apply a source to change the background.'}</Dialog.Description>
        <LibraryContent />
      </Dialog.Popup>
    </Dialog.Portal>}</AnimatePresence>
  </Dialog.Root>
}

function LibraryContent() {
  const { locale, region } = usePreferences(), zh = locale === 'zh'
  const { preference, choose, current, error } = useWallpaper()
  const [tab, setTab] = useState(preference?.source === 'local' ? 'local' : 'featured'), [visitedLocal, visitLocal] = useState(tab === 'local')
  const [query, setQuery] = useState(''), [debounced, setDebounced] = useState(''), [page, setPage] = useState(1)
  const [result, setResult] = useState<WallpaperPage | null>(null), [loading, setLoading] = useState(true), [status, setStatus] = useState('')
  const [selected, select] = useState<Wallpaper | null>(current), [selectedLocal, selectLocal] = useState<LocalSelection | null>(null)
  const [focusedLocal, focusLocal] = useState<LocalFile | null>(null), [previewing, setPreviewing] = useState(false), [previewError, setPreviewError] = useState('')
  const pages = useRef(new Map<string, { value: WallpaperPage; at: number }>()), previews = useRef<LocalPreviewCache>(new Map()), previewWork = useRef<AbortController | null>(null)
  const group = useId(), timing = useMotionTiming(0.34)
  useEffect(() => { const timer = setTimeout(() => { setDebounced(query); setPage(1) }, 300); return () => clearTimeout(timer) }, [query])
  useEffect(() => () => { previewWork.current?.abort(); previews.current.clear() }, [])
  useEffect(() => () => { if (selectedLocal) URL.revokeObjectURL(selectedLocal.url) }, [selectedLocal?.url])
  useEffect(() => {
    if (tab !== 'featured') return
    const abort = new AbortController(), key = region + ':' + page + ':' + debounced
    setStatus('')
    const cached = pages.current.get(key)
    if (cached && Date.now() - cached.at < 30000) { setResult(cached.value); setLoading(false); return }
    // Keep the displayed page and its pager together until its replacement arrives.
    setLoading(true)
    void wallpaperRequest<WallpaperPage>('?page=' + page + '&page_size=9&q=' + encodeURIComponent(debounced), region, abort.signal).then(value => {
      if (abort.signal.aborted) return
      setResult(value); setLoading(false)
      if (pages.current.size >= 3) pages.current.delete(pages.current.keys().next().value!)
      pages.current.set(key, { value, at: Date.now() })
    }).catch(e => { if (!abort.signal.aborted) { setStatus(e instanceof WallpaperError ? e.state : 'unavailable'); setLoading(false) } })
    return () => abort.abort()
  }, [region, tab, page, debounced])
  const showLocal = async (file: LocalFile) => {
    previewWork.current?.abort()
    const abort = new AbortController(); previewWork.current = abort
    focusLocal(file); setPreviewing(true); setPreviewError('')
    let url = ''
    try {
      const [image, fields] = await Promise.all([
        readLocalPreview(file, abort.signal, previews.current).then(image => { url = image.url; return image }),
        localMetadata(file, abort.signal),
      ])
      abort.signal.throwIfAborted()
      selectLocal({ file, ...image, fields }); url = ''; setPreviewing(false)
    } catch { if (!abort.signal.aborted) { setPreviewing(false); setPreviewError(zh ? '图片预览读取失败，请选择其他图片。' : 'Could not read this preview. Choose another image.') } }
    finally { if (url) URL.revokeObjectURL(url) }
  }
  const showOfficial = (item: Wallpaper) => { previewWork.current?.abort(); setPreviewing(false); setPreviewError(''); focusLocal(null); selectLocal(null); select(item) }
  const pinned = (item: Wallpaper) => preference?.source === 'official' && preference.mode === 'fixed' && preference.fixedId === item.id
  const pin = (item: Wallpaper) => choose({ version: 1, source: 'official', mode: pinned(item) ? 'random' : 'fixed', ...(!pinned(item) ? { fixedId: item.id } : {}) })
  const text = (cn: string, en: string) => zh ? cn : en
  const messages: Record<string, string> = {
    empty: text('精选目录暂无已发布壁纸。', 'No published wallpapers.'), unavailable: text('壁纸服务暂不可用，请稍后重试。', 'Wallpaper service unavailable. Try again later.'),
    network: text('网络连接失败。', 'Network request failed.'), timeout: text('请求超时，请重试。', 'Request timed out.'),
    cdn: text('壁纸图片加载失败，保留可用画面。', 'Image failed; keeping the available background.'), permission: text('请在本地页选择文件夹。', 'Choose a folder in Local.'),
    missing: text('本地文件不存在，请刷新文件列表。', 'Local file missing. Refresh the folder.'), localempty: text('所选文件夹中没有可用的静态图片。', 'No supported static images in the folder.'),
    format: text('图片无法解码、为动态格式或超出安全尺寸。', 'Image cannot be decoded, is animated, or exceeds size limits.'),
    removed: text('固定的精选壁纸已下架，已恢复精选随机。', 'Pinned wallpaper was removed. Featured random is restored.'),
  }
  const displayedPage = result?.page ?? page
  return <div className={styles.workspace}>
    <section className={styles.detail} aria-label={text('壁纸预览', 'Wallpaper preview')} aria-busy={previewing}>
      {selectedLocal ? <LocalDetail selection={selectedLocal} /> : <>
        <div className={styles.preview}>{selected ? <><LibraryImage key={selected.id} src={selected.preview.url} /><WallpaperPin preview pinned={pinned(selected)} onClick={() => pin(selected)} label={pinned(selected) ? text('取消固定', 'Unpin') : text('固定壁纸', 'Pin wallpaper')} /></> : <span>{text('选择图片预览', 'Select an image to preview')}</span>}</div>
        {selected && <>
          <h3>{selected.title}</h3>
          <dl><dt>{text('分辨率', 'Resolution')}</dt><dd>{selected.image.width} × {selected.image.height}</dd><dt>{text('格式 / 大小', 'Format / Size')}</dt><dd>{selected.image.content_type?.replace('image/', '').toUpperCase()} · {formatBytes(selected.image.bytes)}</dd>
            {selected.created_at && <><dt>{text('创建', 'Created')}</dt><dd>{formatDate(selected.created_at, locale)}</dd></>}{selected.published_at && <><dt>{text('发布', 'Published')}</dt><dd>{formatDate(selected.published_at, locale)}</dd></>}
            {selected.credit && <><dt>{text('署名', 'Credit')}</dt><dd>{selected.credit}</dd></>}
          </dl>
          {safeLink(selected.source_url) && <a href={safeLink(selected.source_url)} target="_blank" rel="noopener noreferrer">{text('来源与声明', 'Source and declaration')}</a>}
        </>}
      </>}
      {(error || previewError) && <p role="status">{previewError || messages[error] || messages.unavailable}</p>}
    </section>
    <LayoutGroup id={group}><Tabs.Root value={tab} onValueChange={value => { setTab(String(value)); if (value === 'local') visitLocal(true) }} className={styles.browser}>
      <Tabs.List className={styles.tabs} aria-label={text('壁纸来源', 'Wallpaper source')}>
        <Tabs.Tab value="featured" className={styles.tab}>{text('精选', 'Featured')}{tab === 'featured' && <motion.span className={styles.tabIndicator} layoutId="source" transition={timing} />}</Tabs.Tab>
        <Tabs.Tab value="local" className={styles.tab}>{text('本地', 'Local')}{tab === 'local' && <motion.span className={styles.tabIndicator} layoutId="source" transition={timing} />}</Tabs.Tab>
      </Tabs.List>
      <Tabs.Panel value="featured" keepMounted tabIndex={-1} className={styles.panel}>
        <div className={styles.tools}><input aria-label={text('搜索精选壁纸', 'Search Featured wallpapers')} placeholder={text('搜索标题或署名', 'Search title or credit')} value={query} maxLength={128} onChange={e => setQuery(e.target.value)} className={styles.search} /><ApplyWallpapers source="official" /></div>
        <div className={styles.grid} aria-busy={loading} inert={loading}>
          {!result && loading ? Array.from({ length: 9 }, (_, i) => <div key={i} className={styles.skeleton} />) : result?.backgrounds.map(item => <div key={item.id} className={styles.card} data-selected={!selectedLocal && selected?.id === item.id}>
            <button className={styles.cardImage} onClick={() => showOfficial(item)} aria-pressed={!selectedLocal && selected?.id === item.id} aria-label={text('预览 ', 'Preview ') + item.title}><LibraryImage src={item.preview.url} /><span className={styles.caption}>{item.title}</span></button>
            <WallpaperPin pinned={pinned(item)} onClick={() => pin(item)} label={(pinned(item) ? text('取消固定 ', 'Unpin ') : text('固定 ', 'Pin ')) + item.title} />
          </div>)}
          {status && <p role="status">{messages[status] ?? messages.unavailable}</p>}{!loading && result?.total === 0 && <p role="status">{debounced ? text('没有匹配的壁纸。', 'No matching wallpapers.') : messages.empty}</p>}
        </div>
        <footer className={styles.pagination}><button disabled={loading || displayedPage <= 1} onClick={() => setPage(displayedPage - 1)}>{text('上一页', 'Previous')}</button><span>{displayedPage} / {Math.max(1, Math.min(1000, Math.ceil((result?.total ?? 0) / 9)))}</span><button disabled={loading || !result || displayedPage * 9 >= result.total || displayedPage >= 1000} onClick={() => setPage(displayedPage + 1)}>{text('下一页', 'Next')}</button></footer>
      </Tabs.Panel>
      <Tabs.Panel value="local" keepMounted tabIndex={-1} className={styles.panel}>{visitedLocal && <LocalPanel active={tab === 'local'} selected={focusedLocal ?? selectedLocal?.file ?? null} select={file => void showLocal(file)} cache={previews.current} />}</Tabs.Panel>
    </Tabs.Root></LayoutGroup>
  </div>
}

export function WallpaperPin({ pinned, onClick, label, preview = false }: { pinned: boolean; onClick: () => void; label: string; preview?: boolean }) {
  const timing = useMotionTiming(0.34)
  return <button className={styles.pin + ' ' + (preview ? styles.previewPin : styles.cardPin)} data-pinned={pinned} aria-label={label} aria-pressed={pinned} onClick={onClick}>
    <motion.span aria-hidden="true" className={styles.pinGlyph} initial={false} animate={{ opacity: pinned ? 0 : 1 }} transition={timing}><PushPinIcon size={17} /></motion.span>
    <motion.span aria-hidden="true" className={styles.pinGlyph} initial={false} animate={{ opacity: pinned ? 1 : 0 }} transition={timing}><PushPinIcon size={17} weight="fill" /></motion.span>
  </button>
}

export function ApplyWallpapers({ source, disabled = false }: { source: 'official' | 'local'; disabled?: boolean }) {
  const { choose, preference } = useWallpaper(), { locale } = usePreferences()
  const label = source === 'official' ? locale === 'zh' ? '应用精选壁纸' : 'Use Featured wallpapers' : locale === 'zh' ? '应用本地壁纸' : 'Use Local wallpapers'
  return <button className={styles.iconButton} aria-label={label} aria-pressed={preference?.source === source && preference.mode === 'random'} disabled={disabled} onClick={() => choose({ version: 1, source, mode: 'random' })}><CheckIcon size={19} /></button>
}

export function LibraryImage({ src, instant = false }: { src: string; instant?: boolean }) {
  const [state, setState] = useState(instant ? 'ready' : 'loading'), timing = useMotionTiming(0.2)
  return <><motion.img src={src} alt="" loading="lazy" decoding="async" referrerPolicy="no-referrer" initial={false} animate={{ opacity: state === 'ready' ? 1 : 0 }} transition={timing} onLoad={e => { void e.currentTarget.decode().then(() => setState('ready'), () => setState('failed')) }} onError={() => setState('failed')} />{state === 'failed' && <span role="status"><WarningCircleIcon size={20} /></span>}</>
}
export const formatBytes = (n: number) => Number.isFinite(n) ? n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.ceil(n / 1024) + ' KB' : ''
export const formatDate = (s: string, locale: string) => new Date(s).toLocaleDateString(locale === 'zh' ? 'zh-CN' : 'en-US')
