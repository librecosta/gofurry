import { lazy, useEffect, useRef, useState, type ReactNode } from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { Popover } from '@base-ui/react/popover'
import { CalculatorIcon } from '@phosphor-icons/react/dist/ssr/Calculator'
import { DiceFiveIcon } from '@phosphor-icons/react/dist/ssr/DiceFive'
import { RulerIcon } from '@phosphor-icons/react/dist/ssr/Ruler'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr/Plus'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import { AnimatePresence, motion } from 'motion/react'
import { useMotionTiming } from '../motion'
import { usePreferences } from '../preferences'
import { cx } from '../styles/classes'
import chrome from './chrome.module.scss'
import controls from './controls.module.scss'
import settings from './settings.module.scss'
import { RailContent } from './rail-content'
import { useRailMessages } from './rail-messages'
import styles from './side-rails.module.scss'
import { getWidget } from './widget-catalog'
import { useWidgetRails } from './widget-rails-state'

const tools = [
  { id: 'calculator', Icon: CalculatorIcon, Panel: lazy(() => import('./tools/calculator')) },
  { id: 'random', Icon: DiceFiveIcon, Panel: lazy(() => import('./tools/random')) },
  { id: 'units', Icon: RulerIcon, Panel: lazy(() => import('./tools/units')) },
] as const
const WidgetManager = lazy(() => import('./widget-manager'))
const stackedQuery = '(max-width: 800px), (max-height: 399px)'

export function SideRails() {
  const { locale } = usePreferences()
  const m = useRailMessages()
  const { leftEnabled, rightEnabled, widgets } = useWidgetRails()
  const [active, setActive] = useState<string | null>(null)
  const change = (id: string, open: boolean) => setActive(current => open ? id : current === id ? null : current)
  const transition = useMotionTiming()

  useEffect(() => {
    if (!active) return
    const media = matchMedia(stackedQuery)
    const check = () => {
      const tool = tools.some(item => item.id === active)
      const widget = widgets.findIndex(id => id === active)
      const visible = tool ? leftEnabled : rightEnabled && (active === 'manager' || (widget >= 0 && (!media.matches || widget < 3)))
      if (!visible) setActive(null)
    }
    check()
    media.addEventListener('change', check)
    return () => media.removeEventListener('change', check)
  }, [active, leftEnabled, rightEnabled, widgets])

  if (!leftEnabled && !rightEnabled) return null

  return <div className={styles.rails}>
    {leftEnabled && <div role="group" aria-label={m.official} data-start-tools className={cx(styles.rail, styles.left)}>
      {tools.map(({ id, Icon, Panel }) => <RailPanel key={id} id={id} label={m[id]} icon={<Icon size={21} aria-hidden="true" />}
        side="right" active={active} change={change}><Panel /></RailPanel>)}
    </div>}
    {rightEnabled && <div role="group" aria-label={m.widgets} data-start-widgets className={cx(styles.rail, styles.right)}>
      {widgets.map(id => {
        const { Icon, Panel, name } = getWidget(id)
        return <div key={id} className={styles.widget}>
          <RailPanel id={id} label={name[locale]} icon={<Icon size={21} aria-hidden="true" />} side="left" active={active} change={change}><Panel /></RailPanel>
        </div>
      })}
      <Dialog.Root open={active === 'manager'} onOpenChange={value => change('manager', value)}>
        <Dialog.Trigger data-widget-manager-trigger className={cx(controls.glass, styles.trigger)} aria-label={m.manage}><PlusIcon size={21} aria-hidden="true" /></Dialog.Trigger>
        <AnimatePresence>{active === 'manager' && <Dialog.Portal keepMounted>
          <Dialog.Backdrop className={settings.backdrop} render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition} />} />
          <Dialog.Popup data-widget-manager className={cx(settings.popup, styles.manager)} render={<motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 6 }} transition={transition} />}>
            <div className={settings.heading}>
              <Dialog.Title className={settings.title}>{m.manage}</Dialog.Title>
              <Dialog.Close className={cx(controls.glass, settings.close)} aria-label={m.close}><XIcon size={18} aria-hidden="true" /></Dialog.Close>
            </div>
            <RailContent><WidgetManager /></RailContent>
          </Dialog.Popup>
        </Dialog.Portal>}</AnimatePresence>
      </Dialog.Root>
    </div>}
  </div>
}

function RailPanel({ id, label, icon, side, active, change, children }: {
  id: string; label: string; icon: ReactNode; side: 'left' | 'right'; active: string | null
  change: (id: string, open: boolean) => void; children: ReactNode
}) {
  const m = useRailMessages()
  const popup = useRef<HTMLDivElement>(null)
  const transition = useMotionTiming()
  return <Popover.Root open={active === id} onOpenChange={value => change(id, value)}>
    <Popover.Trigger data-rail-trigger={id} className={cx(controls.glass, styles.trigger)} aria-label={label}>{icon}</Popover.Trigger>
    {(!active || active === id) && <AnimatePresence>{active === id && <Popover.Portal keepMounted>
      <Popover.Positioner side={side} align="start" sideOffset={8} collisionPadding={12} positionMethod="fixed" className={styles.positioner}>
        <Popover.Popup ref={popup} data-rail-panel={id} className={cx(chrome.menu, styles.popup)}
          initialFocus={type => type === 'keyboard' ? true : popup.current}
          render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition} />}>
          <div className={settings.heading}>
            <Popover.Title className={styles.title}>{label}</Popover.Title>
            <Popover.Close className={cx(controls.glass, settings.close)} aria-label={m.close}><XIcon size={17} aria-hidden="true" /></Popover.Close>
          </div>
          <RailContent>{children}</RailContent>
        </Popover.Popup>
      </Popover.Positioner>
    </Popover.Portal>}</AnimatePresence>}
  </Popover.Root>
}
