import { useRef } from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { Tabs } from '@base-ui/react/tabs'
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
        <Tabs.Root defaultValue="basic">
          <Tabs.List aria-label={m.settings} activateOnFocus {...stylex.props(styles.tabs)}>
            <Tabs.Tab value="basic" {...stylex.props(styles.tab)}>{m.basic}</Tabs.Tab>
            <Tabs.Tab value="advanced" {...stylex.props(styles.tab)}>{m.advanced}</Tabs.Tab>
            <Tabs.Indicator data-settings-tab-indicator {...stylex.props(styles.tabIndicator)} />
          </Tabs.List>
          <div {...stylex.props(styles.panels)}>
            <Tabs.Panel value="basic" keepMounted {...stylex.props(styles.panel)}>
              <fieldset {...stylex.props(styles.field)}><legend {...stylex.props(styles.label)}>{m.languageSetting}</legend>
                <RadioGroup aria-label={m.languageSetting} value={locale} onValueChange={(value: Locale) => setLocale(value)} {...stylex.props(styles.languages)}>
                  <Radio.Root value="zh" {...stylex.props(styles.radio, styles.language)}><span aria-hidden="true" {...stylex.props(styles.radioMark)}><Radio.Indicator keepMounted {...stylex.props(styles.radioDot)} /></span>简体中文</Radio.Root>
                  <Radio.Root value="en" {...stylex.props(styles.radio, styles.language)}><span aria-hidden="true" {...stylex.props(styles.radioMark)}><Radio.Indicator keepMounted {...stylex.props(styles.radioDot)} /></span>English</Radio.Root>
                </RadioGroup>
              </fieldset>
              <fieldset {...stylex.props(styles.field)}><legend {...stylex.props(styles.label)}>{m.appearance}</legend>
                <RadioGroup ref={themes} aria-label={m.appearance} value={themePreference} onValueChange={(value: ThemePreference) => {
                  const anchor = themes.current?.querySelector<HTMLElement>(`[data-theme-choice="${value}"]`)
                  if (anchor) reveal(anchor, value)
                }} {...stylex.props(styles.track, styles.columns(3))}>
                  <span aria-hidden="true" data-settings-selection="appearance" {...stylex.props(styles.selection(3, ['system', 'light', 'dark'].indexOf(themePreference)))} />
                  <Radio.Root value="system" data-theme-choice="system" {...stylex.props(styles.radio, styles.segment)}><MonitorIcon size={16} aria-hidden="true" />{m.system}</Radio.Root>
                  <Radio.Root value="light" data-theme-choice="light" {...stylex.props(styles.radio, styles.segment)}><SunIcon size={16} aria-hidden="true" />{m.light}</Radio.Root>
                  <Radio.Root value="dark" data-theme-choice="dark" {...stylex.props(styles.radio, styles.segment)}><MoonIcon size={16} aria-hidden="true" />{m.dark}</Radio.Root>
                </RadioGroup>
              </fieldset>
              {region === 'INTL' && <fieldset {...stylex.props(styles.field)}><legend {...stylex.props(styles.label)}>{m.contentPreference}</legend>
                <RadioGroup aria-label={m.contentPreference} value={contentPreference} onValueChange={(value: ContentPreference) => setContentPreference(value)} {...stylex.props(styles.track, styles.columns(2))}>
                  <span aria-hidden="true" data-settings-selection="content" {...stylex.props(styles.selection(2, contentPreference === 'SFW' ? 0 : 1))} />
                  <Radio.Root value="SFW" {...stylex.props(styles.radio, styles.segment)}>SFW</Radio.Root>
                  <Radio.Root value="NSFW" {...stylex.props(styles.radio, styles.segment)}>NSFW</Radio.Root>
                </RadioGroup>
                <p {...stylex.props(styles.hint)}>{m.contentHint}</p>
              </fieldset>}
            </Tabs.Panel>
            <Tabs.Panel value="advanced" keepMounted {...stylex.props(styles.panel)} />
          </div>
        </Tabs.Root>
        <footer {...stylex.props(styles.footer)}><GithubLogoIcon size={17} aria-hidden="true" {...stylex.props(styles.githubIcon)} /><p {...stylex.props(styles.caption)}>{m.githubLead}<a href="https://github.com/gofurry" target="_blank" rel="noopener noreferrer" aria-label={m.github} {...stylex.props(styles.github)}>GitHub</a>{m.githubStop}</p></footer>
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
  heading: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 12 },
  title: { margin: 0, fontSize: 18, lineHeight: 1.4, fontWeight: 600 },
  close: { display: 'grid', placeItems: 'center', width: 28, height: 28, padding: 0 },
  tabs: { position: 'relative', display: 'flex', gap: 24, marginBottom: 20 },
  tab: {
    paddingBlock: 8, paddingInline: 0, borderWidth: 0, borderRadius: 2, backgroundColor: 'transparent', boxShadow: 'none',
    fontFamily: 'inherit', fontSize: 13, lineHeight: 1.5, fontWeight: { default: 400, ':is([data-active])': 600 },
    color: { default: `color-mix(in srgb, ${tokens.overlayText} 78%, transparent)`, ':is([data-active])': tokens.overlaySelectedText, ':hover:not(:focus-visible)': tokens.overlayHoverText, ':active:not(:focus-visible)': tokens.overlaySelectedText },
    cursor: { default: 'pointer', ':disabled': 'not-allowed' }, opacity: { default: 1, ':disabled': 0.5 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: 3, outlineColor: tokens.overlayFocus,
    transitionProperty: 'color', transitionDuration: '300ms', transitionTimingFunction: tokens.overlayEasing,
  },
  tabIndicator: {
    position: 'absolute', left: 0, bottom: 0, height: 1.5, width: 'var(--active-tab-width)', transform: 'translateX(var(--active-tab-left))',
    backgroundColor: tokens.overlayText, pointerEvents: 'none',
    transitionProperty: 'transform, width', transitionDuration: '400ms', transitionTimingFunction: tokens.overlayEasing,
  },
  panels: { display: 'grid' },
  // Reserve Basic's natural height while Advanced is empty; switching tabs must
  // not resize/recenter the dialog. Base UI keeps the inactive panel inert.
  panel: {
    gridArea: '1 / 1', display: 'block', visibility: { default: 'visible', ':is([hidden], [inert])': 'hidden' },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: 4, outlineColor: tokens.overlayFocus,
  },
  field: { minWidth: 0, borderWidth: 0, padding: 0, margin: 0, marginBottom: 18 },
  label: { fontSize: 13, fontWeight: 500, lineHeight: 1.5, padding: 0, marginBottom: 8 },
  languages: { display: 'flex', gap: 24 },
  radio: {
    display: 'inline-flex', alignItems: 'center', gap: 8, borderWidth: 0, backgroundColor: 'transparent', boxShadow: 'none',
    fontFamily: 'inherit', fontSize: 13, fontWeight: 400, lineHeight: 1.5, userSelect: 'none',
    color: { default: tokens.overlayText, ':is([data-checked])': tokens.overlaySelectedText, ':hover:not([data-disabled]):not(:focus-visible)': tokens.overlayHoverText, ':active:not([data-disabled]):not(:focus-visible)': tokens.overlaySelectedText },
    cursor: { default: 'pointer', ':is([data-disabled])': 'not-allowed' }, opacity: { default: 1, ':is([data-disabled])': 0.5 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: -2, outlineColor: tokens.overlayFocus,
    transitionProperty: 'color', transitionDuration: '400ms', transitionTimingFunction: tokens.overlayEasing,
  },
  language: { paddingBlock: 6, borderRadius: 2, outlineOffset: 4 },
  radioMark: { display: 'grid', placeItems: 'center', width: 14, height: 14, flexShrink: 0, borderWidth: 1, borderStyle: 'solid', borderColor: 'currentColor', borderRadius: '50%' },
  radioDot: { display: 'block', width: 6, height: 6, borderRadius: '50%', backgroundColor: 'currentColor', opacity: { default: 0, ':is([data-checked])': 1 }, transitionProperty: 'opacity', transitionDuration: '300ms', transitionTimingFunction: tokens.overlayEasing },
  track: { position: 'relative', display: 'grid', padding: 3, borderRadius: 10, backgroundColor: `color-mix(in srgb, ${tokens.overlaySurface} 45%, transparent)` },
  columns: (count: number) => ({ gridTemplateColumns: `repeat(${count}, minmax(0, 1fr))` }),
  selection: (count: number, index: number) => ({
    position: 'absolute', left: 3, top: 3, bottom: 3, width: `calc((100% - 6px) / ${count})`, borderRadius: 8, pointerEvents: 'none',
    backgroundColor: `color-mix(in srgb, ${tokens.overlaySelectedSurface} 70%, transparent)`, transform: `translateX(${index * 100}%)`,
    transitionProperty: 'transform, background-color', transitionDuration: '400ms', transitionTimingFunction: tokens.overlayEasing,
  }),
  segment: {
    position: 'relative', justifyContent: 'center', minWidth: 0, height: 36, paddingInline: 6, gap: { default: 5, '@media (max-width: 400px)': 4 }, borderRadius: 8,
    fontSize: { default: 13, '@media (max-width: 400px)': 12 }, whiteSpace: 'nowrap',
    backgroundColor: { default: 'transparent', ':hover:not([data-disabled]):not(:focus-visible)': `color-mix(in srgb, ${tokens.overlaySurfaceHover} 35%, transparent)`, ':active:not([data-disabled]):not(:focus-visible)': `color-mix(in srgb, ${tokens.overlaySelectedSurface} 50%, transparent)` },
    transitionProperty: 'background-color, color', transitionDuration: { default: '400ms', ':active': '120ms' },
  },
  hint: { margin: 0, marginTop: 8, fontSize: 12, lineHeight: 1.6, color: `color-mix(in srgb, ${tokens.overlayText} 80%, transparent)` },
  footer: { display: 'flex', alignItems: 'flex-start', gap: 8, paddingTop: 14, borderTopWidth: 2, borderTopStyle: 'dashed', borderTopColor: `color-mix(in srgb, ${tokens.overlayText} 20%, transparent)`, color: `color-mix(in srgb, ${tokens.overlayText} 80%, transparent)` },
  githubIcon: { flexShrink: 0, marginTop: 1 },
  caption: { margin: 0, fontSize: 12, lineHeight: 1.6 },
  github: {
    borderRadius: 2, color: { default: 'inherit', ':hover': tokens.overlayHoverText, ':active': tokens.overlaySelectedText },
    textDecoration: { default: 'none', ':hover': 'underline' }, textUnderlineOffset: 3,
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: 3, outlineColor: tokens.overlayFocus,
    transitionProperty: 'color', transitionDuration: '300ms', transitionTimingFunction: tokens.overlayEasing,
  },
})
