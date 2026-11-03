import * as stylex from '@stylexjs/stylex'

// Search keeps the migrated geometry with its own Light/Dark acrylic material.
export const search = stylex.defineVars({
  chipBackground: 'rgba(246, 248, 251, 0.46)', chipHoverBackground: 'rgba(248, 250, 252, 0.64)',
  chipText: 'rgba(38, 45, 55, 0.92)', chipHoverText: 'rgba(25, 31, 39, 0.98)',
  selectedBackground: 'rgba(250, 251, 253, 0.78)', selectedText: 'rgba(25, 31, 39, 0.98)',
  selectedHoverBackground: 'rgba(252, 253, 254, 0.88)', selectedHoverText: 'rgba(25, 31, 39, 0.98)',
  inputBackground: 'rgba(250, 251, 253, 0.62)',
  inputFocusBackground: 'rgba(250, 251, 253, 0.76)', inputText: 'rgba(30, 37, 46, 0.94)', inputPlaceholder: 'rgba(52, 61, 73, 0.72)',
  focusRing: 'rgba(39, 47, 58, 0.72)', motion: '300ms', easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
})

export const darkSearch = stylex.createTheme(search, {
  chipBackground: 'rgba(25, 29, 36, 0.48)', chipHoverBackground: 'rgba(32, 38, 47, 0.66)',
  chipText: 'rgba(224, 230, 239, 0.92)', chipHoverText: 'rgba(242, 245, 249, 0.98)',
  selectedBackground: 'rgba(172, 186, 206, 0.32)', selectedText: 'rgba(242, 245, 249, 0.98)',
  selectedHoverBackground: 'rgba(188, 201, 220, 0.44)', selectedHoverText: 'rgba(242, 245, 249, 0.98)',
  inputBackground: 'rgba(25, 30, 38, 0.55)',
  inputFocusBackground: 'rgba(30, 36, 46, 0.70)', inputText: 'rgba(233, 238, 245, 0.96)', inputPlaceholder: 'rgba(209, 218, 231, 0.76)',
  focusRing: 'rgba(213, 223, 236, 0.78)',
})

export const searchStyles = stylex.create({
  shell: { position: 'relative', isolation: 'isolate', display: 'flex', flexDirection: 'column', alignItems: 'center', width: '100%', maxWidth: 620, marginInline: 'auto' },
  categoryTrack: {
    width: '100%', maxWidth: 500, overflowX: 'auto', paddingBlock: 10,
    marginBottom: { default: 12, '@media (max-width: 639px)': 8 }, scrollbarWidth: 'none', '::-webkit-scrollbar': { display: 'none' },
  },
  categories: { display: 'flex', width: 'max-content', gap: { default: 12, '@media (max-width: 767px)': 8 }, marginInline: 'auto' },
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
  category: { flexShrink: 0, paddingBlock: 6, paddingInline: 12, fontSize: 14, lineHeight: 1.42857 },
  search: {
    position: 'relative', zIndex: 1, width: '100%', maxWidth: 500, height: 48,
    borderWidth: 0, borderRadius: 12, backgroundColor: { default: search.inputBackground, ':has(input:enabled):active': search.inputFocusBackground, ':has(input:focus)': search.inputFocusBackground },
    color: search.inputText, boxShadow: 'none',
    backdropFilter: { default: 'blur(18px) saturate(1.08)', '@media (max-width: 639px)': 'blur(12px) saturate(1.04)' },
    outlineStyle: 'none', opacity: { default: 1, ':has(input:disabled)': 0.5 },
    transitionProperty: 'background-color, box-shadow, color', transitionDuration: search.motion, transitionTimingFunction: search.easing,
  },
  // One keyboard-only ring; pointer focus only brightens the acrylic surface.
  keyboardFocus: { ':has(input:focus-visible)': { boxShadow: `0 0 0 2px ${search.focusRing}` } },
  input: {
    width: '100%', minWidth: 0, height: 48, paddingBlock: 0, paddingInlineStart: 16, paddingInlineEnd: 40,
    borderWidth: 0, borderRadius: 12, outlineStyle: 'none', backgroundColor: 'transparent', color: search.inputText,
    cursor: { default: 'text', ':disabled': 'not-allowed' },
    fontFamily: 'inherit', fontWeight: 400, fontSize: 16, lineHeight: '24px', '::placeholder': { color: search.inputPlaceholder, opacity: 1 },
  },
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
  mobilePlatforms: { position: 'relative', display: { default: 'none', '@media (max-width: 767px)': 'block' }, width: '100%', maxWidth: 500, height: 44, marginTop: 8, pointerEvents: 'none' },
  grid: (columns: number) => ({ position: 'absolute', right: 0, top: 0, display: 'grid', width: '100%', gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gap: 6 }),
  slot: (column: number, row: number) => ({ gridColumn: column, gridRow: row }),
  mobileOption: {
    display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 4, minWidth: 0, minHeight: 44, paddingBlock: 0, paddingInline: 8,
    borderWidth: 0, borderRadius: 6, fontFamily: 'inherit', fontSize: 12, fontWeight: 400, lineHeight: 1.33333, whiteSpace: 'nowrap',
    backgroundColor: { default: search.chipBackground, ':enabled:hover:not(:active)': search.chipHoverBackground, ':enabled:active': search.selectedBackground },
    color: { default: search.chipText, ':enabled:hover:not(:active)': search.chipHoverText, ':enabled:active': search.selectedText },
    backdropFilter: 'blur(10px)', pointerEvents: 'auto', cursor: { default: 'pointer', ':disabled': 'not-allowed' }, opacity: { default: 1, ':disabled': 0.5 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: -2, outlineColor: { default: search.chipText, ':active': search.selectedText },
    transitionProperty: 'background-color, color', transitionDuration: { default: search.motion, ':active': '120ms' }, transitionTimingFunction: search.easing,
  },
  truncate: { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' },
})
