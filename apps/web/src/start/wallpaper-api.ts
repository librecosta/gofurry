export type Asset = { url: string; width: number; height: number; bytes: number; content_type: string }
export type Wallpaper = { id: string; title: string; credit: string; source_url: string; created_at: string; published_at: string; sfw: boolean; preview: Asset; image: Asset }
export type WallpaperPage = { backgrounds: Wallpaper[]; total: number; page: number; page_size: number }
export class WallpaperError extends Error { constructor(public state: string) { super(state) } }
export const safeLink = (value: string) => { try { const u = new URL(value); return u.protocol === 'https:' && !u.username && !u.password ? u.href : undefined } catch { return undefined } }
export async function wallpaperRequest<T>(path: string, region: string, signal: AbortSignal): Promise<T> {
  let response: Response
  try { response = await fetch(`/api/v1/start/backgrounds${path}`, { signal: AbortSignal.any([signal, AbortSignal.timeout(7000)]), credentials: 'omit' }) }
  catch (e) { if (signal.aborted) throw e; throw new WallpaperError(e instanceof DOMException && e.name === 'TimeoutError' ? 'timeout' : 'network') }
  if (response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW') throw new WallpaperError('unavailable')
  if (!response.ok) throw new WallpaperError(response.status === 404 ? 'removed' : 'unavailable')
  const body = await response.json()
  const valid = (item: Wallpaper) => item && typeof item.id === 'string' && item.sfw === true && [item.preview, item.image].every(a => a && safeLink(a.url) && a.width > 0 && a.height > 0)
  if (Array.isArray(body.backgrounds) ? !body.backgrounds.every(valid) : !valid(body)) throw new WallpaperError('unavailable')
  return body as T
}
