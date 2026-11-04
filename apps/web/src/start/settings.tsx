import { cx } from '../styles/classes'
import { useId, useRef, useState } from 'react'
import { AnimatePresence, LayoutGroup, motion } from 'motion/react'
import { useMotionTiming } from '../motion'
import { Dialog } from '@base-ui/react/dialog'
import { Tabs } from '@base-ui/react/tabs'
import { Switch } from '@base-ui/react/switch'
import { useQuickMessages } from './quick-access-messages'
import { Radio } from '@base-ui/react/radio'
import { RadioGroup } from '@base-ui/react/radio-group'
import { GearSixIcon } from '@phosphor-icons/react/dist/ssr/GearSix'
import { GithubLogoIcon } from '@phosphor-icons/react/dist/ssr/GithubLogo'
import { MonitorIcon } from '@phosphor-icons/react/dist/ssr/Monitor'
import { SunIcon } from '@phosphor-icons/react/dist/ssr/Sun'
import { MoonIcon } from '@phosphor-icons/react/dist/ssr/Moon'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import styles from './settings.module.scss'
import { usePreferences, type ContentPreference, type Locale, type ThemePreference } from '../preferences'
import controls from './controls.module.scss'
import { useStartMessages } from './messages'
import { useThemeReveal } from './theme-reveal'

export function StartSettings() {
  const { region, locale, themePreference, contentPreference, setLocale, setContentPreference, quickAccess, setQuickAccess } = usePreferences()
  const m = useStartMessages()
  const q = useQuickMessages()
  const themes = useRef<HTMLDivElement>(null)
  const reveal = useThemeReveal()
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState('basic')
  const layoutId = useId()
  const transition = useMotionTiming()
  const selectionTransition = useMotionTiming(0.4)
  const radioTransition = useMotionTiming(0.3)
  return <Dialog.Root open={open} onOpenChange={(value) => { if (value) setTab('basic'); setOpen(value) }}>
    <Dialog.Trigger aria-label={m.settings} title={m.settings} data-start-settings className={cx(controls.header, styles.trigger)}><GearSixIcon size={21} aria-hidden="true" /></Dialog.Trigger>
    <AnimatePresence>{open && <Dialog.Portal keepMounted>
      <Dialog.Backdrop className={styles.backdrop} render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition} />} />
      <Dialog.Popup data-start-preferences className={styles.popup} render={<motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 6 }} transition={transition} />}>
        <div className={styles.heading}>
          <Dialog.Title className={styles.title}>{m.settings}</Dialog.Title>
          <Dialog.Close aria-label={m.close} title={m.close} className={cx(controls.glass, styles.close)}><XIcon size={18} aria-hidden="true" /></Dialog.Close>
        </div>
        <LayoutGroup id={layoutId}><Tabs.Root value={tab} onValueChange={(value) => setTab(String(value))}>
          <Tabs.List aria-label={m.settings} activateOnFocus className={styles.tabs}>
            <Tabs.Tab value="basic" className={styles.tab}>{m.basic}{tab === 'basic' && <motion.span layoutId="tab" data-settings-tab-indicator className={styles.tabIndicator} transition={selectionTransition} />}</Tabs.Tab>
            <Tabs.Tab value="advanced" className={styles.tab}>{m.advanced}{tab === 'advanced' && <motion.span layoutId="tab" data-settings-tab-indicator className={styles.tabIndicator} transition={selectionTransition} />}</Tabs.Tab>
          </Tabs.List>
          <div className={styles.panels}>
            <Tabs.Panel value="basic" keepMounted className={styles.panel}>
              <fieldset className={styles.field}><legend className={styles.label}>{m.languageSetting}</legend>
                <RadioGroup aria-label={m.languageSetting} value={locale} onValueChange={(value: Locale) => setLocale(value)} className={styles.languages}>
                  <Radio.Root value="zh" className={cx(styles.radio, styles.language)}><span aria-hidden="true" className={styles.radioMark}><Radio.Indicator keepMounted className={styles.radioDot} render={<motion.span initial={false} animate={{ opacity: locale === 'zh' ? 1 : 0 }} transition={radioTransition} />} /></span>简体中文</Radio.Root>
                  <Radio.Root value="en" className={cx(styles.radio, styles.language)}><span aria-hidden="true" className={styles.radioMark}><Radio.Indicator keepMounted className={styles.radioDot} render={<motion.span initial={false} animate={{ opacity: locale === 'en' ? 1 : 0 }} transition={radioTransition} />} /></span>English</Radio.Root>
                </RadioGroup>
              </fieldset>
              <fieldset className={styles.field}><legend className={styles.label}>{m.appearance}</legend>
                <RadioGroup ref={themes} aria-label={m.appearance} value={themePreference} onValueChange={(value: ThemePreference) => {
                  const anchor = themes.current?.querySelector<HTMLElement>(`[data-theme-choice="${value}"]`)
                  if (anchor) reveal(anchor, value)
                }} className={cx(styles.track, styles.threeColumns)}>
                  <motion.span aria-hidden="true" data-settings-selection="appearance" className={styles.selection} initial={false} animate={{ x: `${['system', 'light', 'dark'].indexOf(themePreference) * 100}%` }} transition={selectionTransition} />
                  <Radio.Root value="system" data-theme-choice="system" className={cx(styles.radio, styles.segment)}><MonitorIcon size={16} aria-hidden="true" />{m.system}</Radio.Root>
                  <Radio.Root value="light" data-theme-choice="light" className={cx(styles.radio, styles.segment)}><SunIcon size={16} aria-hidden="true" />{m.light}</Radio.Root>
                  <Radio.Root value="dark" data-theme-choice="dark" className={cx(styles.radio, styles.segment)}><MoonIcon size={16} aria-hidden="true" />{m.dark}</Radio.Root>
                </RadioGroup>
              </fieldset>
              <label className={styles.quickSetting}>{q.toggle}<Switch.Root checked={quickAccess} onCheckedChange={setQuickAccess} className={styles.switch}>
                <Switch.Thumb className={styles.switchThumb} render={<motion.span initial={false} animate={{ x: quickAccess ? 16 : 0 }} transition={selectionTransition} />} />
              </Switch.Root></label>
              {region === 'INTL' && <fieldset className={styles.field}><legend className={styles.label}>{m.contentPreference}</legend>
                <RadioGroup aria-label={m.contentPreference} value={contentPreference} onValueChange={(value: ContentPreference) => setContentPreference(value)} className={cx(styles.track, styles.twoColumns)}>
                  <motion.span aria-hidden="true" data-settings-selection="content" className={styles.selection} initial={false} animate={{ x: contentPreference === 'SFW' ? '0%' : '100%' }} transition={selectionTransition} />
                  <Radio.Root value="SFW" className={cx(styles.radio, styles.segment)}>SFW</Radio.Root>
                  <Radio.Root value="NSFW" className={cx(styles.radio, styles.segment)}>NSFW</Radio.Root>
                </RadioGroup>
                <p className={styles.hint}>{m.contentHint}</p>
              </fieldset>}
            </Tabs.Panel>
            <Tabs.Panel value="advanced" keepMounted className={styles.panel} />
          </div>
        </Tabs.Root></LayoutGroup>
        <footer className={styles.footer}><GithubLogoIcon size={17} aria-hidden="true" className={styles.githubIcon} /><p className={styles.caption}>{m.githubLead}<a href="https://github.com/gofurry" target="_blank" rel="noopener noreferrer" aria-label={m.github} className={styles.github}>GitHub</a>{m.githubStop}</p></footer>
      </Dialog.Popup>
    </Dialog.Portal>}</AnimatePresence>
  </Dialog.Root>
}
