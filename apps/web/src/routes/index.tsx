import { createFileRoute, Link } from '@tanstack/react-router'
import { Popover } from '@base-ui/react/popover'
import * as stylex from '@stylexjs/stylex'
import { tokens } from '../styles/tokens.stylex'
import { useMessages, usePreferences } from '../preferences'

export const Route = createFileRoute('/')({ component: RuntimeCheck })

function RuntimeCheck() {
  const { region, theme, locale, setTheme, setLocale } = usePreferences()
  const m = useMessages()
  return (
    <main {...stylex.props(styles.page)}>
      <h1>GoFurry runtime skeleton</h1>
      <p>V0.1-A · SSR / StyleX / Base UI</p>
      <p><code {...stylex.props(styles.code)}>{region} / SFW</code></p>
      <label>{m.theme} <select aria-label="Theme" value={theme} onChange={(e) => setTheme(e.target.value as 'light' | 'dark')}>
        <option value="light">Light</option><option value="dark">Dark</option>
      </select></label>{' '}
      <label>{m.language} <select aria-label="Language" value={locale} onChange={(e) => setLocale(e.target.value as 'zh' | 'en')}>
        <option value="zh">中文</option><option value="en">English</option>
      </select></label>
      <hr />
      <Popover.Root>
        <Popover.Trigger {...stylex.props(styles.button)}>{m.interact}</Popover.Trigger>
        <Popover.Portal>
          <Popover.Positioner sideOffset={8}>
            <Popover.Popup {...stylex.props(styles.popup)}>
              <Popover.Title>{m.title}</Popover.Title>
              <Popover.Description>{m.description}</Popover.Description>
              <Popover.Close {...stylex.props(styles.button)}>{m.close}</Popover.Close>
            </Popover.Popup>
          </Popover.Positioner>
        </Popover.Portal>
      </Popover.Root>
      {' '}
      <Popover.Root>
        <Popover.Trigger disabled {...stylex.props(styles.button)}>{m.disabled}</Popover.Trigger>
      </Popover.Root>
      <p><Link to="/status">{m.status}</Link></p>
    </main>
  )
}

const styles = stylex.create({
  code: { fontFamily: tokens.fontMono, color: tokens.textMuted },
  page: { minHeight: '100vh', padding: 32, color: tokens.text },
  button: {
    paddingBlock: 8, paddingInline: 12, borderRadius: tokens.radiusSmall,
    borderWidth: 1, borderStyle: 'solid', borderColor: tokens.border,
    backgroundColor: tokens.surface, color: tokens.text,
    opacity: { default: 1, ':disabled': 0.5 },
    outlineOffset: 3,
    outlineColor: { default: 'transparent', ':focus-visible': tokens.focusRing },
    transitionDuration: tokens.motionFast, transitionProperty: 'background-color',
    ':hover': { backgroundColor: tokens.surfaceHover },
  },
  popup: {
    transitionDuration: tokens.motionBase, transitionProperty: 'opacity',
    padding: 20, maxWidth: 320, borderRadius: tokens.radiusLarge,
    borderWidth: 1, borderStyle: 'solid', borderColor: tokens.border,
    backgroundColor: tokens.surface, color: tokens.text,
  },
})
