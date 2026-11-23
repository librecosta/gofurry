import { createServer } from 'node:http'

const image = 'https://shared.steamstatic.com/steam/apps/1/'
const video = 'https://video.akamai.steamstatic.com/store_trailers/1/'
const legacy = {
  platform_code: 'steam', external_key: '1', active: true, preferred: true,
  source_url: 'https://store.steampowered.com/app/1/', quotes: [],
  media: { screenshots: [image + 'legacy.jpg'], movies: [video + 'legacy.mp4'] },
}
const media = {
  version: 2, locale: 'en', fallback: true,
  cover: { asset_type: 'library_capsule', asset_family: 'library', url: image + 'cover.jpg' },
  images: [
    { asset_type: 'library_hero', asset_family: 'library', url: image + 'hero.png' },
    { asset_type: 'library_hero_2x', asset_family: 'library', url: image + 'hero.png' },
    { asset_type: 'library_logo', asset_family: 'library', url: image + 'logo.png' },
    { asset_type: 'community_icon', asset_family: 'community', url: 'https://shared.st.dl.eccdnx.com/steam/apps/1/icon.png' },
  ],
  screenshots: [
    { screenshot_id: '9007199254740995', thumbnail_url: image + 'z-thumb.jpg', full_url: image + 'z-full.jpg' },
    { screenshot_id: '2', thumbnail_url: image + 'bad-thumb.jpg', full_url: image + 'a-full.jpg' },
    { screenshot_id: '0', thumbnail_url: image + 'bad-full.jpg' },
    { screenshot_id: '3', thumbnail_url: image + 'fallback-thumb.jpg', full_url: image + 'bad-full.jpg' },
  ],
  movies: [
    { movie_id: '9007199254740993', name: 'Direct trailer', thumbnail_url: image + 'poster.jpg', highlight: true,
      sources: { mp4_max: video + 'direct.mp4', hls_h264: video + 'fallback.m3u8' } },
    { movie_id: '9007199254740997', name: 'WebM trailer', thumbnail_url: image + 'poster.jpg', sources: { webm_max: video + 'direct.webm' } },
    { movie_id: '3', name: 'DASH trailer', thumbnail_url: image + 'poster.jpg', sources: { dash_h264: video + 'only.mpd' } },
    { movie_id: '4', name: '<em>Poster only</em>', thumbnail_url: image + 'poster.jpg', sources: { hls_h264: video + 'wrong.mp4' } },
  ],
}
const variants = {
  1: media,
  2: undefined,
  3: { version: 2, locale: 'en', fallback: true, images: [], screenshots: [], movies: [] },
  4: { ...media, cover: undefined, images: [], screenshots: [], movies: [
    { movie_id: '8', name: 'HLS trailer', thumbnail_url: image + 'poster.jpg', sources: { hls_h264: video + 'only.m3u8' } },
  ] },
  5: { ...media, cover: undefined, screenshots: [], movies: [] },
  6: { ...media, cover: undefined, images: [], movies: [] },
}
createServer((request, response) => {
  const url = new URL(request.url, 'http://localhost')
  const match = /^\/api\/v1\/games\/p00000000000000([1-6])$/.exec(url.pathname)
  response.setHeader('X-GoFurry-Region', 'CN')
  response.setHeader('X-GoFurry-Content-Mode', 'SFW')
  response.setHeader('Content-Type', 'application/json')
  if (url.pathname === '/readyz') { response.end('{}'); return }
  if (url.pathname === '/api/v1/games/directory') {
    const games = Array.from({ length: 36 }, (_, i) => ({
      public_id: i === 35 ? 'p000000000000001' : 'p000000000000' + String(i + 10).padStart(3, '0'),
      slug: 'gallery', localizations: [{ locale: 'en', name: i === 35 ? 'Published game' : 'Other game ' + i }],
      platforms: [], genres: [], systems: [], release_state: 'unknown', first_published_at: null,
    }))
    response.end(JSON.stringify({ items: games }))
    return
  }
  if (!match || url.searchParams.get('profile') !== '1') {
    response.writeHead(404).end('{}')
    return
  }
  const n = match[1]
  const published = variants[n]
  response.end(JSON.stringify({
    public_id: 'p00000000000000' + n, slug: 'gallery', cover_url: image + 'legacy-cover.jpg',
    localizations: [{ locale: 'zh', name: '正式游戏' }, { locale: 'en', name: 'Published game' }],
    sources: n <= '3' ? [legacy] : [],
    published_media: published && { ...published, fallback: url.searchParams.get('locale') !== published.locale },
    related: [], news: { items: [] }, news_status: 'available',
  }))
}).listen(17561, '127.0.0.1')
