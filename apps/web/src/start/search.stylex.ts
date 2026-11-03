import * as stylex from '@stylexjs/stylex'

// Only the consumed SearchBox/nav.less roles. The legacy Hero search material is
// deliberately identical in Light/Dark; it is independent of toolbar glass.
export const search = stylex.defineVars({
  chipBackground: 'rgba(15, 23, 42, 0.50)', chipHoverBackground: 'rgba(15, 23, 42, 0.68)',
  chipText: 'rgba(248, 250, 252, 0.92)', chipHoverText: 'rgba(255, 255, 255, 0.98)',
  selectedBackground: 'rgba(255, 248, 241, 0.92)', selectedText: 'rgba(15, 23, 42, 0.94)',
  selectedHoverBackground: 'rgba(255, 255, 255, 0.96)', selectedHoverText: 'rgba(15, 23, 42, 0.96)',
  chipShadow: { default: 'inset 0 1px 0 rgba(255, 255, 255, 0.12), 0 8px 24px rgba(2, 6, 23, 0.14)', '@media (max-width: 639px)': 'inset 0 1px 0 rgba(255, 255, 255, 0.10)' },
  chipHoverShadow: { default: 'inset 0 1px 0 rgba(255, 255, 255, 0.16), 0 10px 28px rgba(2, 6, 23, 0.18)', '@media (max-width: 639px)': 'inset 0 1px 0 rgba(255, 255, 255, 0.10)' },
  selectedShadow: { default: 'inset 0 1px 0 rgba(255, 255, 255, 0.65), 0 12px 30px rgba(2, 6, 23, 0.20)', '@media (max-width: 639px)': 'inset 0 1px 0 rgba(255, 255, 255, 0.52)' },
  inputBackground: { default: 'rgba(255, 255, 255, 0.78)', '@media (max-width: 639px)': 'rgba(255, 255, 255, 0.82)' },
  inputFocusBackground: 'rgba(255, 255, 255, 0.90)', inputText: 'rgba(15, 23, 42, 0.94)', inputPlaceholder: 'rgba(71, 85, 105, 0.72)',
  inputShadow: { default: 'inset 0 1px 0 rgba(255, 255, 255, 0.86), 0 12px 36px rgba(2, 6, 23, 0.22), 0 0 0 1px rgba(255, 255, 255, 0.18)', '@media (max-width: 639px)': 'inset 0 1px 0 rgba(255, 255, 255, 0.82), 0 0 0 1px rgba(255, 255, 255, 0.28)' },
  inputFocusShadow: { default: 'inset 0 1px 0 rgba(255, 255, 255, 0.90), 0 16px 42px rgba(2, 6, 23, 0.26), 0 0 0 1px rgba(255, 255, 255, 0.34), 0 0 0 4px rgba(15, 23, 42, 0.20)', '@media (max-width: 639px)': 'inset 0 1px 0 rgba(255, 255, 255, 0.88), 0 0 0 1px rgba(255, 255, 255, 0.38), 0 0 0 3px rgba(15, 23, 42, 0.12)' },
  iconFilter: { default: 'drop-shadow(0 1px 1px rgba(255, 255, 255, 0.42))', '@media (max-width: 639px)': 'none' },
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
    boxShadow: { default: search.chipShadow, ':enabled:hover:not(:active)': search.chipHoverShadow, ':enabled:active': search.selectedShadow },
    backdropFilter: 'blur(14px) saturate(1.12)', cursor: { default: 'pointer', ':disabled': 'not-allowed' }, opacity: { default: 1, ':disabled': 0.5 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineColor: { default: search.chipText, ':active': search.selectedText }, outlineWidth: 2, outlineOffset: -2,
    transitionProperty: 'background-color, box-shadow, color', transitionDuration: '500ms', transitionTimingFunction: 'ease',
  },
  selected: {
    backgroundColor: { default: search.selectedBackground, ':enabled:hover:not(:active)': search.selectedHoverBackground, ':enabled:active': search.chipHoverBackground },
    color: { default: search.selectedText, ':enabled:hover:not(:active)': search.selectedHoverText, ':enabled:active': search.chipHoverText },
    boxShadow: { default: search.selectedShadow, ':enabled:hover:not(:active)': search.selectedShadow, ':enabled:active': search.chipShadow },
    outlineColor: { default: search.selectedText, ':active': search.chipText },
  },
  category: { flexShrink: 0, paddingBlock: 6, paddingInline: 12, fontSize: 14, lineHeight: 1.42857 },
  search: {
    position: 'relative', zIndex: 1, width: '100%', maxWidth: 500, height: 48,
    borderWidth: 0, borderRadius: 12, backgroundColor: { default: search.inputBackground, ':has(input:enabled):active': search.inputFocusBackground, ':has(input:focus)': search.inputFocusBackground },
    color: search.inputText, boxShadow: search.inputShadow,
    backdropFilter: { default: 'blur(18px) saturate(1.08)', '@media (max-width: 639px)': 'blur(12px) saturate(1.04)' },
    outlineStyle: 'none', opacity: { default: 1, ':has(input:disabled)': 0.5 },
    transitionProperty: 'background-color, box-shadow, color', transitionDuration: '500ms', transitionTimingFunction: 'ease',
  },
  // Preserve the legacy soft shadow/halo only for keyboard focus. Pointer focus
  // gets the brighter glass without layering a native outline over that halo.
  keyboardFocus: { ':has(input:focus-visible)': { boxShadow: search.inputFocusShadow } },
  input: {
    width: '100%', minWidth: 0, height: 48, paddingBlock: 0, paddingInlineStart: 16, paddingInlineEnd: 40,
    borderWidth: 0, borderRadius: 12, outlineStyle: 'none', backgroundColor: 'transparent', color: search.inputText,
    cursor: { default: 'text', ':disabled': 'not-allowed' },
    fontFamily: 'inherit', fontWeight: 400, fontSize: 16, lineHeight: '24px', '::placeholder': { color: search.inputPlaceholder, opacity: 1 },
  },
  submit: {
    position: 'absolute', top: '50%', right: 12, transform: { default: 'translateY(-50%)', ':enabled:hover': 'translateY(-50%) scale(1.1)', ':enabled:active': 'translateY(-50%)' },
    display: 'grid', placeItems: 'center', width: 20, height: 20, padding: 0, borderWidth: 0, borderRadius: 2,
    backgroundColor: 'transparent', color: search.inputText, filter: search.iconFilter,
    cursor: { default: 'pointer', ':disabled': 'not-allowed' }, opacity: { default: 0.72, ':enabled:hover': 0.96, ':enabled:active': 1, ':disabled': 0.4 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: 3, outlineColor: search.inputText,
    transitionProperty: 'opacity, transform', transitionDuration: '500ms', transitionTimingFunction: 'ease',
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
  },
  truncate: { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' },
})
