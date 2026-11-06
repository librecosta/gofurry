import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { usePreferences } from '../preferences'
import { WallpaperError, wallpaperRequest, type Wallpaper } from './wallpaper-api'
import { useLocalFolder } from './local-wallpapers'

export type WallpaperPreference = { version: 1; source: 'official' | 'local'; mode: 'random' | 'fixed' | 'off'; fixedId?: string; fixedPath?: string }
export type BackgroundResource = { key: string; preview: string; image: string; release?: () => void }
const defaults: WallpaperPreference = { version: 1, source: 'official', mode: 'random' }
export function parseWallpaperPreference(raw: string | null): WallpaperPreference {
  try {
    if (!raw || raw.length > 4096) return defaults
    const p = JSON.parse(raw)
    if (p.version !== 1 || !['official', 'local'].includes(p.source) || !['fixed', 'random', 'off'].includes(p.mode)) return defaults
    if (p.mode === 'fixed' && (p.source === 'official' ? !/^[a-f\d-]{36}$/i.test(p.fixedId ?? '') : typeof p.fixedPath !== 'string' || !p.fixedPath || p.fixedPath.length > 2048 || p.fixedPath.split('/').some((s: string) => s === '..' || !s))) return defaults
    return { version: 1, source: p.source, mode: p.mode, ...(p.mode === 'fixed' ? p.source === 'official' ? { fixedId: p.fixedId } : { fixedPath: p.fixedPath } : {}) }
  } catch { return defaults }
}
type Controller = { preference: WallpaperPreference | null; choose: (p: WallpaperPreference) => void; shuffle: () => void; resource: BackgroundResource | null; current: Wallpaper | null; error: string; setError: (s: string) => void; local: ReturnType<typeof useLocalFolder> }
const Context = createContext<Controller | null>(null)
export function WallpaperProvider({ children }: { children: ReactNode }) {
  const { region } = usePreferences()
  const local = useLocalFolder(region)
  const [preference, setPreference] = useState<WallpaperPreference | null>(null), [revision, refresh] = useState(0)
  const [resource, setResource] = useState<BackgroundResource | null>(null), [current, setCurrent] = useState<Wallpaper | null>(null), [error, setError] = useState('')
  const choose = (p: WallpaperPreference) => { setPreference(p); try { localStorage.setItem(`gf-wallpaper:${region}`, JSON.stringify(p)) } catch { /* Memory-only fallback. */ } }
  useEffect(() => { let p = defaults; try { p = parseWallpaperPreference(localStorage.getItem(`gf-wallpaper:${region}`)) } catch { /* Storage unavailable. */ } setPreference(p) }, [region])
  useEffect(() => {
    if (!preference) return
    const abort = new AbortController()
    setError('')
    if (preference.mode === 'off') { setResource(null); setCurrent(null); return }
    if (preference.source === 'local') {
      void local.resolve(preference.mode === 'fixed' ? preference.fixedPath : undefined, abort.signal).then(file => {
        if (abort.signal.aborted) return
        const url = URL.createObjectURL(file)
        setCurrent(null); setResource({ key: url, preview: url, image: url, release: () => URL.revokeObjectURL(url) })
      }).catch(e => { if (!abort.signal.aborted) { setResource(null); setError(e instanceof DOMException && e.name === 'NotFoundError' ? 'missing' : e instanceof Error && ['localempty','format'].includes(e.message) ? e.message : 'permission') } })
      return () => abort.abort()
    }
    const path = preference.mode === 'fixed' ? `/${preference.fixedId}` : `/random${current ? `?exclude=${current.id}` : ''}`
    void wallpaperRequest<Wallpaper>(path, region, abort.signal).then(item => {
      if (!abort.signal.aborted) { setCurrent(item); setResource({ key: item.id, preview: item.preview.url, image: item.image.url }) }
    }).catch(e => {
      if (abort.signal.aborted) return
      if (e instanceof WallpaperError && e.state === 'removed' && preference.mode === 'fixed') choose(defaults)
      else setError(e instanceof WallpaperError ? e.state === 'removed' ? 'empty' : e.state : 'unavailable')
    })
    return () => abort.abort()
    // A stable session selection: locale/theme/resize/current metadata never rerandomize.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preference, revision, region])
  return <Context value={{ preference, choose, shuffle: () => refresh(n => n + 1), resource, current, error, setError, local }}>{children}</Context>
}
export function useWallpaper() { const value = useContext(Context); if (!value) throw new Error('WallpaperProvider required'); return value }
