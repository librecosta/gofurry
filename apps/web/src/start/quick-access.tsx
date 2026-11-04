import { useRef, useState } from 'react'
import { AnimatePresence, motion, Reorder, useDragControls, useIsPresent } from 'motion/react'
import { Dialog } from '@base-ui/react/dialog'
import { Menu } from '@base-ui/react/menu'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr/Plus'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr/Globe'
import { DotsThreeIcon } from '@phosphor-icons/react/dist/ssr/DotsThree'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import { DotsSixVerticalIcon } from '@phosphor-icons/react/dist/ssr/DotsSixVertical'
import { ArrowUpIcon } from '@phosphor-icons/react/dist/ssr/ArrowUp'
import { ArrowDownIcon } from '@phosphor-icons/react/dist/ssr/ArrowDown'
import { PencilSimpleIcon } from '@phosphor-icons/react/dist/ssr/PencilSimple'
import { TrashIcon } from '@phosphor-icons/react/dist/ssr/Trash'
import { PushPinIcon } from '@phosphor-icons/react/dist/ssr/PushPin'
import { useMotionTiming } from '../motion'
import { cx } from '../styles/classes'
import { useQuickAccess, siteKey, siteURL, visibleSites, type PinnedSite, type QuickSite } from './quick-access-data'
import { useQuickMessages } from './quick-access-messages'
import settings from './settings.module.scss'
import chrome from './chrome.module.scss'
import controls from './controls.module.scss'
import styles from './quick-access.module.scss'

function SiteIcon({ url }: { url: string }) {
  const [failed, setFailed] = useState(false), [loaded, setLoaded] = useState(false)
  return <span className={styles.icon} aria-hidden="true">
    {!loaded && <GlobeIcon size={32} weight="light" />}
    {!failed && <img src={new URL('/favicon.ico', siteURL(url)!).href} alt="" width={34} height={34} loading="lazy" decoding="async" referrerPolicy="no-referrer"
      style={{ visibility: loaded ? 'visible' : 'hidden' }} onLoad={() => setLoaded(true)} onError={() => { setFailed(true); setLoaded(false) }} />}
  </span>
}

export function QuickAccess() {
  const { data, ready } = useQuickAccess()
  const m = useQuickMessages()
  const [manager, setManager] = useState<{ edit?: string } | null>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const transition = useMotionTiming()
  const animation = { initial: { opacity: 0, y: 4 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0, y: 4 }, transition }
  return <>
    <motion.div data-quick-access className={styles.row} initial={false} animate={{ opacity: ready ? 1 : 0 }} transition={transition}>
      <AnimatePresence initial={false}>{ready && visibleSites(data).map(site => <SiteEntry key={site.key} site={site} edit={id => setManager({ edit: id })} />)}</AnimatePresence>
      <button ref={trigger} data-quick-manage type="button" aria-label={m.title} title={m.title} disabled={!ready} className={cx(styles.tile, styles.manage)} onClick={() => setManager({})}><span className={styles.icon}><PlusIcon size={30} weight="light" aria-hidden="true" /></span></button>
    </motion.div>
    <Dialog.Root open={manager !== null} onOpenChange={open => { if (!open) setManager(null) }}>
      <AnimatePresence>{manager && <Dialog.Portal keepMounted>
        <Dialog.Backdrop className={settings.backdrop} render={<motion.div {...animation} />} />
        <Dialog.Popup data-quick-manager className={cx(settings.popup, styles.manager)} finalFocus={trigger} render={<motion.div {...animation} />}>
          <div className={settings.heading}><Dialog.Title className={settings.title}>{m.title}</Dialog.Title>
            <Dialog.Close className={cx(controls.glass, settings.close)} aria-label={m.close}><XIcon size={18} aria-hidden="true" /></Dialog.Close></div>
          <Dialog.Description className={styles.srOnly}>{m.pinned} · {m.recent}</Dialog.Description>
          <ManagerContent initialEdit={manager.edit} />
        </Dialog.Popup>
      </Dialog.Portal>}</AnimatePresence>
    </Dialog.Root>
  </>
}

function SiteEntry({ site, edit }: { site: QuickSite; edit: (id: string) => void }) {
  const { pin, unpin, remove, removeRecent, openSite } = useQuickAccess()
  const m = useQuickMessages(), transition = useMotionTiming()
  const [open, setOpen] = useState(false)
  const present = useIsPresent()
  const trigger = useRef<HTMLButtonElement>(null)
  const menu = () => {
    trigger.current?.focus({ preventScroll: true })
    if (!open) trigger.current?.click()
  }
  return <motion.div data-quick-site data-site-key={site.key} data-pinned={!!site.pin} className={styles.entry} inert={!present} layout="position" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition}>
    <a href={site.url} target="_blank" rel="noopener noreferrer" data-quick-name={site.name} className={styles.tile} title={site.name}
      onContextMenu={event => { event.preventDefault(); menu() }}
      onKeyDown={event => { if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) { event.preventDefault(); menu() } }}>
      <SiteIcon key={site.url} url={site.url} /><span className={styles.name}>{site.name}</span>
    </a>
    <Menu.Root open={open} onOpenChange={setOpen} modal={false} highlightItemOnHover={false}>
      <Menu.Trigger ref={trigger} aria-label={`${m.actions}: ${site.name}`} className={styles.more}><DotsThreeIcon size={18} aria-hidden="true" /></Menu.Trigger>
      <AnimatePresence>{open && <Menu.Portal keepMounted>
        <Menu.Positioner positionMethod="fixed" align="start" sideOffset={4} collisionPadding={12} className={chrome.menuPositioner}>
          <Menu.Popup data-quick-menu className={chrome.menu}
            finalFocus={() => !trigger.current || trigger.current.closest('[inert]') ? document.querySelector<HTMLButtonElement>('[data-quick-manage]') : trigger.current}
            render={<motion.div initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -4 }} transition={transition} />}>
            <Menu.Item className={chrome.menuItem} onClick={() => openSite(site.url, site.name)}>{m.open}</Menu.Item>
            {site.pin ? <>
              <Menu.Item className={chrome.menuItem} onClick={() => edit(site.pin!.id)}>{m.edit}</Menu.Item>
              <Menu.Item className={chrome.menuItem} title={m.unpinHint} onClick={() => unpin(site.pin!.id)}>{m.unpin}</Menu.Item>
              <Menu.Item className={chrome.menuItem} title={m.deleteHint} onClick={() => remove(site.key)}>{m.remove}</Menu.Item>
            </> : <>
              <Menu.Item className={chrome.menuItem} onClick={() => pin(site)}>{m.pin}</Menu.Item>
              <Menu.Item className={chrome.menuItem} onClick={() => removeRecent(site.key)}>{m.removeRecent}</Menu.Item>
            </>}
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>}</AnimatePresence>
    </Menu.Root>
  </motion.div>
}

function ManagerContent({ initialEdit }: { initialEdit?: string }) {
  const { data, save, pin, remove, removeRecent, reorder } = useQuickAccess()
  const m = useQuickMessages()
  const initial = data.pinned.find(p => p.id === initialEdit)
  const [editing, setEditing] = useState(initial?.id)
  const [name, setName] = useState(initial?.name ?? ''), [url, setURL] = useState(initial?.url ?? '')
  const [error, setError] = useState(false)
  const nameField = useRef<HTMLInputElement>(null)
  const reset = () => { setEditing(undefined); setName(''); setURL(''); setError(false) }
  const edit = (site: PinnedSite) => { setEditing(site.id); setName(site.name); setURL(site.url); setError(false); nameField.current?.focus() }
  const move = (id: string, delta: number) => {
    const next = [...data.pinned], index = next.findIndex(p => p.id === id), target = index + delta
    if (target < 0 || target >= next.length) return
    ;[next[index], next[target]] = [next[target], next[index]]; reorder(next)
  }
  return <>
    <form className={styles.form} aria-label={editing ? m.edit : m.add} onSubmit={event => {
      event.preventDefault()
      if (!save(name, url, editing)) { setError(true); return }
      reset(); nameField.current?.focus()
    }}>
      <label>{m.name}<input ref={nameField} name="site-name" value={name} onChange={e => setName(e.target.value)} required maxLength={80} autoComplete="off" /></label>
      <label>{m.url}<input name="site-url" value={url} onChange={e => setURL(e.target.value)} required maxLength={2048} inputMode="url" autoComplete="off" placeholder="https://example.com" /></label>
      <div className={styles.formActions}><button type="submit" className={styles.textButton}>{editing ? m.save : m.add}</button>{editing && <button type="button" className={styles.textButton} onClick={reset}>{m.cancel}</button>}</div>
      {error && <p role="alert" className={styles.hint}>{m.invalid}</p>}
    </form>
    <h3 className={styles.sectionTitle}>{m.pinned}</h3>
    {data.pinned.length === 0 && <p className={styles.hint}>{m.empty}</p>}
    <Reorder.Group axis="y" values={data.pinned} onReorder={reorder} layoutScroll className={styles.list} aria-label={m.pinned}>
      {data.pinned.map((site, index) => <PinnedRow key={site.id} site={site} first={index === 0} last={index === data.pinned.length - 1}
        edit={() => edit(site)} move={delta => move(site.id, delta)} remove={() => { remove(siteKey(site.url)); if (editing === site.id) reset() }} />)}
    </Reorder.Group>
    <details className={styles.recent}><summary>{m.recent} ({data.recent.length})</summary>
      {data.recent.length === 0 && <p className={styles.hint}>{m.noRecent}</p>}
      <ul className={styles.list}>{data.recent.map(site => {
        const key = siteKey(site.url), pinned = data.pinned.some(p => siteKey(p.url) === key)
        return <li key={key} className={styles.recentRow}>
          <span className={styles.rowName}>{site.name}<small>{new URL(site.url).host}</small></span>
          <button type="button" className={styles.smallButton} aria-label={`${m.pin}: ${site.name}`} title={m.pin} disabled={pinned} onClick={() => pin({ ...site, key })}><PushPinIcon size={16} aria-hidden="true" /></button>
          <button type="button" className={styles.smallButton} aria-label={`${m.removeRecent}: ${site.name}`} title={m.removeRecent} onClick={() => removeRecent(key)}><XIcon size={16} aria-hidden="true" /></button>
        </li>
      })}</ul>
    </details>
  </>
}

function PinnedRow({ site, first, last, edit, move, remove }: { site: PinnedSite; first: boolean; last: boolean; edit: () => void; move: (delta: number) => void; remove: () => void }) {
  const drag = useDragControls(), m = useQuickMessages(), transition = useMotionTiming()
  return <Reorder.Item value={site} data-quick-pin={site.id} dragListener={false} dragControls={drag} dragElastic={0} dragMomentum={false} layout="position" transition={transition} className={styles.pinRow}>
    <button type="button" className={cx(styles.smallButton, styles.drag)} aria-label={`${m.drag}: ${site.name}`} title={m.drag} onPointerDown={event => drag.start(event)}
      onKeyDown={event => { if (event.key === 'ArrowUp' || event.key === 'ArrowDown') { event.preventDefault(); move(event.key === 'ArrowUp' ? -1 : 1) } }}><DotsSixVerticalIcon size={18} aria-hidden="true" /></button>
    <span className={styles.rowName}>{site.name}<small>{new URL(site.url).host}</small></span>
    <button type="button" className={styles.smallButton} disabled={first} aria-label={`${m.up}: ${site.name}`} title={m.up} onClick={() => move(-1)}><ArrowUpIcon size={15} aria-hidden="true" /></button>
    <button type="button" className={styles.smallButton} disabled={last} aria-label={`${m.down}: ${site.name}`} title={m.down} onClick={() => move(1)}><ArrowDownIcon size={15} aria-hidden="true" /></button>
    <button type="button" className={styles.smallButton} aria-label={`${m.edit}: ${site.name}`} title={m.edit} onClick={edit}><PencilSimpleIcon size={15} aria-hidden="true" /></button>
    <button type="button" className={styles.smallButton} aria-label={`${m.remove}: ${site.name}`} title={m.deleteHint} onClick={remove}><TrashIcon size={15} aria-hidden="true" /></button>
  </Reorder.Item>
}
