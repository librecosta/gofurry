import { SunIcon } from '@phosphor-icons/react/dist/ssr/Sun'
import { MoonIcon } from '@phosphor-icons/react/dist/ssr/Moon'
import { GithubLogoIcon } from '@phosphor-icons/react/dist/ssr/GithubLogo'
import { GearSixIcon } from '@phosphor-icons/react/dist/ssr/GearSix'
import { UserCircleIcon } from '@phosphor-icons/react/dist/ssr/UserCircle'
import * as stylex from '@stylexjs/stylex'
import { usePreferences } from '../preferences'
import { tokens } from '../styles/tokens.stylex'
import logo from '../assets/logo-mini.svg'
import { controls } from './controls.stylex'
import { cnFiling } from './filing.config'
import { useStartMessages } from './messages'
import { useThemeReveal } from './theme-reveal'

export function StartHeader() {
  const { locale, theme, setLocale } = usePreferences()
  const revealTheme = useThemeReveal()
  const m = useStartMessages()
  const themeLabel = theme === 'light' ? m.themeDark : m.themeLight
  return <header {...stylex.props(styles.header)}>
    <span role="img" aria-label="GoFurry" data-start-logo {...stylex.props(styles.logo(logo))} />
    <div data-start-actions {...stylex.props(styles.actions)}>
      <button type="button" aria-label={m.language} title={m.language}
        onClick={() => setLocale(locale === 'zh' ? 'en' : 'zh')} {...stylex.props(controls.glass, styles.action)}>{locale === 'zh' ? '中' : 'EN'}</button>
      <button type="button" aria-label={themeLabel} title={themeLabel}
        onClick={(event) => revealTheme(event.currentTarget)} {...stylex.props(controls.glass, styles.action)}>
        {theme === 'light' ? <SunIcon size={21} aria-hidden="true" data-theme-icon="sun" /> : <MoonIcon size={21} aria-hidden="true" data-theme-icon="moon" />}
      </button>
      <a href="https://github.com/gofurry" target="_blank" rel="noopener noreferrer" aria-label={m.github} title={m.github}
        {...stylex.props(controls.glass, styles.action, styles.link)}><GithubLogoIcon size={21} aria-hidden="true" /></a>
      <button type="button" disabled aria-label={m.settings} title={m.settings} {...stylex.props(controls.glass, styles.action)}><GearSixIcon size={21} aria-hidden="true" /></button>
      <button type="button" disabled aria-label={m.user} title={m.user} {...stylex.props(controls.glass, styles.action)}><UserCircleIcon size={21} aria-hidden="true" /></button>
    </div>
  </header>
}

export function StartFooter() {
  const { region } = usePreferences()
  const m = useStartMessages()
  if (region !== 'CN') return null
  const hasICP = !!cnFiling.icp || cnFiling.showPending
  const hasSecurity = !!cnFiling.publicSecurity || cnFiling.showPending
  return <footer data-start-filing {...stylex.props(styles.footer)}>
    <span {...stylex.props(styles.text)}>{m.company}</span>
    <div {...stylex.props(styles.text, styles.filings)}>
      {cnFiling.icp ? <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer" {...stylex.props(styles.footerLink)}>{cnFiling.icp.number}</a>
        : cnFiling.showPending && <span>{m.icpPending}</span>}
      {hasICP && hasSecurity && <span aria-hidden="true">·</span>}
      {cnFiling.publicSecurity ? <a href={cnFiling.publicSecurity.queryUrl} target="_blank" rel="noopener noreferrer" {...stylex.props(styles.footerLink)}>
        <img src={cnFiling.publicSecurity.icon} alt={m.securityIcon} width={14} height={14} />{cnFiling.publicSecurity.number}
      </a> : cnFiling.showPending && <span>{m.securityPending}</span>}
    </div>
  </footer>
}

const styles = stylex.create({
  header: {
    position: 'absolute', top: { default: 24, '@media (max-width: 767px)': 16 },
    insetInline: { default: 32, '@media (max-width: 767px)': 16 }, zIndex: 2,
    display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12,
  },
  // An alpha mask paints the unchanged SVG paths with the inherited brand role.
  logo: (url: string) => ({
    display: 'block', flexShrink: 0, width: { default: 40, '@media (max-width: 767px)': 36 }, aspectRatio: '1287 / 1024',
    backgroundColor: tokens.logoBrand, maskImage: `url("${url}")`, maskSize: 'contain', maskPosition: 'center', maskRepeat: 'no-repeat',
  }),
  actions: { display: 'flex', gap: { default: 8, '@media (max-width: 767px)': 6 } },
  action: {
    display: 'grid', placeItems: 'center', padding: 0, flexShrink: 0,
    width: { default: 40, '@media (max-width: 767px)': 36 }, height: { default: 40, '@media (max-width: 767px)': 36 },
    fontSize: 13, fontWeight: 600, lineHeight: 1, textAlign: 'center', letterSpacing: 'normal',
  },
  link: { textDecoration: 'none' },
  footer: {
    position: 'fixed', bottom: 'max(12px, env(safe-area-inset-bottom))',
    insetInline: { default: 24, '@media (max-width: 767px)': 12 },
    display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', alignItems: 'flex-end', gap: 6,
    pointerEvents: 'none', fontSize: { default: 11, '@media (max-width: 767px)': 10 }, fontWeight: 400, lineHeight: 1.5,
    color: tokens.overlayText, opacity: 0.7, textShadow: '0 1px 2px rgba(0, 0, 0, 0.35)',
  },
  text: { maxWidth: '100%', pointerEvents: 'auto' },
  filings: { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '2px 8px', marginInlineStart: 'auto' },
  footerLink: {
    display: 'inline-flex', alignItems: 'center', gap: 4, color: 'inherit',
    textDecoration: { default: 'none', ':hover': 'underline', ':active': 'underline' },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: 2, outlineColor: tokens.accent,
  },
})
