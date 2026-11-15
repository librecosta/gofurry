import { useRef, useState, type KeyboardEvent } from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { motion, useReducedMotion } from 'motion/react'
import { ArrowLeftIcon } from '@phosphor-icons/react/dist/ssr/ArrowLeft'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowRight'
import { ArrowsOutIcon } from '@phosphor-icons/react/dist/ssr/ArrowsOut'
import { PlayIcon } from '@phosphor-icons/react/dist/ssr/Play'
import { ImageIcon } from '@phosphor-icons/react/dist/ssr/Image'
import type { Locale } from '../preferences'
import { profileCopy } from './profile-copy'
import { profileMediaURL, type GameSource } from './profile-data'
import styles from './profile.module.scss'

type Item = { url: string; type: 'screenshot' | 'video'; label: string }

function Media({ item, fallback, full }: { item: Item; fallback: string; full?: boolean }) {
  const [failed, setFailed] = useState(false)
  if (failed) return <p role="status" className={styles.mediaFailed}>{fallback}</p>
  return item.type === 'video'
    ? <video src={item.url} controls playsInline preload="none" controlsList="nodownload" aria-label={item.label} onError={() => setFailed(true)} />
    : <img src={item.url} alt={item.label} width={1280} height={720} loading={full ? 'eager' : 'lazy'}
      referrerPolicy="no-referrer" onError={() => setFailed(true)} />
}

export default function Gallery({ sources, locale }: { sources: GameSource[]; locale: Locale }) {
  const m = profileCopy[locale]
  const [source, setSource] = useState(0)
  const [index, setIndex] = useState(0)
  const [open, setOpen] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  const reduce = useReducedMotion()
  const media = sources[source]?.media
  const images = media?.screenshots?.length ? media.screenshots : [media?.header ?? media?.capsule]
  const items: Item[] = [...images.flatMap((url, i) => {
    const safe = profileMediaURL(url)
    return safe ? [{ url: safe, type: 'screenshot' as const, label: m.screenshot + ' ' + (i + 1) }] : []
  }), ...(media?.movies ?? []).flatMap((url, i) => {
    const safe = profileMediaURL(url, true)
    return safe ? [{ url: safe, type: 'video' as const, label: m.video + ' ' + (i + 1) }] : []
  })]
  const active = items[index]
  function step(delta: number) { setIndex(current => (current + delta + items.length) % items.length) }
  function arrows(event: KeyboardEvent) {
    if (items.length < 2 || event.target instanceof HTMLVideoElement) return
    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
      event.preventDefault()
      step(event.key === 'ArrowLeft' ? -1 : 1)
    }
  }
  return <div className={styles.gallery}>
    {sources.length > 1 && <label className={styles.control}>{m.app}
      <select value={source} onChange={event => { setSource(Number(event.target.value)); setIndex(0) }}>
        {sources.map((item, i) => <option key={item.external_key} value={i}>Steam · {item.external_key}</option>)}
      </select>
    </label>}
    {active && <Dialog.Root open={open} onOpenChange={setOpen}>
      <div className={styles.stage} onKeyDown={arrows}>
        {!open && <Media key={active.url} item={active} fallback={m.mediaFailed} />}
        <Dialog.Trigger ref={trigger} className={styles.expand} aria-label={m.expand}>
          <ArrowsOutIcon aria-hidden="true" size={20} />
        </Dialog.Trigger>
      </div>
      <Dialog.Portal>
        <Dialog.Backdrop className={styles.backdrop} />
        <Dialog.Popup className={styles.lightbox} finalFocus={trigger} onKeyDown={arrows}>
          <header><Dialog.Title>{active.label}</Dialog.Title><Dialog.Close className={styles.button}>{m.close}</Dialog.Close></header>
          <motion.div className={styles.stage} initial={reduce ? false : { opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: .18 }}>
            {open && <Media key={active.url} item={active} fallback={m.mediaFailed} full />}
          </motion.div>
          {items.length > 1 && <div className={styles.galleryControls}>
            <button type="button" className={styles.button} onClick={() => step(-1)} aria-label={m.previous}><ArrowLeftIcon aria-hidden="true" /></button>
            <span aria-live="polite">{index + 1} / {items.length}</span>
            <button type="button" className={styles.button} onClick={() => step(1)} aria-label={m.next}><ArrowRightIcon aria-hidden="true" /></button>
          </div>}
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>}
    <ul className={styles.thumbs}>{items.map((item, i) => <li key={item.url}>
      <button type="button" aria-pressed={i === index} aria-label={item.label} onClick={() => setIndex(i)}>
        {item.type === 'video' ? <PlayIcon size={19} aria-hidden="true" /> : <><ImageIcon size={19} aria-hidden="true" /><img src={item.url} alt="" width={112} height={63}
          loading="lazy" referrerPolicy="no-referrer" onError={event => { event.currentTarget.hidden = true }} /></>}
        <span>{i + 1}</span>
      </button>
    </li>)}</ul>
  </div>
}
