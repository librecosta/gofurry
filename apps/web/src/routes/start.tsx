import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import * as stylex from '@stylexjs/stylex'
import logo from '../assets/logo-mini.svg'
import { useMessages, usePreferences } from '../preferences'
import { tokens } from '../styles/tokens.stylex'
import { StartBackground } from '../start-background'

export const Route = createFileRoute('/start')({
  head: () => ({ meta: [{ title: 'GoFurry · Start' }] }),
  component: Start,
})

function Start() {
  const { region } = usePreferences()
  const m = useMessages()
  // Text inputs can match :focus-visible after a pointer click too. Keep the
  // native CSS fallback, but suppress the keyboard frame for that focus session.
  const [pointerFocus, setPointerFocus] = useState(false)
  return <>
    <StartBackground />
    <header {...stylex.props(styles.brand)}>
      <img src={logo} width={40} height={32} alt="" {...stylex.props(styles.logo)} />
      <span>GoFurry</span>
    </header>
    <main {...stylex.props(styles.page)}>
      <form role="search" aria-label={m.search} method="get" acceptCharset="UTF-8"
        action={region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'}
        {...stylex.props(styles.search, !pointerFocus && styles.keyboardFocus)}>
        <MagnifyingGlassIcon size={22} aria-hidden="true" focusable="false" />
        <input type="search" name="q" aria-label={m.search} placeholder={m.search}
          required pattern={String.raw`.*\S.*`} autoComplete="off" autoCapitalize="off" spellCheck={false}
          onPointerDown={() => setPointerFocus(true)} onBlur={() => setPointerFocus(false)}
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
    borderWidth: 1, borderStyle: 'solid', borderRadius: tokens.radiusLarge,
    borderColor: { default: tokens.border, ':has(:enabled):active': tokens.accent },
    backgroundColor: { default: tokens.surface, ':has(:enabled):hover': tokens.surfaceHover },
    color: tokens.textMuted, outlineStyle: 'none',
    opacity: { default: 1, ':has(:disabled)': 0.5 },
    cursor: { default: 'text', ':has(:disabled)': 'not-allowed' },
  },
  keyboardFocus: {
    ':has(:focus-visible)': {
      // Replace the visible border in place; never add a second, offset frame.
      borderColor: 'transparent', outlineStyle: 'solid', outlineWidth: 2,
      outlineOffset: -2, outlineColor: tokens.accent,
    },
  },
  input: {
    flexGrow: 1, minWidth: 0, width: '100%', padding: 0, borderWidth: 0,
    outlineStyle: 'none', backgroundColor: 'transparent', color: tokens.text,
    cursor: { default: 'text', ':disabled': 'not-allowed' },
    fontFamily: 'inherit', fontSize: { default: 18, '@media (max-width: 767px)': 16 }, lineHeight: '24px',
    '::placeholder': { color: tokens.text, opacity: 0.7 },
  },
})
