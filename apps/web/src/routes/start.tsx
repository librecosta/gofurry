import { createFileRoute } from '@tanstack/react-router'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import * as stylex from '@stylexjs/stylex'
import logo from '../assets/logo-mini.svg'
import { useMessages, usePreferences } from '../preferences'
import { tokens } from '../styles/tokens.stylex'

export const Route = createFileRoute('/start')({
  head: () => ({ meta: [{ title: 'GoFurry · Start' }] }),
  component: Start,
})

function Start() {
  const { region } = usePreferences()
  const m = useMessages()
  return <>
    <header {...stylex.props(styles.brand)}>
      <img src={logo} width={40} height={32} alt="" {...stylex.props(styles.logo)} />
      <span>GoFurry</span>
    </header>
    <main {...stylex.props(styles.page)}>
      <form role="search" aria-label={m.search} method="get" acceptCharset="UTF-8"
        action={region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'}
        {...stylex.props(styles.search)}>
        <MagnifyingGlassIcon size={22} aria-hidden="true" focusable="false" />
        <input type="search" name="q" aria-label={m.search} placeholder={m.search}
          required pattern={String.raw`.*\S.*`} autoComplete="off" autoCapitalize="off" spellCheck={false}
          enterKeyHint="search" {...stylex.props(styles.input)} />
      </form>
    </main>
  </>
}

const styles = stylex.create({
  brand: {
    position: 'absolute', top: { default: 28, '@media (max-width: 767px)': 20 },
    insetInlineStart: { default: 32, '@media (max-width: 767px)': 20 },
    display: 'flex', alignItems: 'center', gap: 10,
    fontSize: 20, fontWeight: 700, lineHeight: 1.2, letterSpacing: '-0.5px',
  },
  logo: { objectFit: 'contain' },
  page: {
    minHeight: '100dvh', display: 'grid', placeItems: 'center',
    paddingBlock: 96, paddingInline: { default: 32, '@media (max-width: 767px)': 20 },
  },
  search: {
    display: 'flex', alignItems: 'center', gap: 12, width: '100%', maxWidth: 640,
    minHeight: { default: 60, '@media (max-width: 767px)': 56 },
    paddingInline: 20, paddingBlock: { default: 16, '@media (max-width: 767px)': 14 },
    borderWidth: 1, borderStyle: 'solid', borderColor: tokens.border, borderRadius: tokens.radiusLarge,
    backgroundColor: tokens.surface, color: tokens.textMuted,
    outlineStyle: 'solid', outlineWidth: 2, outlineOffset: 4,
    outlineColor: { default: 'transparent', ':focus-within': tokens.accent },
  },
  input: {
    flexGrow: 1, minWidth: 0, width: '100%', padding: 0, borderWidth: 0,
    outlineStyle: 'none', backgroundColor: 'transparent', color: tokens.text,
    fontFamily: 'inherit', fontSize: { default: 18, '@media (max-width: 767px)': 16 }, lineHeight: '24px',
    '::placeholder': { color: tokens.text, opacity: 0.7 },
  },
})
