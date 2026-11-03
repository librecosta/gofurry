import { useRef } from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { Radio } from '@base-ui/react/radio'
import { RadioGroup } from '@base-ui/react/radio-group'
import { GearSixIcon } from '@phosphor-icons/react/dist/ssr/GearSix'
import { GithubLogoIcon } from '@phosphor-icons/react/dist/ssr/GithubLogo'
import { MonitorIcon } from '@phosphor-icons/react/dist/ssr/Monitor'
import { SunIcon } from '@phosphor-icons/react/dist/ssr/Sun'
import { MoonIcon } from '@phosphor-icons/react/dist/ssr/Moon'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import * as stylex from '@stylexjs/stylex'
import { usePreferences, type ContentPreference, type Locale, type ThemePreference } from '../preferences'
import { tokens } from '../styles/tokens.stylex'
import { controls } from './controls.stylex'
import { useStartMessages } from './messages'
import { useThemeReveal } from './theme-reveal'

export function StartSettings() {
  const { region, locale, themePreference, contentPreference, setLocale, setContentPreference } = usePreferences()
  const m = useStartMessages()
  const themes = useRef<HTMLDivElement>(null)
  const reveal = useThemeReveal()
  return <Dialog.Root>
    <Dialog.Trigger aria-label={m.settings} title={m.settings} data-start-settings {...stylex.props(controls.glass, styles.trigger)}><GearSixIcon size={21} aria-hidden="true" /></Dialog.Trigger>
    <Dialog.Portal>
      <Dialog.Backdrop {...stylex.props(styles.backdrop)} />
      <Dialog.Popup data-start-preferences {...stylex.props(styles.popup)}>
        <div {...stylex.props(styles.heading)}>
          <Dialog.Title {...stylex.props(styles.title)}>{m.settings}</Dialog.Title>
          <Dialog.Close aria-label={m.close} title={m.close} {...stylex.props(controls.glass, styles.close)}><XIcon size={18} aria-hidden="true" /></Dialog.Close>
        </div>
        <fieldset {...stylex.props(styles.field)}><legend {...stylex.props(styles.label)}>{m.languageSetting}</legend>
          <RadioGroup aria-label={m.languageSetting} value={locale} onValueChange={(value: Locale) => setLocale(value)} {...stylex.props(styles.choices)}>
            <Radio.Root value="zh" {...stylex.props(controls.glass, styles.choice, locale === 'zh' && styles.selected)}>简体中文</Radio.Root>
            <Radio.Root value="en" {...stylex.props(controls.glass, styles.choice, locale === 'en' && styles.selected)}>English</Radio.Root>
          </RadioGroup>
        </fieldset>
        <fieldset {...stylex.props(styles.field)}><legend {...stylex.props(styles.label)}>{m.appearance}</legend>
          <RadioGroup ref={themes} aria-label={m.appearance} value={themePreference} onValueChange={(value: ThemePreference) => {
            const anchor = themes.current?.querySelector<HTMLElement>(`[data-theme-choice="${value}"]`)
            if (anchor) reveal(anchor, value)
          }} {...stylex.props(styles.choices)}>
            <Radio.Root value="system" data-theme-choice="system" {...stylex.props(controls.glass, styles.choice, themePreference === 'system' && styles.selected)}><MonitorIcon size={17} aria-hidden="true" />{m.system}</Radio.Root>
            <Radio.Root value="light" data-theme-choice="light" {...stylex.props(controls.glass, styles.choice, themePreference === 'light' && styles.selected)}><SunIcon size={17} aria-hidden="true" />{m.light}</Radio.Root>
            <Radio.Root value="dark" data-theme-choice="dark" {...stylex.props(controls.glass, styles.choice, themePreference === 'dark' && styles.selected)}><MoonIcon size={17} aria-hidden="true" />{m.dark}</Radio.Root>
          </RadioGroup>
        </fieldset>
        {region === 'INTL' && <fieldset {...stylex.props(styles.field)}><legend {...stylex.props(styles.label)}>{m.contentPreference}</legend>
          <RadioGroup aria-label={m.contentPreference} value={contentPreference} onValueChange={(value: ContentPreference) => setContentPreference(value)} {...stylex.props(styles.choices)}>
            <Radio.Root value="SFW" {...stylex.props(controls.glass, styles.choice, contentPreference === 'SFW' && styles.selected)}>SFW</Radio.Root>
            <Radio.Root value="NSFW" {...stylex.props(controls.glass, styles.choice, contentPreference === 'NSFW' && styles.selected)}>NSFW</Radio.Root>
          </RadioGroup>
          <p {...stylex.props(styles.hint)}>{m.contentHint}</p>
        </fieldset>}
        <a href="https://github.com/gofurry" target="_blank" rel="noopener noreferrer" aria-label={m.github} {...stylex.props(controls.glass, styles.github)}><GithubLogoIcon size={18} aria-hidden="true" />GitHub</a>
      </Dialog.Popup>
    </Dialog.Portal>
  </Dialog.Root>
}

const styles = stylex.create({
  trigger: { display: 'grid', placeItems: 'center', padding: 0, width: { default: 40, '@media (max-width: 767px)': 36 }, height: { default: 40, '@media (max-width: 767px)': 36 } },
  backdrop: {
    position: 'fixed', inset: 0, zIndex: 10, backgroundColor: 'rgba(0, 0, 0, 0.10)',
    opacity: { default: 1, ':is([data-starting-style], [data-ending-style])': 0 },
    transitionProperty: 'opacity', transitionDuration: '420ms', transitionTimingFunction: tokens.overlayEasing,
  },
  popup: {
    position: 'fixed', top: '50%', left: '50%', zIndex: 11, width: 'min(420px, calc(100vw - 32px))', maxHeight: 'calc(100dvh - 32px)', overflowY: 'auto', overscrollBehavior: 'contain',
    padding: 20, borderWidth: 0, borderRadius: tokens.radiusMedium, outlineStyle: 'none', boxShadow: 'none',
    backgroundColor: tokens.overlaySurface, color: tokens.overlayText, backdropFilter: 'blur(18px) saturate(1.05)',
    opacity: { default: 1, ':is([data-starting-style], [data-ending-style])': 0 },
    transform: { default: 'translate(-50%, -50%) translateY(0)', ':is([data-starting-style], [data-ending-style])': 'translate(-50%, -50%) translateY(6px)' },
    pointerEvents: { default: 'auto', ':is([data-ending-style])': 'none' },
    transitionProperty: 'opacity, transform', transitionDuration: '420ms', transitionTimingFunction: tokens.overlayEasing,
  },
  heading: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 20 },
  title: { margin: 0, fontSize: 18, lineHeight: 1.4, fontWeight: 600 },
  close: { display: 'grid', placeItems: 'center', width: 28, height: 28, padding: 0 },
  field: { minWidth: 0, borderWidth: 0, padding: 0, margin: 0, marginBottom: 20 },
  label: { fontSize: 13, fontWeight: 500, lineHeight: 1.5, padding: 0, marginBottom: 8 },
  choices: { display: 'flex', flexWrap: 'wrap', gap: 6 },
  choice: { display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 6, paddingBlock: 9, paddingInline: 12, fontSize: 13, fontWeight: 500, lineHeight: 1.5, userSelect: 'none' },
  selected: { backgroundColor: { default: tokens.overlaySelectedSurface, ':hover:not(:focus-visible)': tokens.overlaySurfaceHover, ':active:not(:focus-visible)': tokens.overlaySelectedSurface, ':focus-visible': tokens.overlaySelectedSurface } },
  hint: { margin: 0, marginTop: 8, fontSize: 12, lineHeight: 1.6, color: tokens.overlayText },
  github: { display: 'inline-flex', alignItems: 'center', gap: 8, paddingBlock: 8, paddingInline: 10, textDecoration: 'none', fontSize: 13, lineHeight: 1.5 },
})
