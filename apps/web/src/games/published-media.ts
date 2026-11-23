import type { PublicGameMedia, PublicGameMediaImage, SteamMovieSources } from './profile-data'

const steamDomains = ['steamstatic.com', 'steamcontent.com']
const steamHosts = ['steamcdn-a.akamaihd.net', 'steamuserimages-a.akamaihd.net']
const imageMirrors = ['shared.st.dl.eccdnx.com', 'shared.cdn.steamchina.queniuam.com']
const steamOrigins = [...steamDomains.map(domain => 'https://*.' + domain), ...steamHosts.map(host => 'https://' + host)]
const imageOrigins = [...steamOrigins, ...imageMirrors.map(host => 'https://' + host)]
const unsafe = /[\\<>\p{Cc}\p{Cf}]/u
const encoder = new TextEncoder()
const imageExtension = /\.(?:jpe?g|png|webp|avif|gif)$/i
const streamExtension = /\.(?:m3u8|m4s|mp4|ts|aac|m4a|cmfv|cmfa)$/i

export const profileMediaPolicy = [
  ["img-src", "'self'", 'data:', ...imageOrigins],
  ['media-src', "'self'", 'blob:', ...steamOrigins],
  ['connect-src', "'self'", ...steamOrigins],
].map(directive => directive.join(' ')).join('; ')

const sourceExtensions = {
  mp4_max: '.mp4', mp4_480: '.mp4', webm_max: '.webm', webm_480: '.webm',
  hls_h264: '.m3u8', dash_h264: '.mpd', dash_av1: '.mpd',
} as const satisfies Record<keyof SteamMovieSources, string>

export function publishedMediaURL(raw: unknown, format: 'image' | 'stream' | (typeof sourceExtensions)[keyof SteamMovieSources] = 'image') {
  if (typeof raw !== 'string' || raw.length > 2048 || encoder.encode(raw).length > 2048 ||
      /[?#\s]/u.test(raw) || unsafe.test(raw)) return undefined
  // Validate the original path before URL can normalize dot segments.
  const parts = /^https:\/\/([a-z0-9.-]+)(?::443)?(\/.+)$/i.exec(raw)
  if (!parts || !raw.startsWith('https://')) return undefined
  const [, authority, path] = parts
  const host = authority.toLowerCase()
  if (host.startsWith('.') || host.includes('..')) return undefined
  if (imageMirrors.includes(host)) {
    if (format !== 'image' || !/^\/(?:store_item_assets\/)?steam\/apps\//.test(path)) return undefined
  } else if (!steamHosts.includes(host) && !steamDomains.some(domain => host.endsWith('.' + domain))) return undefined

  try {
    const segments = path.slice(1).split('/').map(segment => decodeURIComponent(segment))
    if (segments.some(segment => !segment || segment === '.' || segment === '..' ||
      /[/%]/.test(segment) || unsafe.test(segment))) return undefined
    const filename = segments.at(-1)!
    const valid = format === 'image' ? imageExtension.test(filename) : format === 'stream' ? streamExtension.test(filename) :
      filename.toLowerCase().endsWith(format)
    if (!valid) return undefined
    const href = new URL(raw).href
    return href.length <= 2048 ? href : undefined
  } catch {
    return undefined
  }
}

export const artworkFamilies = {
  header: 'store', header_2x: 'store', capsule: 'store', capsule_v5: 'store',
  main_capsule: 'store', main_capsule_2x: 'store', small_capsule: 'store', small_capsule_2x: 'store',
  hero_capsule: 'store', hero_capsule_2x: 'store', background: 'store', background_raw: 'store',
  page_background: 'store', page_background_raw: 'store', community_icon: 'community',
  library_capsule: 'library', library_capsule_2x: 'library', library_hero: 'library', library_hero_2x: 'library',
  library_logo: 'library', library_logo_2x: 'library',
} as const

function artwork(value?: PublicGameMediaImage): PublicGameMediaImage | undefined {
  if (!value || !Object.hasOwn(artworkFamilies, value.asset_type) ||
      artworkFamilies[value.asset_type] !== value.asset_family) return undefined
  const url = publishedMediaURL(value.url)
  return url ? { asset_type: value.asset_type, asset_family: value.asset_family, url } : undefined
}

function mediaID(value: unknown, allowZero: boolean): value is string {
  return typeof value === 'string' && /^(?:0|[1-9]\d{0,19})$/.test(value) &&
    (allowZero || value !== '0') && (value.length < 20 || value <= '18446744073709551615')
}

export function selectPublishedMedia(value?: PublicGameMedia): PublicGameMedia | undefined {
  if (value?.version !== 2 || (value.locale !== 'zh' && value.locale !== 'en') ||
      typeof value.fallback !== 'boolean' || !Array.isArray(value.images) ||
      !Array.isArray(value.screenshots) || !Array.isArray(value.movies)) return undefined

  const cover = artwork(value.cover)
  const roles = new Set<PublicGameMediaImage['asset_type']>()
  const images = value.images.slice(0, 22).flatMap(image => {
    const item = artwork(image)
    if (!item || roles.has(item.asset_type)) return []
    roles.add(item.asset_type)
    return [item]
  })
  const screenshots = value.screenshots.slice(0, 8).flatMap(shot => {
    if (!mediaID(shot?.screenshot_id, true)) return []
    const thumbnail_url = publishedMediaURL(shot.thumbnail_url)
    const full_url = publishedMediaURL(shot.full_url)
    return thumbnail_url || full_url ? [{ screenshot_id: shot.screenshot_id, thumbnail_url, full_url }] : []
  })
  const movies = value.movies.slice(0, 4).flatMap(movie => {
    if (!mediaID(movie?.movie_id, false)) return []
    const sources: SteamMovieSources = {}
    for (const key of Object.keys(sourceExtensions) as (keyof SteamMovieSources)[]) {
      const url = publishedMediaURL(movie.sources?.[key], sourceExtensions[key])
      if (url) sources[key] = url
    }
    const name = typeof movie.name === 'string' && encoder.encode(movie.name).length <= 256 && !/[\p{Cc}\p{Cf}]/u.test(movie.name) ? movie.name : ''
    const thumbnail_url = publishedMediaURL(movie.thumbnail_url)
    if (!name && !thumbnail_url && Object.keys(sources).length === 0) return []
    return [{ movie_id: movie.movie_id, name, thumbnail_url,
      highlight: typeof movie.highlight === 'boolean' ? movie.highlight : undefined, sources }]
  })

  return cover || images.length || screenshots.length || movies.length
    ? { version: 2, locale: value.locale, fallback: value.fallback, cover, images, screenshots, movies }
    : undefined
}
