import { useEffect, useRef, useState } from 'react'
import type { Locale } from '../preferences'
import { profileCopy } from './profile-copy'
import type { PublicGameMovie, SteamMovieSources } from './profile-data'
import { publishedMediaURL } from './published-media'
import styles from './profile.module.scss'

const files = [
  ['mp4_max', 'video/mp4'], ['mp4_480', 'video/mp4'],
  ['webm_max', 'video/webm'], ['webm_480', 'video/webm'],
] as const satisfies readonly (readonly [keyof SteamMovieSources, string])[]
const hlsMime = 'application/vnd.apple.mpegurl'
const loadPolicy = {
  default: { maxTimeToFirstByteMs: 6500, maxLoadTimeMs: 12000, timeoutRetry: null, errorRetry: null },
}

type Failure = 'videoNetwork' | 'videoDecode' | 'videoUnsupported' | 'videoManifest'
type State = 'loading' | 'ready' | Failure | 'videoDash' | 'videoNoSource'

export function PublishedMoviePlayer({ movie, label, locale }: { movie: PublicGameMovie; label: string; locale: Locale }) {
  const video = useRef<HTMLVideoElement>(null)
  const [status, setStatus] = useState<State>('loading')
  const m = profileCopy[locale]

  useEffect(() => {
    const element = video.current!
    const options = files.flatMap(([key, mime]) => {
      const url = movie.sources[key]
      return url && element.canPlayType(mime) ? [{ url, mode: 'file' as const }] : []
    })
    const hls = movie.sources.hls_h264
    const playback: { url: string; mode: 'file' | 'native-hls' | 'mse' }[] = [...options]
    if (hls) {
      if (element.canPlayType(hlsMime)) playback.push({ url: hls, mode: 'native-hls' })
      playback.push({ url: hls, mode: 'mse' })
    }
    const onlyDash = !files.some(([key]) => movie.sources[key]) && !hls && (movie.sources.dash_av1 || movie.sources.dash_h264)
    const hasSources = Object.keys(movie.sources).length > 0

    let disposed = false
    let player: import('hls.js').default | undefined
    let generation = 0
    let timeout: ReturnType<typeof setTimeout> | undefined
    function clearTimer() { clearTimeout(timeout) }
    function release() {
      clearTimer()
      element.onerror = element.oncanplay = element.onwaiting = element.onplaying = element.onpause = null
      element.pause()
      player?.destroy()
      player = undefined
      if (element.hasAttribute('src')) {
        element.removeAttribute('src')
        element.load()
      }
    }

    async function attempt(index: number, previous?: Failure) {
      const current = ++generation
      release()
      const option = playback[index]
      if (!option) {
        setStatus(previous ?? (onlyDash ? 'videoDash' : hasSources ? 'videoUnsupported' : 'videoNoSource'))
        return
      }
      const active = () => !disposed && current === generation
      const advance = (failure: Failure) => queueMicrotask(() => {
        if (active()) void attempt(index + 1, failure === 'videoUnsupported' ? previous ?? failure : failure)
      })
      function watch(failure: Failure) {
        clearTimer()
        timeout = setTimeout(() => advance(failure), 14000)
      }

      setStatus('loading')
      watch(option.mode === 'file' ? 'videoNetwork' : 'videoManifest')
      element.oncanplay = () => { if (active()) { clearTimer(); setStatus('ready') } }
      element.onplaying = element.onpause = clearTimer
      element.onwaiting = () => { if (!element.paused && active()) watch('videoNetwork') }
      element.onerror = () => advance(element.error?.code === 3 ? 'videoDecode' :
        option.mode === 'native-hls' ? 'videoManifest' : 'videoNetwork')

      if (option.mode !== 'mse') {
        element.src = option.url
        return
      }
      try {
        const { default: Hls, FetchLoader } = await import('hls.js/light')
        if (!active()) return
        if (!Hls.isSupported()) { advance('videoUnsupported'); return }
        const instance = new Hls({
          enableWorker: false,
          loader: FetchLoader,
          fetchSetup: async (context, init: RequestInit) => {
            if (!publishedMediaURL(context.url, 'stream')) throw new Error('Invalid Steam stream URL')
            // Keep loader-owned abort signals and Range headers for fMP4.
            return new Request(context.url, { ...init, credentials: 'omit', redirect: 'error', referrerPolicy: 'no-referrer' })
          },
          manifestLoadPolicy: loadPolicy,
          playlistLoadPolicy: loadPolicy,
          fragLoadPolicy: loadPolicy,
          keyLoadPolicy: loadPolicy,
          maxBufferLength: 15,
          maxMaxBufferLength: 30,
          backBufferLength: 15,
        })
        player = instance
        instance.on(Hls.Events.MEDIA_ATTACHED, () => { if (active()) instance.loadSource(option.url) })
        instance.on(Hls.Events.ERROR, (_, error) => {
          if (!error.fatal) return
          advance(error.type === Hls.ErrorTypes.MEDIA_ERROR ? 'videoDecode' :
            /manifest|level|audioTrack/i.test(error.details) ? 'videoManifest' : 'videoNetwork')
        })
        instance.attachMedia(element)
      } catch {
        advance('videoUnsupported')
      }
    }

    void attempt(0)
    return () => { disposed = true; release() }
  }, [movie])

  const failed = status !== 'ready' && status !== 'loading'
  return <div className={styles.movie}>
    <video ref={video} aria-label={label} controls playsInline preload="metadata" crossOrigin="anonymous"
      poster={movie.thumbnail_url} hidden={failed} />
    {failed && movie.thumbnail_url && <img src={movie.thumbnail_url} alt={label}
      decoding="async" referrerPolicy="no-referrer" onError={event => { event.currentTarget.hidden = true }} />}
    {status !== 'ready' && <p role="status" className={styles.movieStatus}>{m[status]}</p>}
  </div>
}
