import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { AnimatePresence, motion, useIsPresent } from 'motion/react'
import { useMotionTiming } from './motion'
import { useWallpaper, type BackgroundResource } from './start/wallpaper-state'
import styles from './start-background.module.scss'

export function StartBackground() {
  const { resource, preference, setError } = useWallpaper()
  const [visible, setVisible] = useState<BackgroundResource | null>(null)
  const [stage, setStage] = useState('theme')
  const latest = useRef(resource); latest.current = resource
  const off = preference?.mode === 'off' || !resource
  const layers = off ? [] : visible && visible.key !== resource?.key ? [visible, ...(resource ? [resource] : [])] : resource ? [resource] : []
  return <div aria-hidden="true" data-start-background data-stage={off || !visible ? 'theme' : stage} className={styles.container}>
    <AnimatePresence onExitComplete={() => { if (off) setVisible(null) }}>
      {layers.map(item => <BackgroundLayer key={item.key} item={item} ready={hd => { if (latest.current?.key === item.key) { setVisible(item); setStage(hd || item.preview === item.image ? 'image' : 'preview') } }} failed={() => { if (latest.current?.key === item.key) setError(item.preview.startsWith('blob:') ? 'format' : 'cdn') }} />)}
    </AnimatePresence>
  </div>
}
function BackgroundLayer({ item, ready, failed }: { item: BackgroundResource; ready: (hd: boolean) => void; failed: () => void }) {
  const [preview, setPreview] = useState(false), [hd, setHD] = useState(false), [previewFailed, setPreviewFailed] = useState(false)
  const transition = useMotionTiming(0.48)
  const present = useIsPresent(), active = useRef(true), callbacks = useRef({ ready, failed, present })
  callbacks.current = { ready, failed, present }
  useEffect(() => { active.current = true; return () => { active.current = false; item.release?.() } }, [item.key])
  useLayoutEffect(() => { item.claim?.() }, [item.key])
  const decoded = (image: HTMLImageElement, hd: boolean) => { void image.decode().then(() => {
    if (!active.current || !callbacks.current.present) return
    if (hd) setHD(true); else setPreview(true)
    callbacks.current.ready(hd)
  }, () => { if (active.current && callbacks.current.present) { if (hd) callbacks.current.failed(); else setPreviewFailed(true) } }) }
  return <motion.div className={styles.layer} exit={{ opacity: 0, transition: preview || hd ? transition : { duration: 0 } }} transition={transition}>
    <motion.img src={item.preview} alt="" draggable={false} decoding="async" fetchPriority="low" data-start-preview className={styles.image}
      onLoad={e => decoded(e.currentTarget, false)} onError={() => { setPreviewFailed(true); if (item.image === item.preview) callbacks.current.failed() }} initial={{ opacity: 0 }} animate={{ opacity: preview ? 1 : 0 }} transition={transition} />
    {(hd || present && (preview || previewFailed)) && item.image !== item.preview && <motion.img src={item.image} alt="" draggable={false} decoding="async" fetchPriority="low" data-start-hd className={styles.image}
      onLoad={e => decoded(e.currentTarget, true)} onError={failed} initial={{ opacity: 0 }} animate={{ opacity: hd ? 1 : 0 }} transition={transition} />}
  </motion.div>
}
