import { useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { useMotionTiming } from './motion'
import { useWallpaper, type BackgroundResource } from './start/wallpaper-state'
import styles from './start-background.module.scss'

export function StartBackground() {
  const { resource, preference, setError } = useWallpaper()
  const [visible, setVisible] = useState<BackgroundResource | null>(null)
  const off = preference?.mode === 'off' || (preference?.source === 'local' && !resource)
  const layers = off ? [] : visible && visible.key !== resource?.key ? [visible, ...(resource ? [resource] : [])] : resource ? [resource] : []
  return <div aria-hidden="true" data-start-background data-stage={off || !visible ? 'theme' : 'image'} className={styles.container}>
    <AnimatePresence onExitComplete={() => { if (off) setVisible(null) }}>
      {layers.map(item => <BackgroundLayer key={item.key} item={item} ready={() => setVisible(item)} failed={() => setError('cdn')} />)}
    </AnimatePresence>
  </div>
}
function BackgroundLayer({ item, ready, failed }: { item: BackgroundResource; ready: () => void; failed: () => void }) {
  const [preview, setPreview] = useState(false), [hd, setHD] = useState(false), [previewFailed, setPreviewFailed] = useState(false)
  const transition = useMotionTiming(0.48)
  return <motion.div className={styles.layer} exit={{ opacity: 0 }} transition={transition}>
    <motion.img src={item.preview} alt="" draggable={false} decoding="async" fetchPriority="low" data-start-preview className={styles.image}
      onLoad={e => { void e.currentTarget.decode().then(() => { setPreview(true); ready() }, () => setPreviewFailed(true)) }} onError={() => setPreviewFailed(true)} initial={{ opacity: 0 }} animate={{ opacity: preview ? 1 : 0 }} transition={transition} />
    {(preview || previewFailed) && item.image !== item.preview && <motion.img src={item.image} alt="" draggable={false} decoding="async" fetchPriority="low" data-start-hd className={styles.image}
      onLoad={e => { void e.currentTarget.decode().then(() => { setHD(true); ready() }, failed) }} onError={failed} initial={{ opacity: 0 }} animate={{ opacity: hd ? 1 : 0 }} transition={transition} />}
  </motion.div>
}
