import { useEffect, useId, useRef, useState } from 'react'
import { Popover } from '@base-ui/react/popover'
import { Radio } from '@base-ui/react/radio'
import { RadioGroup } from '@base-ui/react/radio-group'
import { Tabs } from '@base-ui/react/tabs'
import { Tooltip } from '@base-ui/react/tooltip'
import { CopyIcon } from '@phosphor-icons/react/dist/ssr/Copy'
import { HouseIcon } from '@phosphor-icons/react/dist/ssr/House'
import { AnimatePresence, motion } from 'motion/react'
import { useMotionTiming } from '../motion'
import { usePreferences } from '../preferences'
import { cx } from '../styles/classes'
import chrome from './chrome.module.scss'
import controls from './controls.module.scss'
import { detectHomepageBrowser, homepageAddress, homepageBrowsers, homepageMessages, type HomepageBrowser } from './homepage-guide-data'
import styles from './homepage-guide.module.scss'

export function HomepageGuide({ open, blocked, setOpen, hide }: {
  open: boolean; blocked: boolean; setOpen: (open: boolean) => void; hide: () => void
}) {
  const { region, locale } = usePreferences()
  const m = homepageMessages[locale]
  const [browser, setBrowser] = useState<HomepageBrowser>()
  const [origin, setOrigin] = useState('')
  const [tip, setTip] = useState(false)
  const tipId = useId()
  const popup = useRef<HTMLDivElement>(null)
  const transition = useMotionTiming()
  const tipTransition = useMotionTiming(0.16)

  useEffect(() => {
    if (!open) return
    const mobile = matchMedia('(max-width: 767px), (hover: none) and (pointer: coarse)')
    const closeOnMobile = () => { if (mobile.matches) setOpen(false) }
    closeOnMobile()
    mobile.addEventListener('change', closeOnMobile)
    return () => mobile.removeEventListener('change', closeOnMobile)
  }, [open, setOpen])

  const selected = browser ?? 'other'
  const address = homepageAddress(region, locale, origin)
  const tabGuide = selected === 'Safari' || selected === 'other' ? m.newTabSteps[selected] : [m.extension]
  const showTip = tip && !open && !blocked
  return <Popover.Root open={open} onOpenChange={value => {
    if (value) {
      setTip(false)
      setOrigin(window.location.origin)
      setBrowser(current => current ?? detectHomepageBrowser(navigator.userAgent))
    }
    setOpen(value)
  }}>
    <Tooltip.Root open={showTip} disabled={open || blocked} onOpenChange={setTip}>
      <Tooltip.Trigger delay={360} render={<Popover.Trigger />} aria-label={m.title} aria-describedby={showTip ? tipId : undefined}
        data-start-homepage className={cx(controls.header, chrome.action, styles.trigger)}><HouseIcon size={21} aria-hidden="true" /></Tooltip.Trigger>
      {!open && !blocked && <AnimatePresence>{showTip && <Tooltip.Portal keepMounted>
        <Tooltip.Positioner positionMethod="fixed" side="bottom" sideOffset={8} className={styles.positioner}>
          <Tooltip.Popup id={tipId} role="tooltip" className={cx(chrome.menu, styles.tooltip)} render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={tipTransition} />}>
            {m.title}
          </Tooltip.Popup>
        </Tooltip.Positioner>
      </Tooltip.Portal>}</AnimatePresence>}
    </Tooltip.Root>
    {!blocked && <AnimatePresence>{open && <Popover.Portal keepMounted>
      <Popover.Positioner positionMethod="fixed" align="end" sideOffset={8} collisionPadding={16} className={styles.positioner}>
        <Popover.Popup ref={popup} data-start-homepage-guide className={cx(chrome.menu, styles.popup)}
          initialFocus={type => type === 'keyboard' ? true : popup.current}
          render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition} />}>
          <Popover.Title className={styles.title}>{m.title}</Popover.Title>
          <Tabs.Root defaultValue="home" className={styles.guides}>
            <Tabs.List aria-label={m.title} activateOnFocus className={styles.tabs}>
              <Tabs.Tab value="home" className={cx(controls.glass, styles.choice)}>{m.homepage}</Tabs.Tab>
              <Tabs.Tab value="tab" className={cx(controls.glass, styles.choice)}>{m.newTab}</Tabs.Tab>
            </Tabs.List>
            <RadioGroup aria-label={m.browser} value={selected} onValueChange={setBrowser} className={styles.browsers}>
              {homepageBrowsers.map(name => <Radio.Root key={name} value={name} className={cx(controls.glass, styles.choice)}>{name}</Radio.Root>)}
            </RadioGroup>
            <Tabs.Panel value="home" className={styles.panel}>
              <ol>{m.homeSteps[selected].map(step => <li key={step}>{step}</li>)}</ol>
            </Tabs.Panel>
            <Tabs.Panel value="tab" className={styles.panel}>
              {tabGuide.length === 1 ? <p>{tabGuide[0]}</p> : <ol>{tabGuide.map(step => <li key={step}>{step}</li>)}</ol>}
            </Tabs.Panel>
          </Tabs.Root>
          <CopyAddress key={address.url} {...address} locale={locale} />
          <button type="button" onClick={hide} className={cx(controls.glass, styles.hide)}>{m.hide}</button>
        </Popover.Popup>
      </Popover.Positioner>
    </Popover.Portal>}</AnimatePresence>}
  </Popover.Root>
}

function CopyAddress({ url, local, locale }: ReturnType<typeof homepageAddress> & { locale: keyof typeof homepageMessages }) {
  const m = homepageMessages[locale]
  const [feedback, setFeedback] = useState('')
  async function copy() {
    try {
      await navigator.clipboard.writeText(url)
      setFeedback(m.copied)
    } catch {
      setFeedback(m.copyFailed)
    }
  }
  return <div className={styles.address}>
    <label className={styles.addressLabel}>
      <span>{local ? m.localAddress : m.address}</span>
      <input className={styles.url} readOnly value={url} spellCheck={false} />
    </label>
    <button type="button" aria-label={m.copy} className={cx(controls.glass, styles.copy)} onClick={() => void copy()}><CopyIcon size={18} aria-hidden="true" /></button>
    <span role="status" className={styles.feedback}>{feedback}</span>
  </div>
}
