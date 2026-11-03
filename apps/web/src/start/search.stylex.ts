import * as stylex from '@stylexjs/stylex'

// Search keeps the migrated geometry with its own Light/Dark acrylic material.
export const search = stylex.defineVars({
  chipBackground: 'rgba(246, 248, 251, 0.29)', chipHoverBackground: 'rgba(248, 250, 252, 0.45)',
  chipText: 'rgba(38, 45, 55, 0.92)', chipHoverText: 'rgba(25, 31, 39, 0.98)',
  selectedBackground: 'rgba(250, 251, 253, 0.50)', selectedText: 'rgba(25, 31, 39, 0.98)',
  selectedHoverBackground: 'rgba(252, 253, 254, 0.63)', selectedHoverText: 'rgba(25, 31, 39, 0.98)',
  inputBackground: 'rgba(250, 251, 253, 0.40)',
  inputFocusBackground: 'rgba(250, 251, 253, 0.52)', inputText: 'rgba(30, 37, 46, 0.94)', inputPlaceholder: 'rgba(52, 61, 73, 0.72)',
  focusRing: 'rgba(39, 47, 58, 0.72)', motion: '420ms', easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
})

export const darkSearch = stylex.createTheme(search, {
  chipBackground: 'rgba(25, 29, 36, 0.30)', chipHoverBackground: 'rgba(32, 38, 47, 0.46)',
  chipText: 'rgba(224, 230, 239, 0.92)', chipHoverText: 'rgba(242, 245, 249, 0.98)',
  selectedBackground: 'rgba(172, 186, 206, 0.21)', selectedText: 'rgba(242, 245, 249, 0.98)',
  selectedHoverBackground: 'rgba(188, 201, 220, 0.34)', selectedHoverText: 'rgba(242, 245, 249, 0.98)',
  inputBackground: 'rgba(25, 30, 38, 0.36)',
  inputFocusBackground: 'rgba(30, 36, 46, 0.49)', inputText: 'rgba(233, 238, 245, 0.96)', inputPlaceholder: 'rgba(209, 218, 231, 0.76)',
  focusRing: 'rgba(213, 223, 236, 0.78)',
})

// CSS resolves system colors before hydration, without guessing the client OS.
export const systemSearch = stylex.createTheme(search, {
  chipBackground: { default: 'rgba(246, 248, 251, 0.29)', '@media (prefers-color-scheme: dark)': 'rgba(25, 29, 36, 0.30)' },
  chipHoverBackground: { default: 'rgba(248, 250, 252, 0.45)', '@media (prefers-color-scheme: dark)': 'rgba(32, 38, 47, 0.46)' },
  chipText: { default: 'rgba(38, 45, 55, 0.92)', '@media (prefers-color-scheme: dark)': 'rgba(224, 230, 239, 0.92)' },
  chipHoverText: { default: 'rgba(25, 31, 39, 0.98)', '@media (prefers-color-scheme: dark)': 'rgba(242, 245, 249, 0.98)' },
  selectedBackground: { default: 'rgba(250, 251, 253, 0.50)', '@media (prefers-color-scheme: dark)': 'rgba(172, 186, 206, 0.21)' },
  selectedText: { default: 'rgba(25, 31, 39, 0.98)', '@media (prefers-color-scheme: dark)': 'rgba(242, 245, 249, 0.98)' },
  selectedHoverBackground: { default: 'rgba(252, 253, 254, 0.63)', '@media (prefers-color-scheme: dark)': 'rgba(188, 201, 220, 0.34)' },
  selectedHoverText: { default: 'rgba(25, 31, 39, 0.98)', '@media (prefers-color-scheme: dark)': 'rgba(242, 245, 249, 0.98)' },
  inputBackground: { default: 'rgba(250, 251, 253, 0.40)', '@media (prefers-color-scheme: dark)': 'rgba(25, 30, 38, 0.36)' },
  inputFocusBackground: { default: 'rgba(250, 251, 253, 0.52)', '@media (prefers-color-scheme: dark)': 'rgba(30, 36, 46, 0.49)' },
  inputText: { default: 'rgba(30, 37, 46, 0.94)', '@media (prefers-color-scheme: dark)': 'rgba(233, 238, 245, 0.96)' },
  inputPlaceholder: { default: 'rgba(52, 61, 73, 0.72)', '@media (prefers-color-scheme: dark)': 'rgba(209, 218, 231, 0.76)' },
  focusRing: { default: 'rgba(39, 47, 58, 0.72)', '@media (prefers-color-scheme: dark)': 'rgba(213, 223, 236, 0.78)' },
})

export const searchStyles = stylex.create({
  shell: { position: 'relative', isolation: 'isolate', display: 'flex', flexDirection: 'column', alignItems: 'center', width: '100%', maxWidth: 620, marginInline: 'auto' },
  categoryViewport: { position: 'relative', width: '100%', maxWidth: 500, marginBottom: { default: 12, '@media (max-width: 639px)': 8 } },
  categoryTrack: {
    width: '100%', maxWidth: 500, overflowX: 'auto', paddingBlock: 10,
    scrollbarWidth: 'none', '::-webkit-scrollbar': { display: 'none' }, overscrollBehaviorX: 'contain',
  },
  fadeLeft: { maskImage: 'linear-gradient(to right, transparent, #000 20px)' },
  fadeRight: { maskImage: 'linear-gradient(to left, transparent, #000 20px)' },
  fadeBoth: { maskImage: 'linear-gradient(to right, transparent, #000 20px, #000 calc(100% - 20px), transparent)' },
  edge: { position: 'absolute', top: '50%', transform: 'translateY(-50%)', display: 'grid', placeItems: 'center', width: 16, height: 24, borderRadius: 6, pointerEvents: 'none', backgroundColor: search.chipBackground, color: search.chipText },
  edgeLeft: { left: 0 }, edgeRight: { right: 0 },
  categories: { display: 'flex', width: 'max-content', gap: { default: 12, '@media (max-width: 767px)': 8, '@media (max-width: 400px)': 6 }, marginInline: 'auto' },
  chip: {
    borderWidth: 0, borderRadius: 12, fontFamily: 'inherit', fontWeight: 500, whiteSpace: 'nowrap',
    backgroundColor: { default: search.chipBackground, ':enabled:hover:not(:active)': search.chipHoverBackground, ':enabled:active': search.selectedBackground },
    color: { default: search.chipText, ':enabled:hover:not(:active)': search.chipHoverText, ':enabled:active': search.selectedText },
    boxShadow: 'none',
    backdropFilter: 'blur(14px) saturate(1.12)', cursor: { default: 'pointer', ':disabled': 'not-allowed' }, opacity: { default: 1, ':disabled': 0.5 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineColor: { default: search.chipText, ':active': search.selectedText }, outlineWidth: 2, outlineOffset: -2,
    transitionProperty: 'background-color, color', transitionDuration: { default: search.motion, ':active': '120ms' }, transitionTimingFunction: search.easing,
  },
  selected: {
    backgroundColor: { default: search.selectedBackground, ':enabled:hover:not(:active)': search.selectedHoverBackground, ':enabled:active': search.chipHoverBackground },
    color: { default: search.selectedText, ':enabled:hover:not(:active)': search.selectedHoverText, ':enabled:active': search.chipHoverText },
    outlineColor: { default: search.selectedText, ':active': search.chipText },
  },
  category: { flexShrink: 0, paddingBlock: 6, paddingInline: { default: 12, '@media (max-width: 400px)': 8 }, fontSize: { default: 14, '@media (max-width: 400px)': 12 }, lineHeight: 1.42857 },
  // The popup is a sibling of the form, outside its backdrop-filter root.
  anchor: { position: 'relative', zIndex: 1, width: '100%', maxWidth: 500 },
  search: {
    position: 'relative', zIndex: 1, display: 'flex', alignItems: 'center', width: '100%', maxWidth: 500, height: 48,
    borderWidth: 0, borderRadius: 12, backgroundColor: { default: search.inputBackground, ':has(input:enabled):active': search.inputFocusBackground, ':has(input:focus)': search.inputFocusBackground },
    color: search.inputText, boxShadow: 'none',
    backdropFilter: { default: 'blur(18px) saturate(1.08)', '@media (max-width: 639px)': 'blur(12px) saturate(1.04)' },
    outlineStyle: 'none', opacity: { default: 1, ':has(input:disabled)': 0.5 },
    transitionProperty: 'background-color, box-shadow, color', transitionDuration: search.motion, transitionTimingFunction: search.easing,
  },
  // One keyboard-only ring; pointer focus only brightens the acrylic surface.
  keyboardFocus: { ':has(input:focus-visible)': { boxShadow: `0 0 0 2px ${search.focusRing}` } },
  input: {
    flex: 1, width: 0, minWidth: 0, height: 48, paddingBlock: 0, paddingInlineStart: { default: 16, '@media (max-width: 767px)': 10 }, paddingInlineEnd: 40,
    borderWidth: 0, borderRadius: 12, outlineStyle: 'none', backgroundColor: 'transparent', color: search.inputText,
    cursor: { default: 'text', ':disabled': 'not-allowed' },
    fontFamily: 'inherit', fontWeight: 400, fontSize: 16, lineHeight: '24px', '::placeholder': { color: search.inputPlaceholder, opacity: 1 },
    '::-webkit-search-cancel-button': { appearance: 'none', display: 'none' },
  },
  inputWithClear: { paddingInlineEnd: 72 },
  clear: { right: 42 },
  submit: {
    position: 'absolute', top: '50%', right: 12, transform: 'translateY(-50%)',
    display: 'grid', placeItems: 'center', width: 20, height: 20, padding: 0, borderWidth: 0, borderRadius: 2,
    backgroundColor: 'transparent', color: search.inputText,
    cursor: { default: 'pointer', ':disabled': 'not-allowed' }, opacity: { default: 0.72, ':enabled:hover': 0.96, ':enabled:active': 1, ':disabled': 0.4 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: 3, outlineColor: search.inputText,
    transitionProperty: 'opacity', transitionDuration: { default: search.motion, ':active': '120ms' }, transitionTimingFunction: search.easing,
  },
  desktopPlatforms: {
    display: { default: 'flex', '@media (max-width: 767px)': 'none' }, flexWrap: 'wrap', justifyContent: 'center', gap: 8,
    width: '100%', maxWidth: 620, paddingInline: 12, paddingBlock: 10, marginTop: { default: 4, '@media (max-width: 639px)': 8 },
  },
  platform: { paddingBlock: 6, paddingInline: 10, fontSize: 12, lineHeight: 1.33333 },
  mobileTrigger: {
    display: { default: 'none', '@media (max-width: 767px)': 'flex' }, alignItems: 'center', justifyContent: 'center', gap: 4,
    flexShrink: 0, minWidth: 0, maxWidth: '36%', height: 40, marginInlineStart: 4, paddingInline: 10,
    borderWidth: 0, borderRadius: 8, fontFamily: 'inherit', fontSize: 12, fontWeight: 500, lineHeight: 1.33333, whiteSpace: 'nowrap',
    backgroundColor: { default: 'transparent', ':enabled:hover:not(:active)': search.chipHoverBackground, ':enabled:active': search.selectedBackground },
    color: search.inputText, cursor: { default: 'pointer', ':disabled': 'default' },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: -2, outlineColor: search.focusRing,
    transitionProperty: 'background-color, color', transitionDuration: '300ms', transitionTimingFunction: search.easing,
  },
  caret: { flexShrink: 0 },
  panel: {
    position: 'absolute', top: 'calc(100% + 8px)', insetInline: 0, zIndex: 2, borderRadius: 12,
    padding: 4, borderWidth: 0, boxShadow: 'none', backgroundColor: search.inputBackground, color: search.inputText,
    backdropFilter: 'blur(18px) saturate(1.08)',
    maxHeight: 'max(48px, min(320px, calc(100dvh - 260px)))', overflowY: 'auto', overscrollBehavior: 'contain',
    opacity: { default: 0, ':is([data-open="true"])': 1 },
    transform: { default: 'translateY(-4px)', ':is([data-open="true"])': 'translateY(0)' },
    pointerEvents: { default: 'none', ':is([data-open="true"])': 'auto' },
    transitionProperty: 'opacity, transform', transitionDuration: { default: '420ms', '@media (prefers-reduced-motion: reduce)': '0ms' }, transitionTimingFunction: search.easing,
  },
  option: {
    display: 'flex', alignItems: 'center', width: '100%', minHeight: 36, paddingBlock: 8, paddingInline: 12,
    borderWidth: 0, borderRadius: 8, fontFamily: 'inherit', fontSize: 14, fontWeight: 500, lineHeight: 1.42857, textAlign: 'start',
    backgroundColor: { default: search.chipBackground, ':enabled:hover:not(:active)': search.chipHoverBackground, ':enabled:active': search.selectedBackground },
    color: { default: search.chipText, ':enabled:hover:not(:active)': search.chipHoverText, ':enabled:active': search.selectedText },
    cursor: 'pointer', whiteSpace: 'normal', overflowWrap: 'anywhere',
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: -2, outlineColor: { default: search.chipText, ':active': search.selectedText },
    transitionProperty: 'background-color, color', transitionDuration: { default: search.motion, ':active': '120ms' }, transitionTimingFunction: search.easing,
  },
  optionBase: { backgroundColor: 'transparent' },
  highlight: { backgroundColor: 'transparent', color: 'inherit', fontWeight: 800 },
  truncate: { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' },
})
