import * as stylex from '@stylexjs/stylex'

// Selected, currently consumed roles from the fixed legacy tokens.less reference.
// Equal literals retain their semantic role; no LESS/Nuxt runtime is imported.
export const tokens = stylex.defineVars({
  fontSans: '"Manrope Variable", "Noto Sans SC Variable", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  fontMono: '"Cascadia Mono", "SFMono-Regular", "SF Mono", Consolas, "Liberation Mono", ui-monospace, monospace',
  pageBackground: '#f2e3d0',
  dotColor: 'rgba(128, 108, 88, 0.24)',
  dotSpacing: { default: '30px', '@media (max-width: 767px)': '28px' },
  dotRadius: { default: '1.1px', '@media (max-width: 767px)': '0.8px' },
  dotHeight: { default: '720px', '@media (max-width: 767px)': '185px' },
  surface: 'rgba(255, 250, 242, 0.72)',
  surfaceHover: 'rgba(255, 239, 213, 0.72)',
  border: 'rgba(126, 92, 58, 0.16)',
  text: 'rgba(28, 25, 23, 0.94)',
  textMuted: 'rgba(87, 83, 78, 0.72)',
  accent: 'rgba(124, 45, 18, 0.86)',
  focusRing: 'rgba(249, 115, 22, 0.32)',
  // Hero-backed glass roles from legacy nav.less, independent of the page theme.
  overlaySurface: 'rgba(15, 23, 42, 0.50)',
  overlaySurfaceHover: 'rgba(15, 23, 42, 0.68)',
  overlayText: 'rgba(248, 250, 252, 0.92)',
  overlaySelectedSurface: 'rgba(255, 248, 241, 0.92)',
  overlaySelectedText: 'rgba(15, 23, 42, 0.94)',
  overlayHighlight: 'inset 0 1px 0 rgba(255, 255, 255, 0.12)',
  radiusSmall: '8px', radiusMedium: '12px', radiusLarge: '16px',
  motionFast: '160ms', motionBase: '220ms',
})

export const darkTheme = stylex.createTheme(tokens, {
  pageBackground: '#1d1f22', dotColor: 'rgba(193, 197, 203, 0.22)',
  surface: 'rgba(229, 231, 235, 0.08)', surfaceHover: 'rgba(229, 231, 235, 0.16)',
  border: 'rgba(226, 232, 240, 0.18)', text: 'rgba(248, 250, 252, 0.94)',
  textMuted: 'rgba(203, 213, 225, 0.76)', accent: 'rgba(203, 213, 225, 0.82)',
  focusRing: 'rgba(56, 189, 248, 0.34)',
})
