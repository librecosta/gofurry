import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { usePreferences } from '../preferences'

export type PinnedSite = { id: string; name: string; url: string; order: number }
export type RecentSite = { url: string; name: string; lastVisited: number; count: number }
export type QuickData = { version: 1; pinned: PinnedSite[]; recent: RecentSite[] }
export type QuickSite = { key: string; name: string; url: string; pin?: PinnedSite }
const empty = (): QuickData => ({ version: 1, pinned: [], recent: [] })
const maxPinned = 128, maxRecent = 64

export function siteURL(value: string): URL | null {
  if (value.length > 2048 || /[\p{Cc}\p{Cf}]/u.test(value)) return null
  try {
    const url = new URL(/^[a-z][a-z\d+.-]*:/i.test(value.trim()) ? value.trim() : `https://${value.trim()}`)
    return ['http:', 'https:'].includes(url.protocol) && url.hostname && !url.username && !url.password ? url : null
  } catch { return null }
}
export function siteKey(value: string) {
  const url = siteURL(value)
  return url ? url.hostname.replace(/^www\./, '').toLowerCase() + (url.port ? `:${url.port}` : '') : ''
}
function validName(value: unknown): value is string { return typeof value === 'string' && !!value.trim() && [...value].length <= 80 && !/[\p{Cc}\p{Cf}]/u.test(value) }

export function parseQuickData(raw: string | null): QuickData {
  if (!raw || raw.length > 256 * 1024) return empty()
  try {
    const value = JSON.parse(raw) as QuickData
    if (value?.version !== 1 || !Array.isArray(value.pinned) || !Array.isArray(value.recent)) return empty()
    const keys = new Set<string>(), ids = new Set<string>()
    const pinned = value.pinned.filter(p => {
      if (!p || typeof p.id !== 'string' || !/^[\w-]{1,80}$/.test(p.id) || ids.has(p.id) || !validName(p.name) || typeof p.url !== 'string' || !siteURL(p.url) || keys.has(siteKey(p.url)) || !Number.isSafeInteger(p.order) || p.order < 0) return false
      ids.add(p.id); keys.add(siteKey(p.url)); return true
    }).sort((a, b) => a.order - b.order).slice(0, maxPinned).map((p, order) => ({ id: p.id, name: p.name.trim(), url: siteURL(p.url)!.href, order }))
    keys.clear()
    const recent = value.recent.filter(r => {
      if (!r || !validName(r.name) || typeof r.url !== 'string' || !siteURL(r.url) || keys.has(siteKey(r.url)) || !Number.isSafeInteger(r.count) || r.count < 1 || !Number.isFinite(r.lastVisited) || r.lastVisited < 1 || r.lastVisited > Date.now() + 60000) return false
      keys.add(siteKey(r.url)); return true
    }).sort((a, b) => b.lastVisited - a.lastVisited).slice(0, maxRecent).map(r => ({ url: siteURL(r.url)!.origin + '/', name: r.name.trim(), lastVisited: r.lastVisited, count: r.count }))
    return { version: 1, pinned, recent }
  } catch { return empty() }
}

export function visibleSites(data: QuickData): QuickSite[] {
  const keys = new Set(data.pinned.map(p => siteKey(p.url)))
  return [
    ...data.pinned.map(pin => ({ key: siteKey(pin.url), name: pin.name, url: pin.url, pin })),
    ...data.recent.filter(r => !keys.has(siteKey(r.url))).map(r => ({ key: siteKey(r.url), name: r.name, url: r.url })),
  ].slice(0, 12)
}

type Context = {
  data: QuickData; ready: boolean
  save: (name: string, url: string, id?: string) => boolean
  pin: (site: QuickSite) => void; unpin: (id: string) => void; remove: (key: string) => void
  removeRecent: (key: string) => void; reorder: (sites: PinnedSite[]) => void
  openSite: (url: string, name: string) => void
}
const QuickContext = createContext<Context | null>(null)

export function QuickAccessProvider({ children }: { children: ReactNode }) {
  const { region, quickAccess } = usePreferences()
  const storageKey = `gf-quick-access:${region}`
  const [data, setData] = useState<QuickData>(empty)
  const current = useRef(data)
  const [ready, setReady] = useState(false)
  function commit(update: (old: QuickData) => QuickData) {
    const next = update(current.current)
    current.current = next; setData(next)
    try { localStorage.setItem(storageKey, JSON.stringify({ ...next, pinned: next.pinned.map((p, order) => ({ ...p, order })) })) } catch { /* Memory-only fallback. */ }
  }
  function record(url: string, name: string) {
    if (!quickAccess || !ready) return
    const parsed = siteURL(url)
    if (!parsed) return
    const key = siteKey(url)
    commit(old => {
      const previous = old.recent.find(r => siteKey(r.url) === key)
      return { ...old, recent: [{ url: parsed.origin + '/', name: validName(name) ? name.trim() : parsed.hostname, lastVisited: Date.now(), count: Math.min(Number.MAX_SAFE_INTEGER, (previous?.count ?? 0) + 1) }, ...old.recent.filter(r => siteKey(r.url) !== key)].slice(0, maxRecent) }
    })
  }
  useEffect(() => {
    let loaded = empty()
    try { loaded = parseQuickData(localStorage.getItem(storageKey)) } catch { /* Storage is optional. */ }
    current.current = loaded; setData(loaded); setReady(true)
    const sync = (event: StorageEvent) => {
      if (event.key !== storageKey && event.key !== null) return
      const next = parseQuickData(event.newValue)
      current.current = next; setData(next)
    }
    window.addEventListener('storage', sync)
    return () => window.removeEventListener('storage', sync)
  }, [storageKey])
  useEffect(() => {
    // Only deliberate GoFurry link activations, including portal links/middle
    // clicks. Search submissions use window.open and never pass through here.
    const clicked = (event: MouseEvent) => {
      if (!event.isTrusted || event.defaultPrevented || (event.type === 'click' ? event.button !== 0 : event.button !== 1)) return
      const link = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>('a[href]') : null
      if (!link || link.hasAttribute('download')) return
      record(link.href, link.dataset.quickName ?? link.textContent?.trim() ?? '')
    }
    document.addEventListener('click', clicked, true); document.addEventListener('auxclick', clicked, true)
    return () => { document.removeEventListener('click', clicked, true); document.removeEventListener('auxclick', clicked, true) }
  }, [quickAccess, ready, storageKey])
  const save: Context['save'] = (name, value, id) => {
    const url = siteURL(value)
    if (!validName(name) || !url || (!id && current.current.pinned.length >= maxPinned)) return false
    const key = siteKey(url.href)
    if (id && !current.current.pinned.some(p => p.id === id)) return false
    if (current.current.pinned.some(p => p.id !== id && siteKey(p.url) === key)) return false
    commit(old => ({ ...old, pinned: id ? old.pinned.map(p => p.id === id ? { ...p, name: name.trim(), url: url.href } : p) : [...old.pinned, { id: crypto.randomUUID(), name: name.trim(), url: url.href, order: old.pinned.length }] }))
    return true
  }
  return <QuickContext value={{ data, ready, save,
    pin: site => { if (!current.current.pinned.some(p => siteKey(p.url) === site.key)) save(site.name, site.url) },
    unpin: id => commit(old => ({ ...old, pinned: old.pinned.filter(p => p.id !== id) })),
    remove: key => commit(old => ({ ...old, pinned: old.pinned.filter(p => siteKey(p.url) !== key), recent: old.recent.filter(r => siteKey(r.url) !== key) })),
    removeRecent: key => commit(old => ({ ...old, recent: old.recent.filter(r => siteKey(r.url) !== key) })),
    reorder: pinned => commit(old => pinned.length === old.pinned.length && new Set(pinned.map(p => p.id)).size === pinned.length && pinned.every(p => old.pinned.includes(p)) ? { ...old, pinned } : old),
    openSite: (url, name) => { if (siteURL(url)) { record(url, name); window.open(url, '_blank', 'noopener,noreferrer') } },
  }}>{children}</QuickContext>
}

export function useQuickAccess() {
  const value = useContext(QuickContext)
  if (!value) throw new Error('QuickAccessProvider is required')
  return value
}
