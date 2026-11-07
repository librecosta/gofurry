import { useEffect, useId, useRef, useState, type RefObject } from 'react'
import { AnimatePresence, motion, Reorder, useDragControls, useIsPresent, type PanInfo } from 'motion/react'
import { Dialog } from '@base-ui/react/dialog'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr/Plus'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr/Globe'
import { CheckIcon } from '@phosphor-icons/react/dist/ssr/Check'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import { DotsSixVerticalIcon } from '@phosphor-icons/react/dist/ssr/DotsSixVertical'
import { ArrowUpIcon } from '@phosphor-icons/react/dist/ssr/ArrowUp'
import { ArrowDownIcon } from '@phosphor-icons/react/dist/ssr/ArrowDown'
import { PencilSimpleIcon } from '@phosphor-icons/react/dist/ssr/PencilSimple'
import { TrashIcon } from '@phosphor-icons/react/dist/ssr/Trash'
import { PushPinIcon } from '@phosphor-icons/react/dist/ssr/PushPin'
import { PushPinSlashIcon } from '@phosphor-icons/react/dist/ssr/PushPinSlash'
import { useMotionTiming } from '../motion'
import { cx } from '../styles/classes'
import { useQuickAccess, siteKey, siteURL, visibleSites, type PinnedSite, type QuickSite } from './quick-access-data'
import { useQuickMessages } from './quick-access-messages'
import { useStartSurface } from './start-material'
import settings from './settings.module.scss'
import controls from './controls.module.scss'
import styles from './quick-access.module.scss'

function SiteIcon({ url, present }: { url: string; present: boolean }) {
  const glass = useStartSurface('quick-icon', present)
  const parsed = siteURL(url)!
  const hostname = parsed.hostname.replace(/\.$/, '')
  // Only public-looking DNS hostnames go to the third party. IP literals,
  // single-label hosts and local/reserved namespaces use the site's icon only.
  const publicHost = hostname.length <= 253 && hostname.split('.').length > 1
    && hostname.split('.').every(label => /^[a-z\d](?:[a-z\d-]{0,61}[a-z\d])?$/i.test(label))
    && /\.[a-z][a-z\d-]+$/i.test(hostname)
    && !/(?:^|\.)(?:localhost|localdomain|local|internal|intranet|lan|home|corp|private|test|invalid|example|onion|alt|arpa)$/.test(hostname)
    && !/(?:^|\.)example\.(?:com|net|org)$/.test(hostname)
  const sources = [...(publicHost ? [`https://a.favicon.im/${encodeURIComponent(hostname)}?larger=true&throw-error-on-404=true`] : []), new URL('/favicon.ico', parsed).href]
  const [attempt, setAttempt] = useState({ index: 0, loaded: false })
  const source = sources[attempt.index]
  const next = () => setAttempt(current => current.index === attempt.index ? { index: current.index + 1, loaded: false } : current)
  return <span {...glass.props} className={cx(styles.icon, glass.className)} data-quick-brand={attempt.loaded || undefined} aria-hidden="true">
    {glass.binding}
    {!attempt.loaded && <GlobeIcon size={32} weight="light" />}
    {source && <img key={source} src={source} alt="" width={34} height={34} loading="lazy" decoding="async" referrerPolicy="no-referrer"
      style={{ visibility: attempt.loaded ? 'visible' : 'hidden' }} onLoad={event => {
        void event.currentTarget.decode().then(() => setAttempt(current => current.index === attempt.index ? { ...current, loaded: true } : current), next)
      }} onError={next} />}
  </span>
}

export function QuickAccess() {
  const { data, ready } = useQuickAccess()
  const m = useQuickMessages()
  const [manager, setManager] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  const glass = useStartSurface('quick-icon', ready)
  const transition = useMotionTiming()
  const animation = { initial: { opacity: 0, y: 4 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0, y: 4 }, transition }
  return <>
    <motion.div data-quick-access className={styles.row} initial={false} animate={{ opacity: ready ? 1 : 0 }} transition={transition}>
      <AnimatePresence initial={false}>{ready && visibleSites(data).map(site => <SiteEntry key={site.key} site={site} />)}</AnimatePresence>
      <button ref={trigger} data-quick-manage type="button" aria-label={m.title} disabled={!ready} className={styles.tile} onClick={() => setManager(true)}>
        <span {...glass.props} className={cx(styles.icon, glass.className)}><PlusIcon size={30} weight="light" aria-hidden="true" /></span>
        {glass.binding}
      </button>
    </motion.div>
    <Dialog.Root open={manager} onOpenChange={setManager}>
      <AnimatePresence>{manager && <Dialog.Portal keepMounted>
        <Dialog.Backdrop className={settings.backdrop} render={<motion.div {...animation} />} />
        <Dialog.Popup data-quick-manager className={cx(settings.popup, styles.manager)} finalFocus={trigger} render={<motion.div {...animation} />}>
          <div className={settings.heading}><Dialog.Title className={settings.title}>{m.title}</Dialog.Title>
            <Dialog.Close className={cx(controls.glass, settings.close)} aria-label={m.close}><XIcon size={18} aria-hidden="true" /></Dialog.Close></div>
          <Dialog.Description className={styles.srOnly}>{m.pinned} · {m.recent}</Dialog.Description>
          <ManagerContent />
        </Dialog.Popup>
      </Dialog.Portal>}</AnimatePresence>
    </Dialog.Root>
  </>
}

function SiteEntry({ site }: { site: QuickSite }) {
  const transition = useMotionTiming()
  const present = useIsPresent()
  return <motion.div data-quick-site data-site-key={site.key} data-pinned={!!site.pin} className={styles.entry} inert={!present} layout="position" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition}>
    <a href={site.url} target="_blank" rel="noopener noreferrer" data-quick-name={site.name} className={styles.tile}>
      <SiteIcon key={site.url} url={site.url} present={present} /><span className={styles.name}>{site.name}</span>
    </a>
  </motion.div>
}

function ManagerContent() {
  const { data, canSave, save, pin, unpin, remove, removeRecent, reorder } = useQuickAccess()
  const m = useQuickMessages()
  const descriptionId = useId()
  const [editing, setEditing] = useState<string>()
  const [name, setName] = useState(''), [url, setURL] = useState('')
  const [error, setError] = useState(false)
  const nameField = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLUListElement>(null)
  const canConfirm = canSave(name, url, editing)
  const reset = () => { setEditing(undefined); setName(''); setURL(''); setError(false) }
  const edit = (site: PinnedSite) => { setEditing(site.id); setName(site.name); setURL(site.url); setError(false); nameField.current?.focus() }
  const move = (id: string, delta: number) => {
    const next = [...data.pinned], index = next.findIndex(p => p.id === id), target = index + delta
    if (target < 0 || target >= next.length) return
    ;[next[index], next[target]] = [next[target], next[index]]; reorder(next)
  }
  const dragEvents = useListEdgeScroll(list, move)
  return <>
    <form className={styles.form} aria-label={editing ? m.edit : m.add} onSubmit={event => {
      event.preventDefault()
      if (!canConfirm) return
      if (!save(name, url, editing)) { setError(true); return }
      reset(); nameField.current?.focus()
    }}>
      <label>{m.name}<input ref={nameField} name="site-name" value={name} onChange={e => setName(e.target.value)} required maxLength={80} autoComplete="off" /></label>
      <label>{m.url}<input name="site-url" value={url} onChange={e => setURL(e.target.value)} required maxLength={2048} inputMode="url" autoComplete="off" placeholder="https://example.com" /></label>
      <button type="submit" className={styles.formAction} aria-label={editing ? m.save : m.add} disabled={!canConfirm}>{editing ? <CheckIcon size={18} aria-hidden="true" /> : <PlusIcon size={18} aria-hidden="true" />}</button>
      <button type="button" className={styles.formAction} aria-label={m.cancel} disabled={!editing} onClick={reset}><XIcon size={18} aria-hidden="true" /></button>
      {error && <p role="alert" className={styles.hint}>{m.invalid}</p>}
    </form>
    <p id={`${descriptionId}-unpin`} className={styles.srOnly}>{m.unpinHint}</p>
    <p id={`${descriptionId}-delete`} className={styles.srOnly}>{m.deleteHint}</p>
    <h3 className={styles.sectionTitle}>{m.pinned}</h3>
    {data.pinned.length === 0 && <p className={styles.hint}>{m.empty}</p>}
    <Reorder.Group ref={list} axis="y" values={data.pinned} onReorder={reorder} layoutScroll className={styles.list} aria-label={m.pinned}>
      {data.pinned.map((site, index) => <PinnedRow key={site.id} site={site} first={index === 0} last={index === data.pinned.length - 1}
        edit={() => edit(site)} move={delta => move(site.id, delta)} descriptionId={descriptionId} list={list} dragEvents={dragEvents}
        unpin={() => { unpin(site.id); if (editing === site.id) reset() }} remove={() => { remove(siteKey(site.url)); if (editing === site.id) reset() }} />)}
    </Reorder.Group>
    <details className={styles.recent}><summary>{m.recent} ({data.recent.length})</summary>
      {data.recent.length === 0 && <p className={styles.hint}>{m.noRecent}</p>}
      <ul className={styles.list}>{data.recent.map(site => {
        const key = siteKey(site.url), pinned = data.pinned.some(p => siteKey(p.url) === key)
        return <li key={key} className={styles.recentRow}>
          <span className={styles.rowName}>{site.name}<small>{new URL(site.url).host}</small></span>
          <button type="button" className={styles.smallButton} aria-label={`${m.pin}: ${site.name}`} disabled={pinned || !canSave(site.name, site.url)} onClick={() => pin({ ...site, key })}><PushPinIcon size={16} aria-hidden="true" /></button>
          <button type="button" className={styles.smallButton} aria-label={`${m.removeRecent}: ${site.name}`} onClick={() => removeRecent(key)}><XIcon size={16} aria-hidden="true" /></button>
        </li>
      })}</ul>
    </details>
  </>
}

function PinnedRow({ site, first, last, edit, move, unpin, remove, descriptionId, list, dragEvents }: { site: PinnedSite; first: boolean; last: boolean; edit: () => void; move: (delta: number) => void; unpin: () => void; remove: () => void; descriptionId: string; list: RefObject<HTMLUListElement | null>; dragEvents: ReturnType<typeof useListEdgeScroll> }) {
  const drag = useDragControls(), m = useQuickMessages(), transition = useMotionTiming()
  return <Reorder.Item value={site} data-quick-pin={site.id} dragListener={false} dragControls={drag} dragConstraints={list}
    onMeasureDragConstraints={bounds => {
      const element = list.current!
      // Constrain to the full list content, not just its visible scroll window.
      return { ...bounds, top: bounds.top - element.scrollTop, bottom: bounds.bottom + element.scrollHeight - element.clientHeight - element.scrollTop }
    }} dragElastic={0} dragMomentum={false} {...dragEvents} onDragStart={(event, info) => dragEvents.onDragStart(site.id, event, info)} layout="position" transition={transition} className={styles.pinRow}>
    <button type="button" className={cx(styles.smallButton, styles.drag)} aria-label={`${m.drag}: ${site.name}`} onPointerDown={event => drag.start(event)}
      onKeyDown={event => { if (event.key === 'ArrowUp' || event.key === 'ArrowDown') { event.preventDefault(); move(event.key === 'ArrowUp' ? -1 : 1) } }}><DotsSixVerticalIcon size={18} aria-hidden="true" /></button>
    <span className={styles.rowName}>{site.name}<small>{new URL(site.url).host}</small></span>
    <button type="button" className={styles.smallButton} disabled={first} aria-label={`${m.up}: ${site.name}`} onClick={() => move(-1)}><ArrowUpIcon size={15} aria-hidden="true" /></button>
    <button type="button" className={styles.smallButton} disabled={last} aria-label={`${m.down}: ${site.name}`} onClick={() => move(1)}><ArrowDownIcon size={15} aria-hidden="true" /></button>
    <button type="button" className={styles.smallButton} aria-label={`${m.edit}: ${site.name}`} onClick={edit}><PencilSimpleIcon size={15} aria-hidden="true" /></button>
    <button type="button" className={styles.smallButton} aria-label={`${m.unpin}: ${site.name}`} aria-describedby={`${descriptionId}-unpin`} onClick={unpin}><PushPinSlashIcon size={15} aria-hidden="true" /></button>
    <button type="button" className={styles.smallButton} aria-label={`${m.remove}: ${site.name}`} aria-describedby={`${descriptionId}-delete`} onClick={remove}><TrashIcon size={15} aria-hidden="true" /></button>
  </Reorder.Item>
}

function useListEdgeScroll(list: RefObject<HTMLUListElement | null>, move: (id: string, delta: number) => void) {
  const pointer = useRef<PanInfo['point'] | null>(null), frame = useRef<number | undefined>(undefined)
  const latestMove = useRef(move), velocity = useRef(0)
  useEffect(() => { latestMove.current = move }, [move])
  const stop = () => { pointer.current = null; if (frame.current !== undefined) cancelAnimationFrame(frame.current); frame.current = undefined }
  useEffect(() => {
    window.addEventListener('blur', stop)
    return () => { stop(); window.removeEventListener('blur', stop) }
  }, [])
  return {
    onDragStart: (id: string, _: unknown, info: PanInfo) => {
      stop(); pointer.current = info.point
      const row = Array.from(list.current!.children).find(row => (row as HTMLElement).dataset.quickPin === id) as HTMLElement
      const grabOffset = row.getBoundingClientRect().top + row.offsetHeight / 2 - info.point.y
      let previous = performance.now()
      const scroll = (now: number) => {
        const element = list.current, point = pointer.current
        if (!element || !point || document.hidden) { stop(); return }
        const bounds = element.getBoundingClientRect(), elapsed = Math.min(now - previous, 32); previous = now
        if (point.x >= bounds.left && point.x <= bounds.right && element.scrollHeight > element.clientHeight) {
          const direction = point.y < bounds.top + 32 ? -Math.min(1, (bounds.top + 32 - point.y) / 32) : point.y > bounds.bottom - 32 ? Math.min(1, (point.y - bounds.bottom + 32) / 32) : 0
          const before = element.scrollTop
          element.scrollTop += direction * 360 * elapsed / 1000
          // Reorder stops comparing neighbours when pointer velocity is zero.
          // Continue one slot at a time as scrolling brings rows under the held item.
          if (element.scrollTop !== before && Math.abs(velocity.current) < 1) {
            const step = Math.sign(direction), neighbour = (step > 0 ? row.nextElementSibling : row.previousElementSibling) as HTMLElement | null
            if (neighbour) {
              const centre = bounds.top + neighbour.offsetTop - (element.firstElementChild as HTMLElement).offsetTop - element.scrollTop + neighbour.offsetHeight / 2
              if (step * (point.y + grabOffset - centre) > 0) latestMove.current(id, step)
            }
          }
        }
        frame.current = requestAnimationFrame(scroll)
      }
      frame.current = requestAnimationFrame(scroll)
    },
    onDrag: (_: unknown, info: PanInfo) => { pointer.current = info.point; velocity.current = info.velocity.y },
    onDragEnd: stop,
  }
}
