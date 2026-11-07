import { useId, useRef, useState } from 'react'
import { Tabs } from '@base-ui/react/tabs'
import { CheckIcon } from '@phosphor-icons/react/dist/ssr/Check'
import { DotsSixVerticalIcon } from '@phosphor-icons/react/dist/ssr/DotsSixVertical'
import { Reorder, useDragControls } from 'motion/react'
import { useMotionTiming } from '../motion'
import { usePreferences } from '../preferences'
import { cx } from '../styles/classes'
import controls from './controls.module.scss'
import { useRailMessages } from './rail-messages'
import { getWidget, widgetCatalog, widgetCategories, type WidgetId } from './widget-catalog'
import { useWidgetRails } from './widget-rails-state'
import { moveWidget, widgetLimit } from './widget-order'
import styles from './widget-manager.module.scss'

const categories = [...new Set(widgetCatalog.map(widget => widget.category))]
const pageSize = 9

export default function WidgetManager() {
  const { locale } = usePreferences()
  const m = useRailMessages()
  const { widgets, toggleWidget } = useWidgetRails()
  const [tab, setTab] = useState<'added' | 'library'>('library')
  const [category, setCategory] = useState<'all' | keyof typeof widgetCategories>('all')
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
      <div role="group" aria-label={m.categories} className={styles.categories}>
        {(['all', ...categories] as const).map(id => <button key={id} type="button" className={cx(controls.glass, styles.category)}
          aria-pressed={category === id} onClick={() => { setCategory(id); browse() }}>
          {id === 'all' ? m.all : widgetCategories[id][locale]}
        </button>)}
      </div>
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
                className={cx(controls.glass, styles.button)} onClick={() => toggleWidget(id)}>{added ? m.remove : m.add}</button>
            </article>
          })}
          {matches.length === 0 && <p role="status" className={styles.empty}>{m.noMatches}</p>}
        </div>
        <footer className={styles.pagination}>
          <button type="button" className={cx(controls.glass, styles.button)} disabled={currentPage === 0}
            onClick={() => setPage(currentPage - 1)}>{m.previous}</button>
          <span>{currentPage + 1} / {pages}</span>
          <button type="button" className={cx(controls.glass, styles.button)} disabled={currentPage + 1 >= pages}
            onClick={() => setPage(currentPage + 1)}>{m.next}</button>
        </footer>
      </Tabs.Panel>
    </section>
  </Tabs.Root>
}

function AddedWidgets() {
  const m = useRailMessages()
  const { widgets, reorder } = useWidgetRails()
  const [draft, setDraft] = useState<WidgetId[] | null>(null)
  const draggingOrder = useRef<WidgetId[] | null>(null)
  const reorderHelp = useId()
  if (!widgets.length) return <p className={styles.empty}>{m.empty}</p>

  const order = draft?.length === widgets.length && draft.every(id => widgets.includes(id)) ? draft : widgets
  return <>
    {widgets.length > 1 && <p className={styles.help} id={reorderHelp}>{m.reorderHelp}</p>}
    <Reorder.Group axis="y" values={order} onReorder={next => { draggingOrder.current = next; setDraft(next) }} className={styles.list}>
      {order.map(id => <AddedWidget key={id} id={id} sortable={widgets.length > 1} help={reorderHelp}
        move={offset => reorder(moveWidget(widgets, id, offset))}
        commit={() => {
          if (draggingOrder.current) reorder(draggingOrder.current)
          draggingOrder.current = null
          setDraft(null)
        }} />)}
    </Reorder.Group>
  </>
}

function AddedWidget({ id, sortable, help, move, commit }: {
  id: WidgetId; sortable: boolean; help: string; move: (offset: -1 | 1) => void; commit: () => void
}) {
  const { locale } = usePreferences()
  const m = useRailMessages()
  const { toggleWidget } = useWidgetRails()
  const { Icon, name } = getWidget(id)
  const drag = useDragControls()
  const timing = useMotionTiming(0.24)
  return <Reorder.Item value={id} dragListener={false} dragControls={drag} dragMomentum={false}
    layout="position" transition={timing} whileDrag={{ opacity: 0.8 }} onDragEnd={commit} className={styles.addedRow}>
    <Icon size={21} aria-hidden="true" />
    <span>{name[locale]}</span>
    <button type="button" className={cx(controls.glass, styles.button)} aria-label={`${m.remove} ${name[locale]}`}
      onClick={() => toggleWidget(id)}>{m.remove}</button>
    {sortable && <button type="button" className={cx(controls.glass, styles.grip)} aria-label={`${m.reorder} ${name[locale]}`} aria-describedby={help}
      onPointerDown={event => { if (event.button === 0) drag.start(event) }}
      onKeyDown={event => {
        if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
        event.preventDefault()
        move(event.key === 'ArrowUp' ? -1 : 1)
      }}><DotsSixVerticalIcon size={20} aria-hidden="true" /></button>}
  </Reorder.Item>
}
