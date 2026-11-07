import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Popover } from '@base-ui/react/popover'
import { AnimatePresence, motion } from 'motion/react'
import { useMotionTiming } from '../motion'
import { cx } from '../styles/classes'
import chrome from './chrome.module.scss'
import controls from './controls.module.scss'
import styles from './homepage-guide.module.scss'
import { useStartMessages } from './messages'

const mobileQuery = '(max-width: 767px), (hover: none) and (pointer: coarse)'

export function HomepageGuide({ open, blocked, setOpen }: {
  open: boolean; blocked: boolean; setOpen: (open: boolean) => void
}) {
  const m = useStartMessages().homepagePrompt
  const [isExtensionPage, setIsExtensionPage] = useState(false)
  const popup = useRef<HTMLDivElement>(null)
  const transition = useMotionTiming(0.18)

  useLayoutEffect(() => {
    if (['chrome-extension:', 'moz-extension:', 'safari-web-extension:'].includes(window.location.protocol)) setIsExtensionPage(true)
  }, [])

  useEffect(() => {
    if (!open) return
    const mobile = matchMedia(mobileQuery)
    const closeOnMobile = () => { if (mobile.matches) setOpen(false) }
    closeOnMobile()
    mobile.addEventListener('change', closeOnMobile)
    return () => mobile.removeEventListener('change', closeOnMobile)
  }, [open, setOpen])

  if (isExtensionPage) return null

  return <Popover.Root open={open} onOpenChange={setOpen}>
    <Popover.Trigger data-start-homepage className={cx(controls.header, chrome.action, styles.trigger)}>{m.entry}</Popover.Trigger>
    {!blocked && <AnimatePresence>{open && <Popover.Portal keepMounted>
      <Popover.Positioner positionMethod="fixed" side="bottom" align="center"
        sideOffset={6} collisionPadding={16} collisionAvoidance={{ side: 'none', align: 'shift', fallbackAxisSide: 'none' }} className={styles.positioner}>
        <Popover.Popup ref={popup} data-start-homepage-guide className={cx(chrome.menu, styles.popup)}
          initialFocus={type => type === 'keyboard' ? true : popup.current}
          render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition} />}>
          <Popover.Title className={styles.title}>{m.title}</Popover.Title>
          <Popover.Description className={styles.description}>{m.description}</Popover.Description>
          <p className={styles.notice}>{m.unreleased}</p>
          <div className={styles.actions}>
            <button type="button" disabled className={cx(controls.glass, styles.button)}>{m.download}</button>
            <Popover.Close className={cx(controls.glass, styles.button)}>{m.close}</Popover.Close>
          </div>
        </Popover.Popup>
      </Popover.Positioner>
    </Popover.Portal>}</AnimatePresence>}
  </Popover.Root>
}
