import { useEffect, useState, type SyntheticEvent } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { useMotionTiming } from './motion'
import { createServerFn } from '@tanstack/react-start'
import styles from './start-background.module.scss'

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
  const transition = useMotionTiming(0.48)
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
  return <div aria-hidden="true" data-start-background data-stage={imageReady ? 'image' : previewReady ? 'preview' : 'theme'} className={styles.container}>
    <AnimatePresence>{background && <motion.img key={background.preview.url} src={background.preview.url} width={background.preview.width} height={background.preview.height}
        alt="" draggable={false} decoding="async" fetchPriority="low" data-start-preview
        onLoad={(event) => decoded(event, () => setPreviewReady(true), () => setPreviewFailed(true))}
        onError={() => setPreviewFailed(true)} className={styles.image} initial={{ opacity: 0 }} animate={{ opacity: previewReady ? 1 : 0 }} exit={{ opacity: 0 }} transition={transition} />}
      {background && (previewReady || previewFailed) && <motion.img key={background.image.url} src={background.image.url} width={background.image.width} height={background.image.height}
        alt="" draggable={false} decoding="async" fetchPriority="low" data-start-hd
        onLoad={(event) => decoded(event, () => setImageReady(true), () => {})}
        className={styles.image} initial={{ opacity: 0 }} animate={{ opacity: imageReady ? 1 : 0 }} exit={{ opacity: 0 }} transition={transition} />}
    </AnimatePresence>
  </div>
}
