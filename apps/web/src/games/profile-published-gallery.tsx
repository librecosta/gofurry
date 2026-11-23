import { useRef, useState, type KeyboardEvent } from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { AnimatePresence, motion, useIsPresent, useReducedMotion } from 'motion/react'
import { ArrowLeftIcon } from '@phosphor-icons/react/dist/ssr/ArrowLeft'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowRight'
import { ImageIcon } from '@phosphor-icons/react/dist/ssr/Image'
import { PlayIcon } from '@phosphor-icons/react/dist/ssr/Play'
import type { Locale } from '../preferences'
import { profileCopy } from './profile-copy'
import type { PublicGameMedia, PublicGameMediaImage, PublicGameMovie } from './profile-data'
import { PublishedMoviePlayer } from './profile-movie'
import styles from './profile.module.scss'

type Picture = { id: string; label: string; thumbnail?: string; full?: string }
type Entry = Picture & { role?: PublicGameMediaImage['asset_type']; movie?: PublicGameMovie }

const artworkLabels: Record<PublicGameMediaImage['asset_type'], [string, string]> = {
  header: ['Store header', '商店横幅'], header_2x: ['Store header · 2×', '商店横幅 · 2×'],
  capsule: ['Store capsule', '商店封面'], capsule_v5: ['Store capsule · V5', '商店封面 · V5'],
  main_capsule: ['Main capsule', '主商店封面'], main_capsule_2x: ['Main capsule · 2×', '主商店封面 · 2×'],
  small_capsule: ['Small capsule', '小尺寸商店封面'], small_capsule_2x: ['Small capsule · 2×', '小尺寸商店封面 · 2×'],
  hero_capsule: ['Hero capsule', '商店焦点封面'], hero_capsule_2x: ['Hero capsule · 2×', '商店焦点封面 · 2×'],
  background: ['Store background', '商店背景'], background_raw: ['Original store background', '商店原始背景'],
  page_background: ['Store page background', '商店页面背景'],
  page_background_raw: ['Original store page background', '商店页面原始背景'],
  community_icon: ['Community icon', '社区图标'],
  library_capsule: ['Library capsule', '库竖版封面'], library_capsule_2x: ['Library capsule · 2×', '库竖版封面 · 2×'],
  library_hero: ['Library hero', '库横幅'], library_hero_2x: ['Library hero · 2×', '库横幅 · 2×'],
  library_logo: ['Library logo', '库标志'], library_logo_2x: ['Library logo · 2×', '库标志 · 2×'],
}

function artworkShape(role?: Entry['role']) {
  if (role?.startsWith('library_capsule')) return 'portrait'
  if (role?.startsWith('library_logo')) return 'logo'
  if (role?.startsWith('library_hero')) return 'wide'
  if (role === 'community_icon') return 'square'
  return undefined
}

function Picture({ item, full = false, fallback }: { item: Picture; full?: boolean; fallback: string }) {
  const preferred = full ? item.full : item.thumbnail
  const alternate = full ? item.thumbnail : item.full
  const [failed, setFailed] = useState<string[]>([])
  const url = [preferred, alternate].find(source => source && !failed.includes(source))
  return url
    ? <img src={url} alt={full ? item.label : ''} loading={full ? 'eager' : 'lazy'} decoding="async"
        referrerPolicy="no-referrer" onError={() => setFailed(current => [...current, url])} />
    : full ? <p role="status" className={styles.mediaFailed}>{fallback}</p>
      : <ImageIcon size={24} aria-hidden="true" />
}

function Preview({ item, locale }: { item: Entry; locale: Locale }) {
  const present = useIsPresent()
  return <div className={styles.stage}>
    {item.movie
      ? present && <PublishedMoviePlayer movie={item.movie} label={item.label} locale={locale} />
      : <Picture item={item} fallback={profileCopy[locale].mediaFailed} full />}
  </div>
}

export default function PublishedGallery({ media, locale }: { media: PublicGameMedia; locale: Locale }) {
  const m = profileCopy[locale]
  const [index, setIndex] = useState(0)
  const [open, setOpen] = useState(false)
  const opener = useRef<HTMLElement>(null)
  const reduce = useReducedMotion()
  const fade = { initial: { opacity: 0 }, animate: { opacity: 1 }, exit: { opacity: 0 },
    transition: { duration: reduce ? 0 : .18 } }

  const groups: { label: string; entries: Entry[] }[] = [
    { label: m.screenshots, entries: media.screenshots.map((shot, i) => ({
      id: shot.screenshot_id, label: m.screenshot + ' ' + (i + 1),
      thumbnail: shot.thumbnail_url, full: shot.full_url,
    })) },
    { label: m.movies, entries: media.movies.map((movie, i) => ({
      id: movie.movie_id, label: movie.name || m.video + ' ' + (i + 1), thumbnail: movie.thumbnail_url, movie,
    })) },
    { label: m.artwork, entries: [...(media.cover ? [media.cover] : []), ...media.images].map(image => ({
      id: image.asset_type, label: artworkLabels[image.asset_type][locale === 'en' ? 0 : 1],
      role: image.asset_type, thumbnail: image.url,
    })) },
  ]
  const items = groups.flatMap(group => group.entries)
  const selected = items[index]
  if (!selected) return null

  function step(delta: number) { setIndex(current => (current + delta + items.length) % items.length) }
  function arrows(event: KeyboardEvent) {
    if (items.length < 2 || (event.target as HTMLElement).tagName === 'VIDEO') return
    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
      event.preventDefault()
      step(event.key === 'ArrowLeft' ? -1 : 1)
    }
  }

  return <Dialog.Root open={open} onOpenChange={setOpen}>
    <div className={styles.publishedGallery}>
      {groups.filter(group => group.entries.length > 0).map(group => <div key={group.label}>
        <h3>{group.label}</h3>
        <ul className={styles.publishedCards}>{group.entries.map(item => {
          const position = items.indexOf(item)
          return <li key={item.id + '/' + position}>
            <Dialog.Trigger className={styles.publishedCard} aria-current={position === index || undefined}
              aria-label={item.movie ? m.watch + ' ' + item.label : item.label}
              data-media-id={item.id}
              onClick={event => { opener.current = event.currentTarget; setIndex(position) }}>
              <span className={styles.publishedThumb} data-shape={artworkShape(item.role)}>
                <Picture item={item} fallback={m.mediaFailed} />
                {item.movie && <span className={styles.playMark}><PlayIcon size={19} aria-hidden="true" /></span>}
              </span>
              <span className={styles.mediaLabel}>{item.label}</span>
              {item.movie?.highlight && <span className={styles.mediaHighlight}>{m.highlight}</span>}
            </Dialog.Trigger>
          </li>
        })}</ul>
      </div>)}
    </div>
    <AnimatePresence>{open && <Dialog.Portal keepMounted>
      <Dialog.Backdrop className={styles.backdrop} render={<motion.div {...fade} />} />
      <Dialog.Popup className={styles.lightbox} finalFocus={() => opener.current ?? true} onKeyDown={arrows}
        render={<motion.div {...fade} />}>
        <header><Dialog.Title>{selected.label}</Dialog.Title><Dialog.Close className={styles.button}>{m.close}</Dialog.Close></header>
        <Preview key={index} item={selected} locale={locale} />
        {items.length > 1 && <div className={styles.galleryControls}>
          <button type="button" className={styles.button} onClick={() => step(-1)} aria-label={m.previous}><ArrowLeftIcon aria-hidden="true" /></button>
          <span aria-live="polite">{index + 1} / {items.length} · {selected.label}</span>
          <button type="button" className={styles.button} onClick={() => step(1)} aria-label={m.next}><ArrowRightIcon aria-hidden="true" /></button>
        </div>}
      </Dialog.Popup>
    </Dialog.Portal>}</AnimatePresence>
  </Dialog.Root>
}
