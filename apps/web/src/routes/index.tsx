import { createFileRoute, Link } from '@tanstack/react-router'
import { useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { useMotionTiming } from '../motion'
import { Popover } from '@base-ui/react/popover'
import styles from './index.module.scss'
import { useMessages, usePreferences } from '../preferences'

export const Route = createFileRoute('/')({ component: RuntimeCheck })

function RuntimeCheck() {
  const { region, theme, locale, setTheme, setLocale } = usePreferences()
  const m = useMessages()
  const [open, setOpen] = useState(false)
  const transition = useMotionTiming()
  return (
    <main className={styles.page}>
      <h1>GoFurry runtime skeleton</h1>
      <p>V0.1-C0 · SSR / SCSS Modules / Motion / Base UI</p>
      <p><code className={styles.code}>{region} / SFW</code></p>
      <label>{m.theme} <select aria-label="Theme" value={theme} onChange={(e) => setTheme(e.target.value as 'light' | 'dark')}>
        <option value="light">Light</option><option value="dark">Dark</option>
      </select></label>{' '}
      <label>{m.language} <select aria-label="Language" value={locale} onChange={(e) => setLocale(e.target.value as 'zh' | 'en')}>
        <option value="zh">中文</option><option value="en">English</option>
      </select></label>
      <hr />
      <Popover.Root open={open} onOpenChange={setOpen}>
        <Popover.Trigger className={styles.button}>{m.interact}</Popover.Trigger>
        <AnimatePresence>{open && <Popover.Portal keepMounted>
          <Popover.Positioner sideOffset={8}>
            <Popover.Popup className={styles.popup} render={<motion.div initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -4 }} transition={transition} />}>
              <Popover.Title>{m.title}</Popover.Title>
              <Popover.Description>{m.description}</Popover.Description>
              <Popover.Close className={styles.button}>{m.close}</Popover.Close>
            </Popover.Popup>
          </Popover.Positioner>
        </Popover.Portal>}</AnimatePresence>
      </Popover.Root>
      {' '}
      <Popover.Root>
        <Popover.Trigger disabled className={styles.button}>{m.disabled}</Popover.Trigger>
      </Popover.Root>
      <p><Link to="/status">{m.status}</Link></p>
    </main>
  )
}
