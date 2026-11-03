import { useEffect, useState, type SyntheticEvent } from 'react'
import { createServerFn } from '@tanstack/react-start'
import * as stylex from '@stylexjs/stylex'
import { tokens } from './styles/tokens.stylex'

type Asset = { url: string; width: number; height: number }
type Background = { id: string; sfw: boolean; preview: Asset; image: Asset }

// Metadata only. The browser downloads public images directly from the CDN.
// This deployment has one API origin; no cookies, user credentials or alternate
// region are forwarded, and availability never blocks the SSR search form.
const readBackgrounds = createServerFn({ method: 'GET' }).handler(async (): Promise<Background[]> => {
  try {
    const base = process.env.WEB_API_BASE_URL ?? 'http://127.0.0.1:17321'
    const response = await fetch(new URL('/api/v1/start/backgrounds', base), { signal: AbortSignal.timeout(9000), credentials: 'omit' })
    if (!response.ok || response.headers.get('x-gofurry-region') !== (process.env.WEB_REGION ?? 'CN') || response.headers.get('x-gofurry-content-mode') !== 'SFW') return []
    const body = await response.json() as { backgrounds: Background[] }
    const publicImage = (asset: Asset) => asset && new URL(asset.url).protocol === 'https:' && asset.width > 0 && asset.height > 0
    return body.backgrounds.filter((item) => item.sfw === true && publicImage(item.preview) && publicImage(item.image))
  } catch { return [] }
})

export function StartBackground() {
  const [background, setBackground] = useState<Background | null>(null)
  const [previewReady, setPreviewReady] = useState(false)
  const [previewFailed, setPreviewFailed] = useState(false)
  const [imageReady, setImageReady] = useState(false)
  useEffect(() => {
    let active = true
    void readBackgrounds().then((items) => { if (active && items[0]) setBackground(items[0]) }).catch(() => {})
    return () => { active = false }
  }, [])
  function decoded(event: SyntheticEvent<HTMLImageElement>, ready: () => void, failed: () => void) {
    const image = event.currentTarget
    void image.decode().then(ready, failed)
  }
  return <div aria-hidden="true" data-start-background data-stage={imageReady ? 'image' : previewReady ? 'preview' : 'theme'} {...stylex.props(styles.container)}>
    {background && <>
      <img src={background.preview.url} width={background.preview.width} height={background.preview.height}
        alt="" draggable={false} decoding="async" fetchPriority="low" data-start-preview
        onLoad={(event) => decoded(event, () => setPreviewReady(true), () => setPreviewFailed(true))}
        onError={() => setPreviewFailed(true)} {...stylex.props(styles.image, previewReady && styles.visible)} />
      {(previewReady || previewFailed) && <img src={background.image.url} width={background.image.width} height={background.image.height}
        alt="" draggable={false} decoding="async" fetchPriority="low" data-start-hd
        onLoad={(event) => decoded(event, () => setImageReady(true), () => {})}
        {...stylex.props(styles.image, styles.fade, imageReady && styles.visible)} />}
    </>}
  </div>
}

const styles = stylex.create({
  container: { position: 'fixed', inset: 0, zIndex: -1, pointerEvents: 'none', overflow: 'hidden' },
  image: { position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover', opacity: 0 },
  visible: { opacity: 1 },
  fade: { transitionProperty: 'opacity', transitionDuration: tokens.motionBase, transitionTimingFunction: 'ease-out' },
})
