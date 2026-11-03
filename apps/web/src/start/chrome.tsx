import { SunIcon } from '@phosphor-icons/react/dist/ssr/Sun'
import { MoonIcon } from '@phosphor-icons/react/dist/ssr/Moon'
import { GithubLogoIcon } from '@phosphor-icons/react/dist/ssr/GithubLogo'
import { GearSixIcon } from '@phosphor-icons/react/dist/ssr/GearSix'
import { UserCircleIcon } from '@phosphor-icons/react/dist/ssr/UserCircle'
import { SignInIcon } from '@phosphor-icons/react/dist/ssr/SignIn'
import { Menu } from '@base-ui/react/menu'
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
      <Menu.Root modal={false} highlightItemOnHover={false}>
        <Menu.Trigger aria-label={m.user} title={m.user} {...stylex.props(controls.glass, styles.action)}><UserCircleIcon size={21} aria-hidden="true" /></Menu.Trigger>
        <Menu.Portal>
          <Menu.Positioner align="end" sideOffset={8} {...stylex.props(styles.menuPositioner)}>
            <Menu.Popup aria-label={m.user} data-start-user-menu {...stylex.props(styles.menu)}>
              <Menu.Item disabled closeOnClick={false} aria-label={`${m.login} (${m.unavailable})`} title={m.unavailable} {...stylex.props(styles.menuItem)}>
                <SignInIcon size={18} aria-hidden="true" />{m.login}
              </Menu.Item>
              <Menu.Item disabled closeOnClick={false} aria-label={`${m.settings} (${m.unavailable})`} title={m.unavailable} {...stylex.props(styles.menuItem)}>
                <GearSixIcon size={18} aria-hidden="true" />{m.settings}
              </Menu.Item>
            </Menu.Popup>
          </Menu.Positioner>
        </Menu.Portal>
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
  menuPositioner: { zIndex: 3 },
  menu: {
    minWidth: 136, padding: 4, borderWidth: 0, borderRadius: tokens.radiusSmall,
    backgroundColor: tokens.overlaySurface, color: tokens.overlayText, backdropFilter: 'blur(14px) saturate(1.05)', boxShadow: 'none',
    outlineStyle: 'none',
    opacity: { default: 1, ':is([data-starting-style], [data-ending-style])': 0 },
    transform: { default: 'translateY(0)', ':is([data-starting-style], [data-ending-style])': 'translateY(-4px)' },
    pointerEvents: { default: 'auto', ':is([data-ending-style])': 'none' },
    transitionProperty: 'opacity, transform', transitionDuration: { default: '420ms', '@media (prefers-reduced-motion: reduce)': '0ms' }, transitionTimingFunction: tokens.overlayEasing,
  },
  menuItem: {
    display: 'flex', alignItems: 'center', gap: 8, minHeight: 36, paddingInline: 10, borderWidth: 0, borderRadius: 5,
    fontSize: 13, fontWeight: 500, lineHeight: 1.4, cursor: 'default', userSelect: 'none',
    backgroundColor: { default: 'transparent', ':hover:not(:focus-visible)': tokens.overlaySurfaceHover, ':active:not([data-disabled]):not(:focus-visible)': tokens.overlaySelectedSurface, ':focus-visible': tokens.overlaySelectedSurface },
    color: { default: tokens.overlayText, ':hover:not(:focus-visible)': tokens.overlayHoverText },
    opacity: { default: 1, ':is([data-disabled])': 0.7 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: -2, outlineColor: tokens.overlayFocus,
    transitionProperty: 'background-color, color', transitionDuration: tokens.overlayMotion, transitionTimingFunction: tokens.overlayEasing,
  },
  footer: {
    position: 'fixed', bottom: 'max(12px, env(safe-area-inset-bottom))',
    insetInline: { default: 24, '@media (max-width: 767px)': 12 },
    display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', alignItems: 'flex-end', gap: 6,
    pointerEvents: 'none', fontSize: { default: 11, '@media (max-width: 767px)': 10 }, fontWeight: 400, lineHeight: 1.5,
    color: tokens.filingText, textShadow: tokens.filingTextShadow,
  },
  text: { maxWidth: '100%', pointerEvents: 'auto' },
  filings: { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '2px 8px', marginInlineStart: 'auto' },
  footerLink: {
    display: 'inline-flex', alignItems: 'center', gap: 4, color: 'inherit',
    textDecoration: { default: 'none', ':hover': 'underline', ':active': 'underline' },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: 2, outlineColor: tokens.accent,
  },
})
