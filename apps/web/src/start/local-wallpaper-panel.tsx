import { useEffect, useRef, useState } from 'react'
import { FolderOpenIcon } from '@phosphor-icons/react/dist/ssr/FolderOpen'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr/ArrowsClockwise'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr/WarningCircle'
import { usePreferences } from '../preferences'
import { localThumbnail, type LocalFile } from './local-wallpapers'
import { useWallpaper } from './wallpaper-state'
import { ApplyWallpapers, formatBytes, formatDate, LibraryImage, WallpaperPin } from './wallpaper-library'
import styles from './wallpaper-library.module.scss'

type SmallPreview = { blob: Blob; width: number; height: number }
export type LocalPreviewCache = Map<LocalFile, SmallPreview>
export type LocalSelection = { file: LocalFile; url: string; width: number; height: number; fields: string[][] }

// Workspace-only cache: two small pages, no original files or persisted index.
export async function readLocalPreview(file: LocalFile, signal: AbortSignal, cache: LocalPreviewCache) {
  let small = cache.get(file), url = ''
  if (small) { cache.delete(file); cache.set(file, small); url = URL.createObjectURL(small.blob) }
  else {
    const result = await localThumbnail(file, signal)
    url = result.url; small = { blob: result.blob, width: result.width, height: result.height }
    if (cache.size >= 18) cache.delete(cache.keys().next().value!)
    cache.set(file, small)
  }
  try {
    signal.throwIfAborted()
    const image = new Image(); image.src = url; await image.decode(); signal.throwIfAborted()
    return { url, width: small.width, height: small.height }
  } catch (error) { URL.revokeObjectURL(url); throw error }
}

export function LocalPanel({ active, selected, select, cache }: { active: boolean; selected: LocalFile | null; select: (file: LocalFile) => void; cache: LocalPreviewCache }) {
  const { local, preference, choose, shuffle } = useWallpaper(), { locale } = usePreferences(), zh = locale === 'zh'
  const [query, setQuery] = useState(''), [term, setTerm] = useState(''), [page, setPage] = useState(1)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => { if (active) void local.browse(); return local.cancel }, [active, local.browse, local.cancel])
  useEffect(() => { const timer = setTimeout(() => { setTerm(query.toLocaleLowerCase()); setPage(1) }, 300); return () => clearTimeout(timer) }, [query])
  const filtered = local.files.filter(file => file.path.toLocaleLowerCase().includes(term)), total = Math.max(1, Math.ceil(filtered.length / 9)), currentPage = Math.min(page, total)
  const pinned = (file: LocalFile) => preference?.source === 'local' && preference.mode === 'fixed' && preference.fixedPath === file.path
  const pin = (file: LocalFile) => choose({ version: 1, source: 'local', mode: pinned(file) ? 'random' : 'fixed', ...(!pinned(file) ? { fixedPath: file.path } : {}) })
  const resume = () => { if (preference?.source === 'local' && preference.mode !== 'off') shuffle() }
  const pick = async () => { const picked = await local.pick(); if (picked === false) input.current!.click(); else if (picked) resume() }
  const messages: Record<string, string> = {
    permission: zh ? '请选择文件夹以恢复只读访问。' : 'Choose a folder to restore read-only access.',
    scanning: zh ? '正在读取文件列表…' : 'Reading file list…',
    limited: zh ? '已达到扫描上限，请选择较小的文件夹。' : 'Scan limit reached. Choose a smaller folder.',
    scanfailed: zh ? '读取文件夹失败，请重新选择。' : 'Folder could not be read. Choose it again.',
  }
  return <div className={styles.panel}>
    <div className={styles.tools}>
      <input ref={element => { input.current = element; element?.setAttribute('webkitdirectory', '') }} data-local-folder type="file" multiple accept="image/jpeg,image/png,image/webp,image/avif" hidden onChange={event => { if (event.target.files) { local.fallback(event.target.files); cache.clear(); resume() }; event.target.value = '' }} />
      <input className={styles.search} aria-label={zh ? '搜索本地壁纸' : 'Search Local wallpapers'} placeholder={zh ? '搜索文件名或相对路径' : 'Search filename or relative path'} value={query} onChange={event => setQuery(event.target.value)} maxLength={128} />
      <button className={styles.iconButton} aria-label={zh ? '选择文件夹' : 'Choose folder'} onClick={() => void pick()}><FolderOpenIcon size={19} /></button>
      <button className={styles.iconButton} aria-label={zh ? '刷新列表' : 'Refresh files'} disabled={local.status === 'scanning'} onClick={() => { if (!local.persistent) input.current!.click(); else void local.scan().then(() => cache.clear()).catch(() => {}) }}><ArrowsClockwiseIcon size={19} /></button>
      <ApplyWallpapers source="local" disabled={!local.files.length || local.status === 'scanning'} />
    </div>
    <div className={styles.gridArea}>
      {(!local.persistent || local.status) && <p className={styles.localNotice} role="status">{local.status ? messages[local.status] : zh ? '兼容模式：刷新后可能需要重新选择文件夹。' : 'Compatibility mode: select the folder again after reloading.'}</p>}
      <div className={styles.grid} aria-busy={local.status === 'scanning'}>
        {!local.files.length && local.status === 'scanning' ? Array.from({ length: 9 }, (_, i) => <div key={i} className={styles.skeleton} />) : filtered.slice((currentPage - 1) * 9, currentPage * 9).map(file => <div className={styles.card} key={file.path} data-selected={selected?.path === file.path}>
          <button className={styles.cardImage} aria-label={zh ? '预览 ' + file.name : 'Preview ' + file.name} aria-pressed={selected?.path === file.path} onClick={() => select(file)}><LocalThumbnail file={file} cache={cache} active={active} /><span className={styles.caption}>{file.name}</span></button>
          <WallpaperPin pinned={pinned(file)} label={(pinned(file) ? zh ? '取消固定 ' : 'Unpin ' : zh ? '固定 ' : 'Pin ') + file.name} onClick={() => pin(file)} />
        </div>)}
        {!filtered.length && local.status !== 'scanning' && <p>{zh ? '尚无可浏览的本地图片。' : 'No local images to browse.'}</p>}
      </div>
    </div>
    <footer className={styles.pagination}><button disabled={currentPage <= 1} onClick={() => setPage(currentPage - 1)}>{zh ? '上一页' : 'Previous'}</button><span>{currentPage} / {total}</span><button disabled={currentPage >= total} onClick={() => setPage(currentPage + 1)}>{zh ? '下一页' : 'Next'}</button></footer>
  </div>
}

function LocalThumbnail({ file, cache, active }: { file: LocalFile; cache: LocalPreviewCache; active: boolean }) {
  const [image, setImage] = useState<{ file: LocalFile; url: string; failed?: boolean } | null>(null)
  useEffect(() => {
    if (!active || image?.file === file) return
    const abort = new AbortController()
    void readLocalPreview(file, abort.signal, cache).then(result => {
      if (abort.signal.aborted) URL.revokeObjectURL(result.url)
      else setImage({ file, url: result.url })
    }).catch(() => { if (!abort.signal.aborted) setImage({ file, url: '', failed: true }) })
    return () => abort.abort()
  }, [file, active, cache, image?.file])
  useEffect(() => () => { if (image?.url) URL.revokeObjectURL(image.url) }, [image?.url])
  return image?.file === file ? image.failed ? <span role="status"><WarningCircleIcon size={20} /></span> : <LibraryImage src={image.url} instant /> : <span className={styles.pendingImage} aria-busy="true" />
}

export function LocalDetail({ selection }: { selection: LocalSelection }) {
  const { preference, choose } = useWallpaper(), { locale } = usePreferences(), zh = locale === 'zh', { file, width, height, fields, url } = selection
  const pinned = preference?.source === 'local' && preference.mode === 'fixed' && preference.fixedPath === file.path
  return <>
    <div className={styles.preview}><LibraryImage key={url} src={url} instant /><WallpaperPin preview pinned={pinned} label={pinned ? zh ? '取消固定' : 'Unpin' : zh ? '固定壁纸' : 'Pin wallpaper'} onClick={() => choose({ version: 1, source: 'local', mode: pinned ? 'random' : 'fixed', ...(!pinned ? { fixedPath: file.path } : {}) })} /></div>
    <h3>{file.name}</h3>
    <dl data-local-metadata><dt>{zh ? '相对路径' : 'Relative path'}</dt><dd>{file.path}</dd><dt>{zh ? '格式 / 大小' : 'Format / Size'}</dt><dd>{file.name.split('.').at(-1)?.toUpperCase()} · {formatBytes(file.size)}</dd><dt>{zh ? '分辨率' : 'Resolution'}</dt><dd>{width} × {height}</dd><dt>{zh ? '修改时间' : 'Modified'}</dt><dd>{formatDate(new Date(file.modified).toISOString(), locale)}</dd>{fields.map(([key, value]) => <div className={styles.metadataRow} key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>
  </>
}
