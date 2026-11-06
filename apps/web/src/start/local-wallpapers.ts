import { useCallback, useEffect, useRef, useState } from 'react'

export interface DirectoryHandle {
  kind: 'directory'; name: string
  values(): AsyncIterable<DirectoryHandle | FileHandle>
  getDirectoryHandle(name: string): Promise<DirectoryHandle>
  getFileHandle(name: string): Promise<FileHandle>
  queryPermission(options: { mode: 'read' }): Promise<PermissionState>
  requestPermission(options: { mode: 'read' }): Promise<PermissionState>
}
interface FileHandle { kind: 'file'; name: string; getFile(): Promise<File> }
export type LocalFile = { path: string; name: string; size: number; modified: number; getFile: () => Promise<File> }
const supported = /\.(jpe?g|png|webp|avif)$/i
const aborted = (signal: AbortSignal) => signal.throwIfAborted()
async function handleStore(region: string, value?: DirectoryHandle) {
  const db = await new Promise<IDBDatabase>((resolve, reject) => { const r = indexedDB.open('gf-wallpaper-folders', 1); r.onupgradeneeded = () => r.result.createObjectStore('handles'); r.onsuccess = () => resolve(r.result); r.onerror = () => reject(r.error); r.onblocked = () => reject(new Error('blocked')) })
  try { return await new Promise<DirectoryHandle | undefined>((resolve, reject) => { const tx = db.transaction('handles', value ? 'readwrite' : 'readonly'), store = tx.objectStore('handles'), r = value ? store.put(value, region) : store.get(region); tx.oncomplete = () => resolve(value ?? r.result); tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error) }) } finally { db.close() }
}
export function useLocalFolder(region: string) {
  const root = useRef<DirectoryHandle | undefined>(undefined), index = useRef<LocalFile[]>([]), work = useRef<AbortController | null>(null)
  const [files, setFiles] = useState<LocalFile[]>([]), [status, setStatus] = useState(''), [persistent, setPersistent] = useState(true)
  const cancel = useCallback(() => { work.current?.abort(); work.current = null }, [])
  useEffect(() => () => cancel(), [cancel])
  const getRoot = useCallback(async () => {
    const handle = root.current ?? await handleStore(region)
    if (!handle || await handle.queryPermission({ mode: 'read' }) !== 'granted') throw new Error('permission')
    root.current = handle; return handle
  }, [region])
  const scan = useCallback(async (signal?: AbortSignal, provided?: DirectoryHandle) => {
    cancel(); const controller = new AbortController(); work.current = controller
    const combined = signal ? AbortSignal.any([signal, controller.signal]) : controller.signal
    setStatus('scanning')
    try {
      const handle = provided ?? await getRoot(), out: LocalFile[] = []
      let visited = 0, limited = false
      async function walk(dir: DirectoryHandle, prefix: string, depth: number) {
        if (depth > 16) { limited = true; return }
        for await (const entry of dir.values()) {
          aborted(combined)
          if (++visited > 10000 || out.length >= 2000) { limited = true; return }
          const path = prefix + entry.name
          if (entry.kind === 'directory') await walk(entry, `${path}/`, depth + 1)
          else if (supported.test(entry.name)) {
            const file = await entry.getFile(); aborted(combined)
            if (file.size <= 50 * 1024 * 1024) out.push({ path, name: entry.name, size: file.size, modified: file.lastModified, getFile: () => entry.getFile() })
          }
          // Yield during metadata-only traversal; no bulk image decoding.
          if (visited % 32 === 0) await new Promise(resolve => setTimeout(resolve, 0))
          if (limited) return
        }
      }
      await walk(handle, '', 0); aborted(combined)
      out.sort((a,b) => a.path.localeCompare(b.path)); index.current = out; setFiles(out); setStatus(limited ? 'limited' : '')
      return out
    } catch (e) { if (!combined.aborted) setStatus(e instanceof Error && e.message === 'permission' ? 'permission' : 'scanfailed'); throw e }
  }, [cancel, getRoot])
  const browse = useCallback(async () => { if (!index.current.length) try { await scan() } catch { /* Local status explains the failure. */ } }, [scan])
  const pick = useCallback(async () => {
    try {
      const picker = (window as Window & { showDirectoryPicker?: (options: { mode: 'read'; id: string }) => Promise<DirectoryHandle> }).showDirectoryPicker
      if (!picker) return false
      const handle = await picker({ mode: 'read', id: `gofurry-wallpaper-${region}` })
      if (await handle.requestPermission({ mode: 'read' }) !== 'granted') { setStatus('permission'); return true }
      root.current = handle; setPersistent(true)
      try { await handleStore(region, handle) } catch { setPersistent(false) }
      await scan(undefined, handle); return true
    } catch (e) { if (!(e instanceof DOMException && e.name === 'AbortError')) setStatus('permission'); return true }
  }, [region, scan])
  const authorize = useCallback(async () => {
    try { const handle = root.current ?? await handleStore(region); if (!handle || await handle.requestPermission({ mode: 'read' }) !== 'granted') { setStatus('permission'); return }; root.current=handle; await scan(undefined,handle) } catch { setStatus('permission') }
  }, [region, scan])
  const fallback = useCallback((selected: FileList) => {
    cancel(); root.current=undefined; setPersistent(false)
    const out=Array.from(selected).filter(f=>supported.test(f.name) && f.size<=50*1024*1024).slice(0,2000).map(file=>({path:file.webkitRelativePath.split('/').slice(1).join('/') || file.name,name:file.name,size:file.size,modified:file.lastModified,getFile:async()=>file}))
    index.current=out;setFiles(out);setStatus(selected.length>2000?'limited':'')
  }, [cancel])
  const resolve = useCallback(async (path: string | undefined, signal: AbortSignal) => {
    let file: File
    if (path) {
      const existing=index.current.find(f=>f.path===path)
      if (existing) file=await existing.getFile()
      else { let dir=await getRoot(); const parts=path.split('/'); for (const part of parts.slice(0,-1)) dir=await dir.getDirectoryHandle(part); file=await (await dir.getFileHandle(parts.at(-1)!)).getFile() }
    } else {
      const items=index.current.length?index.current:await scan(signal)
      if (!items.length) throw new Error('localempty')
      file=await items[Math.floor(Math.random()*items.length)].getFile()
    }
    aborted(signal); await validateLocal(file); aborted(signal); return file
  }, [getRoot, scan])
  return { files, status, persistent, pick, authorize, fallback, browse, scan, cancel, resolve }
}

// Reject animated variants and large files before invoking a decoder. No EXIF/GPS upload.
export async function validateLocal(file: File) {
  if (!supported.test(file.name) || file.size > 50*1024*1024) throw new Error('format')
  const header = new Uint8Array(await file.slice(0,524288).arrayBuffer()), text = new TextDecoder('latin1').decode(header)
  if (/\.webp$/i.test(file.name) && text.includes('ANIM') || /\.png$/i.test(file.name) && text.includes('acTL') || /\.avif$/i.test(file.name) && text.slice(0,128).includes('avis')) throw new Error('format')
}
// A single decoder budget is shared by page thumbnails and selected-file metadata.
let decodeTail: Promise<unknown> = Promise.resolve()
export async function localThumbnail(entry: LocalFile, signal: AbortSignal) {
  const task = decodeTail.catch(() => {}).then(async () => {
    aborted(signal); const file=await entry.getFile(); await validateLocal(file); aborted(signal)
    const bitmap=await createImageBitmap(file)
    try {
      aborted(signal); if (bitmap.width*bitmap.height>36000000 || bitmap.width>16384 || bitmap.height>16384) throw new Error('format')
      const canvas=document.createElement('canvas'), ratio=Math.min(1,480/bitmap.width,270/bitmap.height)
      canvas.width=Math.max(1,Math.round(bitmap.width*ratio));canvas.height=Math.max(1,Math.round(bitmap.height*ratio));canvas.getContext('2d')!.drawImage(bitmap,0,0,canvas.width,canvas.height)
      const blob=await new Promise<Blob>((resolve,reject)=>canvas.toBlob(b=>b?resolve(b):reject(new Error('decode')),'image/webp',.78)); aborted(signal)
      return { url:URL.createObjectURL(blob),width:bitmap.width,height:bitmap.height }
    } finally { bitmap.close() }
  });decodeTail=task;return task
}
export async function localMetadata(entry: LocalFile, signal: AbortSignal) {
  const file=await entry.getFile();await validateLocal(file);aborted(signal)
  const { parse }=await import('exifr')
  const fields=['DateTimeOriginal','CreateDate','Make','Model','LensModel','FNumber','ISO','ExposureTime','Artist','Copyright']
  let exif: Record<string,unknown> = {}
  try { exif=await parse(file,{pick:fields,gps:false,icc:false,xmp:false,iptc:false}) ?? {} } catch { /* Basic metadata remains available for unsupported formats. */ }
  aborted(signal);return Object.entries(exif).filter(([key,value])=>fields.includes(key) && value!==undefined).map(([key,value])=>[key,value instanceof Date?value.toLocaleString():String(value).slice(0,500)])
}
