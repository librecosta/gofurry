import * as stylex from '@stylexjs/stylex'
import { tokens } from '../styles/tokens.stylex'

// Local Start controls only. One inset focus outline, no native/extra ring.
export const controls = stylex.create({
  glass: {
    borderWidth: 0, borderRadius: tokens.radiusSmall, fontFamily: 'inherit',
    // Exclude keyboard focus from pointer rules: their compound selector has
    // higher specificity than :focus-visible. :not(:disabled) covers links too.
    backgroundColor: { default: tokens.overlaySurface, ':is([data-popup-open])': tokens.overlaySelectedSurface, ':hover:not(:disabled):not(:focus-visible)': tokens.overlaySurfaceHover, ':active:not(:disabled):not(:focus-visible)': tokens.overlaySelectedSurface, ':focus-visible': tokens.overlaySelectedSurface },
    color: { default: tokens.overlayText, ':hover:not(:disabled):not(:focus-visible)': tokens.overlayHoverText, ':active:not(:disabled):not(:focus-visible)': tokens.overlaySelectedText, ':focus-visible': tokens.overlaySelectedText },
    backdropFilter: 'blur(14px) saturate(1.05)', boxShadow: 'none',
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    opacity: { default: 1, ':disabled': 0.55 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' },
    outlineWidth: 2, outlineOffset: -2, outlineColor: tokens.overlayFocus,
    transitionProperty: 'background-color, color', transitionDuration: { default: tokens.overlayMotion, ':active': '120ms' }, transitionTimingFunction: tokens.overlayEasing,
  },
})
