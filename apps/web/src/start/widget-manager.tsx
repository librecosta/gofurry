import { useId, useLayoutEffect, useRef, useState, type KeyboardEvent, type RefObject } from 'react'
import { Tabs } from '@base-ui/react/tabs'
import { ArrowDownIcon } from '@phosphor-icons/react/dist/ssr/ArrowDown'
import { ArrowUpIcon } from '@phosphor-icons/react/dist/ssr/ArrowUp'
import { CheckIcon } from '@phosphor-icons/react/dist/ssr/Check'
import { DotsSixVerticalIcon } from '@phosphor-icons/react/dist/ssr/DotsSixVertical'
import { TrashIcon } from '@phosphor-icons/react/dist/ssr/Trash'
import { motion, Reorder, useDragControls } from 'motion/react'
import { useMotionTiming } from '../motion'
import { usePreferences } from '../preferences'
import { cx } from '../styles/classes'
import controls from './controls.module.scss'
import { useRailMessages } from './rail-messages'
import { getWidget, widgetCatalog, widgetCategories, type WidgetId } from './widget-catalog'
import { useWidgetRails } from './widget-rails-state'
import { moveWidget, widgetLimit } from './widget-order'
import styles from './widget-manager.module.scss'

const categories = ['all', ...new Set(widgetCatalog.map(widget => widget.category))] as const
type Category = typeof categories[number]
const pageSize = 9

export default function WidgetManager() {
  const { locale } = usePreferences()
  const m = useRailMessages()
  const { widgets, toggleWidget } = useWidgetRails()
  const [tab, setTab] = useState<'added' | 'library'>('library')
  const [category, setCategory] = useState<Category>('all')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(0)
  const search = query.trim().toLowerCase()
  const matches = widgetCatalog.filter(widget => (category === 'all' || widget.category === category)
    && Object.values(widget.name).some(name => name.toLowerCase().includes(search)))
  const pages = Math.max(1, Math.ceil(matches.length / pageSize))
  const currentPage = Math.min(page, pages - 1)
  const browse = () => { setTab('library'); setPage(0) }

  return <Tabs.Root value={tab} onValueChange={value => setTab(value === 'added' ? 'added' : 'library')} className={styles.workspace}>
    <aside className={styles.sidebar}>
      <input type="search" aria-label={m.search} placeholder={m.search} className={styles.search} value={query} maxLength={80}
        onChange={event => { setQuery(event.target.value); browse() }} />
      <CategoryRail selected={category} select={id => { setCategory(id); browse() }} />
    </aside>
    <section className={styles.content}>
      <Tabs.List aria-label={m.tabs} className={styles.tabs}>
        <Tabs.Tab value="added" className={cx(controls.glass, styles.tab)}>{m.added}</Tabs.Tab>
        <Tabs.Tab value="library" className={cx(controls.glass, styles.tab)}>{m.library}</Tabs.Tab>
      </Tabs.List>
      <Tabs.Panel value="added" className={styles.panel}><AddedWidgets /></Tabs.Panel>
      <Tabs.Panel value="library" className={styles.panel}>
        <div className={styles.grid}>
          {matches.slice(currentPage * pageSize, (currentPage + 1) * pageSize).map(({ id, Icon, name, description }) => {
            const added = widgets.includes(id)
            return <article key={id} className={styles.card} data-widget-card={id}>
              {added && <span className={styles.badge} aria-label={m.added} role="img"><CheckIcon size={15} weight="bold" aria-hidden="true" /></span>}
              <Icon size={24} aria-hidden="true" />
              <h3>{name[locale]}</h3>
              <p>{description[locale]}</p>
              <button type="button" disabled={!added && widgets.length >= widgetLimit} aria-label={`${added ? m.remove : m.add} ${name[locale]}`}
                className={cx(controls.glass, styles.button, styles.cardAction)} onClick={() => toggleWidget(id)}>{added ? m.remove : m.add}</button>
            </article>
          })}
          {matches.length === 0 && <p role="status" className={styles.empty}>{m.noMatches}</p>}
        </div>
        {pages > 1 && <footer className={styles.pagination}>
          <button type="button" className={cx(controls.glass, styles.button)} disabled={currentPage === 0}
            onClick={() => setPage(currentPage - 1)}>{m.previous}</button>
          <span>{currentPage + 1} / {pages}</span>
          <button type="button" className={cx(controls.glass, styles.button)} disabled={currentPage + 1 >= pages}
            onClick={() => setPage(currentPage + 1)}>{m.next}</button>
        </footer>}
      </Tabs.Panel>
    </section>
  </Tabs.Root>
}

function hookPath(y: number, preview = false) {
  const bend = Math.max(0, y - 6)
  return `M2 ${preview ? Math.max(0, bend - 9) : 0} V${bend} Q2 ${y} 8 ${y} H19`
}

function CategoryRail({ selected, select }: { selected: Category; select: (id: Category) => void }) {
  const { locale } = usePreferences()
  const m = useRailMessages()
  const rail = useRef<HTMLDivElement>(null)
  const [centers, setCenters] = useState<number[]>([])
  const [hovered, setHovered] = useState<Category | null>(null)
  const [focused, setFocused] = useState<Category | null>(null)
  const timing = useMotionTiming(0.36)
  const position = timing.duration ? { type: 'spring', duration: timing.duration, bounce: 0.12 } as const : timing

  useLayoutEffect(() => {
    const group = rail.current!
    const rows = Array.from(group.querySelectorAll('button'))
    const measure = () => {
      const next = rows.map(row => row.offsetTop + row.offsetHeight / 2)
      setCenters(current => next.every((y, i) => y === current[i]) && next.length === current.length ? current : next)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(group)
    rows.forEach(row => observer.observe(row))
    return () => observer.disconnect()
  }, [])

  const activeY = centers[categories.indexOf(selected)]
  const preview = hovered ?? focused
  const previewY = preview && preview !== selected ? centers[categories.indexOf(preview)] : undefined

  function navigate(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? categories.length - 1
      : event.key === 'ArrowUp' ? Math.max(0, index - 1) : event.key === 'ArrowDown' ? Math.min(categories.length - 1, index + 1) : undefined
    if (next === undefined) return
    event.preventDefault()
    setHovered(null)
    rail.current?.querySelectorAll('button')[next]?.focus()
    setFocused(categories[next])
  }

  return <div ref={rail} role="group" aria-label={m.categories} className={styles.categories}
    onPointerLeave={() => setHovered(null)} onPointerCancel={() => setHovered(null)}>
    <svg className={styles.categoryRail} aria-hidden="true" width="22" height="100%" fill="none">
      <line x1="2" x2="2" y1="0" y2="100%" className={styles.railLine} />
      {activeY !== undefined && <>
        <motion.path data-widget-category-preview className={styles.previewHook} initial={false}
          animate={{ d: hookPath(previewY ?? activeY, true), opacity: previewY === undefined ? 0 : 1 }}
          transition={{ d: position, opacity: { ...timing, duration: timing.duration ? 0.16 : 0 } }} />
        <motion.path data-widget-category-active className={styles.activeHook} initial={false}
          animate={{ d: hookPath(activeY) }} transition={position} />
      </>}
    </svg>
    {categories.map((id, index) => <button key={id} type="button" className={styles.category}
      aria-pressed={selected === id} onClick={() => select(id)}
      onPointerEnter={event => { if (event.pointerType !== 'touch') setHovered(id) }}
      onFocus={event => { if (event.currentTarget.matches(':focus-visible')) setFocused(id) }}
      onBlur={() => setFocused(null)} onKeyDown={event => navigate(event, index)}>
      {id === 'all' ? m.all : widgetCategories[id].label[locale]}
    </button>)}
  </div>
}

function AddedWidgets() {
  const m = useRailMessages()
  const { widgets, reorder } = useWidgetRails()
  const [draft, setDraft] = useState<WidgetId[] | null>(null)
  const draggingOrder = useRef<WidgetId[] | null>(null)
  const list = useRef<HTMLUListElement>(null)
  const reorderHelp = useId()
  if (!widgets.length) return <p className={styles.empty}>{m.empty}</p>

  const order = draft?.length === widgets.length && draft.every(id => widgets.includes(id)) ? draft : widgets
  return <>
    {widgets.length > 1 && <p className={styles.help} id={reorderHelp}>{m.reorderHelp}</p>}
    <Reorder.Group ref={list} axis="y" values={order} onReorder={next => { draggingOrder.current = next; setDraft(next) }} className={styles.list}>
      {order.map((id, index) => <AddedWidget key={id} id={id} sortable={widgets.length > 1} help={reorderHelp} list={list}
        first={index === 0} last={index === order.length - 1}
        move={offset => reorder(moveWidget(widgets, id, offset))}
        commit={() => {
          if (draggingOrder.current) reorder(draggingOrder.current)
          draggingOrder.current = null
          setDraft(null)
        }} />)}
    </Reorder.Group>
  </>
}

function AddedWidget({ id, sortable, first, last, list, help, move, commit }: {
  id: WidgetId; sortable: boolean; first: boolean; last: boolean; list: RefObject<HTMLUListElement | null>
  help: string; move: (offset: -1 | 1) => void; commit: () => void
}) {
  const { locale } = usePreferences()
  const m = useRailMessages()
  const { toggleWidget } = useWidgetRails()
  const { Icon, name } = getWidget(id)
  const drag = useDragControls()
  const timing = useMotionTiming(0.36)
  return <Reorder.Item value={id} dragListener={false} dragControls={drag} dragConstraints={list} dragElastic={0} dragMomentum={false}
    layout="position" transition={timing} whileDrag={{ opacity: 0.8 }} onDragEnd={commit} className={styles.addedRow}>
    {sortable && <button type="button" className={cx(styles.rowAction, styles.grip)} aria-label={`${m.reorder} ${name[locale]}`} aria-describedby={help}
      onPointerDown={event => { if (event.button === 0) drag.start(event) }}
      onKeyDown={event => {
        if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
        event.preventDefault()
        move(event.key === 'ArrowUp' ? -1 : 1)
      }}><DotsSixVerticalIcon size={18} aria-hidden="true" /></button>}
    <Icon size={19} aria-hidden="true" />
    <span className={styles.rowName}>{name[locale]}</span>
    {sortable && <>
      <button type="button" className={styles.rowAction} disabled={first} aria-label={`${m.up} ${name[locale]}`} onClick={() => move(-1)}><ArrowUpIcon size={15} aria-hidden="true" /></button>
      <button type="button" className={styles.rowAction} disabled={last} aria-label={`${m.down} ${name[locale]}`} onClick={() => move(1)}><ArrowDownIcon size={15} aria-hidden="true" /></button>
    </>}
    <button type="button" className={styles.rowAction} aria-label={`${m.remove} ${name[locale]}`} onClick={() => toggleWidget(id)}>
      <TrashIcon size={16} aria-hidden="true" />
    </button>
  </Reorder.Item>
}
