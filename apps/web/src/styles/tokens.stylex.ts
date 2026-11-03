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
  logoBrand: '#d7dce2',
  // Flat Start toolbar/menu acrylic. Search owns its independent material roles.
  overlaySurface: 'rgba(245, 247, 250, 0.33)',
  overlaySurfaceHover: 'rgba(248, 250, 252, 0.48)',
  overlayText: 'rgba(34, 39, 46, 0.90)',
  overlayHoverText: 'rgba(24, 29, 36, 0.98)',
  overlaySelectedSurface: 'rgba(231, 235, 241, 0.50)',
  overlaySelectedText: 'rgba(24, 29, 36, 0.98)',
  overlayFocus: 'rgba(39, 47, 58, 0.86)',
  overlayMotion: '300ms', overlayEasing: 'cubic-bezier(0.22, 1, 0.36, 1)',
  filingText: 'rgba(232, 236, 242, 0.78)',
  radiusSmall: '8px', radiusMedium: '12px', radiusLarge: '16px',
  motionFast: '160ms', motionBase: '220ms',
})

export const darkTheme = stylex.createTheme(tokens, {
  pageBackground: '#1d1f22', dotColor: 'rgba(193, 197, 203, 0.22)',
  surface: 'rgba(229, 231, 235, 0.08)', surfaceHover: 'rgba(229, 231, 235, 0.16)',
  border: 'rgba(226, 232, 240, 0.18)', text: 'rgba(248, 250, 252, 0.94)',
  textMuted: 'rgba(203, 213, 225, 0.76)', accent: 'rgba(203, 213, 225, 0.82)',
  focusRing: 'rgba(56, 189, 248, 0.34)',
  overlaySurface: 'rgba(27, 31, 38, 0.32)',
  overlaySurfaceHover: 'rgba(34, 40, 49, 0.48)',
  overlayText: 'rgba(224, 230, 238, 0.92)',
  overlayHoverText: 'rgba(242, 245, 249, 0.98)',
  overlaySelectedSurface: 'rgba(58, 67, 80, 0.50)',
  overlaySelectedText: 'rgba(242, 245, 249, 0.98)',
  overlayFocus: 'rgba(213, 223, 236, 0.88)',
  filingText: 'rgba(34, 39, 46, 0.76)',
})

// CSS resolves system colors before hydration, without guessing the client OS.
export const systemTheme = stylex.createTheme(tokens, {
  pageBackground: { default: '#f2e3d0', '@media (prefers-color-scheme: dark)': '#1d1f22' },
  dotColor: { default: 'rgba(128, 108, 88, 0.24)', '@media (prefers-color-scheme: dark)': 'rgba(193, 197, 203, 0.22)' },
  surface: { default: 'rgba(255, 250, 242, 0.72)', '@media (prefers-color-scheme: dark)': 'rgba(229, 231, 235, 0.08)' },
  surfaceHover: { default: 'rgba(255, 239, 213, 0.72)', '@media (prefers-color-scheme: dark)': 'rgba(229, 231, 235, 0.16)' },
  border: { default: 'rgba(126, 92, 58, 0.16)', '@media (prefers-color-scheme: dark)': 'rgba(226, 232, 240, 0.18)' },
  text: { default: 'rgba(28, 25, 23, 0.94)', '@media (prefers-color-scheme: dark)': 'rgba(248, 250, 252, 0.94)' },
  textMuted: { default: 'rgba(87, 83, 78, 0.72)', '@media (prefers-color-scheme: dark)': 'rgba(203, 213, 225, 0.76)' },
  accent: { default: 'rgba(124, 45, 18, 0.86)', '@media (prefers-color-scheme: dark)': 'rgba(203, 213, 225, 0.82)' },
  focusRing: { default: 'rgba(249, 115, 22, 0.32)', '@media (prefers-color-scheme: dark)': 'rgba(56, 189, 248, 0.34)' },
  overlaySurface: { default: 'rgba(245, 247, 250, 0.33)', '@media (prefers-color-scheme: dark)': 'rgba(27, 31, 38, 0.32)' },
  overlaySurfaceHover: { default: 'rgba(248, 250, 252, 0.48)', '@media (prefers-color-scheme: dark)': 'rgba(34, 40, 49, 0.48)' },
  overlayText: { default: 'rgba(34, 39, 46, 0.90)', '@media (prefers-color-scheme: dark)': 'rgba(224, 230, 238, 0.92)' },
  overlayHoverText: { default: 'rgba(24, 29, 36, 0.98)', '@media (prefers-color-scheme: dark)': 'rgba(242, 245, 249, 0.98)' },
  overlaySelectedSurface: { default: 'rgba(231, 235, 241, 0.50)', '@media (prefers-color-scheme: dark)': 'rgba(58, 67, 80, 0.50)' },
  overlaySelectedText: { default: 'rgba(24, 29, 36, 0.98)', '@media (prefers-color-scheme: dark)': 'rgba(242, 245, 249, 0.98)' },
  overlayFocus: { default: 'rgba(39, 47, 58, 0.86)', '@media (prefers-color-scheme: dark)': 'rgba(213, 223, 236, 0.88)' },
  filingText: { default: 'rgba(232, 236, 242, 0.78)', '@media (prefers-color-scheme: dark)': 'rgba(34, 39, 46, 0.76)' },
})
