import { cx } from '../styles/classes'
import { Link } from '@tanstack/react-router'
import { useId, useRef, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { useMotionTiming } from '../motion'
import { HouseIcon } from '@phosphor-icons/react/dist/ssr/House'
import { CompassIcon } from '@phosphor-icons/react/dist/ssr/Compass'
import { GameControllerIcon } from '@phosphor-icons/react/dist/ssr/GameController'
import { UserCircleIcon } from '@phosphor-icons/react/dist/ssr/UserCircle'
import { SignInIcon } from '@phosphor-icons/react/dist/ssr/SignIn'
import { Menu } from '@base-ui/react/menu'
import { Popover } from '@base-ui/react/popover'
import styles from './chrome.module.scss'
import { usePreferences } from '../preferences'
import controls from './controls.module.scss'
import { cnFiling } from './filing.config'
import { useStartMessages } from './messages'
import { localizedPath } from '../locale-path'
import { StartSettings } from './settings'
import { StartWeather } from './weather'

export function StartHeader() {
  const { locale, region } = usePreferences()
  const m = useStartMessages()
  const home = localizedPath('/', locale, region)
  const [brandOpen, setBrandOpen] = useState(false)
  const [userOpen, setUserOpen] = useState(false)
  const brandPopup = useRef<HTMLDivElement>(null)
  const unavailableId = useId()
  const transition = useMotionTiming()
  // Keep text on its final pixel grid throughout the fade, including scaled displays.
  const animation = { initial: { opacity: 0 }, animate: { opacity: 1 }, exit: { opacity: 0 }, transition }
  return <header className={styles.header}>
    <Popover.Root open={brandOpen} onOpenChange={setBrandOpen}>
      <Popover.Trigger aria-label={m.brand} className={styles.brandTrigger}>
        <span role="img" aria-label="GoFurry" data-start-logo className={styles.logo} />
      </Popover.Trigger>
      <AnimatePresence>{brandOpen && <Popover.Portal keepMounted>
        <Popover.Positioner positionMethod="fixed" align="start" sideOffset={8} collisionPadding={16} className={styles.menuPositioner}>
          <Popover.Popup ref={brandPopup} data-start-brand initialFocus={type => type === 'keyboard' ? true : brandPopup.current}
            className={cx(styles.menu, styles.brandPopup)} render={<motion.div {...animation} />}>
            <Popover.Title className={styles.srOnly}>{m.navigation}</Popover.Title>
            <p id={unavailableId} className={styles.srOnly}>{m.unavailable}</p>
            <nav aria-label={m.navigation}>
              {home === '/' ? <Popover.Close nativeButton={false} render={<Link to="/" />} className={styles.navItem}><HouseIcon size={17} aria-hidden="true" />{m.home}</Popover.Close>
                : <button type="button" aria-disabled="true" aria-describedby={unavailableId} data-target={home} className={cx(styles.navItem, styles.pending)}><HouseIcon size={17} aria-hidden="true" />{m.home}</button>}
              <button type="button" aria-disabled="true" aria-describedby={unavailableId} data-target={localizedPath('/sites', locale, region)} className={cx(styles.navItem, styles.pending)}><CompassIcon size={17} aria-hidden="true" />{m.sites}</button>
              <button type="button" aria-disabled="true" aria-describedby={unavailableId} data-target={localizedPath('/games', locale, region)} className={cx(styles.navItem, styles.pending)}><GameControllerIcon size={17} aria-hidden="true" />{m.games}</button>
            </nav>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>}</AnimatePresence>
    </Popover.Root>
    <div data-start-actions className={styles.actions}>
      <StartWeather />
      <StartSettings />
      <Menu.Root open={userOpen} onOpenChange={setUserOpen} modal={false} highlightItemOnHover={false}>
        <Menu.Trigger aria-label={m.user} className={cx(controls.header, styles.action)}><UserCircleIcon size={21} aria-hidden="true" /></Menu.Trigger>
        <AnimatePresence>{userOpen && <Menu.Portal keepMounted>
          <Menu.Positioner positionMethod="fixed" align="end" sideOffset={8} className={styles.menuPositioner}>
            <Menu.Popup aria-label={m.user} data-start-user-menu className={styles.menu} render={<motion.div {...animation} />}>
              <Menu.Item disabled closeOnClick={false} aria-label={`${m.login} (${m.unavailable})`} className={styles.menuItem}>
                <SignInIcon size={18} aria-hidden="true" />{m.login}
              </Menu.Item>
            </Menu.Popup>
          </Menu.Positioner>
        </Menu.Portal>}</AnimatePresence>
      </Menu.Root>
    </div>
  </header>
}

export function StartFooter() {
  const { region } = usePreferences()
  const m = useStartMessages()
  if (region !== 'CN') return null
  const hasICP = !!cnFiling.icp || cnFiling.showPending
  const hasSecurity = !!cnFiling.publicSecurity || cnFiling.showPending
  return <footer data-start-filing className={styles.footer}>
    <span className={styles.text}>{m.company}</span>
    <div className={cx(styles.text, styles.filings)}>
      {cnFiling.icp ? <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer" className={styles.footerLink}>{cnFiling.icp.number}</a>
        : cnFiling.showPending && <span>{m.icpPending}</span>}
      {hasICP && hasSecurity && <span aria-hidden="true">·</span>}
      {cnFiling.publicSecurity ? <a href={cnFiling.publicSecurity.queryUrl} target="_blank" rel="noopener noreferrer" className={styles.footerLink}>
        <img src={cnFiling.publicSecurity.icon} alt={m.securityIcon} width={14} height={14} />{cnFiling.publicSecurity.number}
      </a> : cnFiling.showPending && <span>{m.securityPending}</span>}
    </div>
  </footer>
}
